package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
)

// S3Backend implements Backend against any S3-compatible endpoint (AWS S3,
// Cloudflare R2, Backblaze B2, Wasabi, MinIO, DO Spaces — rclone's `s3`
// backend family). Auth is static keys, so the oauth2.TokenSource plumbing
// of the other backends is unused; NewS3Backend builds its own client.
type S3Backend struct {
	client *s3.Client
	bucket string
}

// isS3NotFound maps NoSuchKey / NotFound / 404 API errors to ErrNotFound.
func isS3NotFound(err error) bool {
	var noKey *types.NoSuchKey
	if errors.As(err, &noKey) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NotFound" {
		return true
	}
	return strings.Contains(err.Error(), "StatusCode: 404")
}

// Compile-time guard.
var _ Backend = (*S3Backend)(nil)

// S3Creds carries the static configuration of one S3-compatible account.
type S3Creds struct {
	Endpoint  string // "" = AWS default; e.g. https://<account>.r2.cloudflarestorage.com
	Region    string // "" = auto/us-east-1
	Bucket    string
	AccessKey string
	SecretKey string
}

// NewS3Backend builds an S3 backend. Path-style addressing is forced (the
// non-AWS endpoints GoClaw targets — MinIO, R2, B2, DO — all require it,
// and virtual-host style is never valid for arbitrary endpoints).
func NewS3Backend(_ context.Context, creds S3Creds) *S3Backend {
	region := creds.Region
	if region == "" {
		region = "us-east-1"
	}
	client := s3.New(s3.Options{
		Region: region,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: creds.AccessKey, SecretAccessKey: creds.SecretKey}, nil
		}),
		UsePathStyle: true,
		BaseEndpoint: func() *string {
			if creds.Endpoint == "" {
				return nil
			}
			return aws.String(strings.TrimRight(creds.Endpoint, "/"))
		}(),
	})
	return &S3Backend{client: client, bucket: creds.Bucket}
}

// key normalizes our "/"-rooted path into an S3 object key (no leading slash).
func s3Key(p string) string {
	return strings.TrimPrefix(p, "/")
}

// dirPrefix returns the listing prefix for a directory path ("" for root).
func s3DirPrefix(dir string) string {
	k := s3Key(dir)
	if k == "" {
		return ""
	}
	return strings.TrimRight(k, "/") + "/"
}

// List lists one directory level (delimiter walk — no recursive listing).
func (b *S3Backend) List(ctx context.Context, dir string, limit int) ([]ListEntry, error) {
	if limit <= 0 {
		limit = 1000
	}
	prefix := s3DirPrefix(dir)
	out := make([]ListEntry, 0, 64)
	var continuation *string
	for len(out) < limit {
		page, err := b.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(b.bucket),
			Prefix:            aws.String(prefix),
			Delimiter:         aws.String("/"),
			MaxKeys:           aws.Int32(int32(min(limit-len(out), 1000))),
			ContinuationToken: continuation,
		})
		if err != nil {
			return nil, fmt.Errorf("s3 list: %w", err)
		}
		for _, cp := range page.CommonPrefixes {
			name := strings.TrimSuffix(strings.TrimPrefix(aws.ToString(cp.Prefix), prefix), "/")
			out = append(out, ListEntry{Name: name, IsDir: true})
		}
		for _, obj := range page.Contents {
			name := strings.TrimPrefix(aws.ToString(obj.Key), prefix)
			if name == "" {
				continue // the folder marker itself
			}
			if strings.Contains(name, "/") {
				continue // deeper keys leaked through (marker-less empty dirs)
			}
			out = append(out, ListEntry{
				Name:    name,
				IsDir:   false,
				Size:    aws.ToInt64(obj.Size),
				ModTime: normModTime(aws.ToTime(obj.LastModified).Format(time.RFC3339Nano)),
			})
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		continuation = page.NextContinuationToken
		if continuation == nil {
			break
		}
	}
	return out, nil
}

// Stat stats one object; directories are prefix-existence checks (S3 has no
// real folders).
func (b *S3Backend) Stat(ctx context.Context, path string) (*StatInfo, error) {
	key := s3Key(path)
	if key == "" {
		return &StatInfo{Name: "", IsDir: true}, nil
	}
	if head, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket), Key: aws.String(key),
	}); err == nil {
		return &StatInfo{
			Name:     key[strings.LastIndex(key, "/")+1:],
			Size:     aws.ToInt64(head.ContentLength),
			MimeType: aws.ToString(head.ContentType),
			ModTime:  normModTime(aws.ToTime(head.LastModified).Format(time.RFC3339Nano)),
		}, nil
	}
	// Not an object — is it a folder prefix (marker or children)?
	prefix := s3DirPrefix(path)
	page, lerr := b.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(1),
	})
	if lerr == nil && (len(page.Contents) > 0 || len(page.CommonPrefixes) > 0) {
		return &StatInfo{Name: key[strings.LastIndex(key, "/")+1:], IsDir: true}, nil
	}
	return nil, ErrNotFound
}

// About returns zeroed quota — S3 exposes no account-level capacity; the UI
// treats 0 totals as "unknown" and hides the bar.
func (b *S3Backend) About(ctx context.Context) (*AboutInfo, error) {
	return &AboutInfo{Total: 0, Used: 0, Free: 0}, nil
}

// Mkdir writes the folder marker object.
func (b *S3Backend) Mkdir(ctx context.Context, dir string) error {
	key := s3DirPrefix(dir)
	if key == "" {
		return nil // root always exists
	}
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket), Key: aws.String(key), ContentLength: aws.Int64(0),
	})
	if err != nil {
		return fmt.Errorf("s3 mkdir: %w", err)
	}
	return nil
}

// Delete removes a file or an EMPTY directory (marker + children check
// preserves rclone rmdir semantics).
func (b *S3Backend) Delete(ctx context.Context, path string, isDir bool) error {
	if isDir {
		prefix := s3DirPrefix(path)
		if prefix != "" {
			page, err := b.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
				Bucket: aws.String(b.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(2),
			})
			if err != nil {
				return fmt.Errorf("s3 delete: %w", err)
			}
			if len(page.Contents) > 0 || len(page.CommonPrefixes) > 0 {
				return ErrDirNotEmpty
			}
		}
	}
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket), Key: aws.String(s3Key(path)),
	})
	if err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	return nil
}

// Move copies then deletes (S3 has no rename).
func (b *S3Backend) Move(ctx context.Context, from, to string) error {
	if err := b.Copy(ctx, from, to); err != nil {
		return err
	}
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket), Key: aws.String(s3Key(from)),
	})
	if err != nil {
		return fmt.Errorf("s3 move (delete source): %w", err)
	}
	return nil
}

// Copy copies one object server-side.
func (b *S3Backend) Copy(ctx context.Context, from, to string) error {
	_, err := b.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(b.bucket),
		Key:        aws.String(s3Key(to)),
		CopySource: aws.String(b.bucket + "/" + s3Key(from)),
	})
	if err != nil {
		return fmt.Errorf("s3 copy: %w", err)
	}
	return nil
}

// CopyURL fetches an http(s) URL through the SSRF guard and stores it (S3
// has no native server-side URL ingest).
func (b *S3Backend) CopyURL(ctx context.Context, rawURL, path string) error {
	if !trimURLScheme(rawURL) {
		return errors.New("s3 copyurl: only http(s) URLs are supported")
	}
	resp, err := safeGet(ctx, rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, size, err := spoolUnknownSize(resp.Body)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	return b.Upload(ctx, path, f, size, resp.Header.Get("Content-Type"), time.Time{})
}

// PublicLink returns a presigned GET URL (S3's equivalent of a share link).
func (b *S3Backend) PublicLink(ctx context.Context, path string) (*PublicLinkInfo, error) {
	presign := s3.NewPresignClient(b.client)
	req, err := presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket), Key: aws.String(s3Key(path)),
	}, s3.WithPresignExpires(24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("s3 presign: %w", err)
	}
	return &PublicLinkInfo{URL: req.URL}, nil
}

// Open streams the object body, relaying rangeHdr (S3 honors Range).
func (b *S3Backend) Open(ctx context.Context, path string, rangeHdr RangeHeader) (*Content, error) {
	in := &s3.GetObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(s3Key(path))}
	if rangeHdr != "" {
		in.Range = aws.String(string(rangeHdr))
	}
	out, err := b.client.GetObject(ctx, in)
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("s3 open: %w", err)
	}
	status := http.StatusOK
	if rangeHdr != "" && out.ContentRange != nil {
		status = http.StatusPartialContent
	}
	length := int64(-1)
	if out.ContentLength != nil {
		length = *out.ContentLength
	}
	return &Content{
		ReadCloser:   out.Body,
		Status:       status,
		Length:       length,
		MimeType:     aws.ToString(out.ContentType),
		ContentRange: aws.ToString(out.ContentRange),
	}, nil
}

// Upload writes src to the key (PutObject overwrite; S3 has no
// client-controlled mtime, so modTime is accepted but not persisted — sync
// mirrors fall back to size comparison).
func (b *S3Backend) Upload(ctx context.Context, path string, src io.Reader, size int64, contentType string, _ time.Time) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket), Key: aws.String(s3Key(path)), Body: src,
	}
	if size >= 0 {
		in.ContentLength = aws.Int64(size)
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := b.client.PutObject(ctx, in); err != nil {
		return fmt.Errorf("s3 upload: %w", err)
	}
	return nil
}
