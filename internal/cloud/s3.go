package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// S3Provider connects any S3-compatible object store (AWS S3, Cloudflare R2,
// Backblaze B2, Wasabi, MinIO, DO Spaces — rclone's `s3` backend family).
// Unlike the OAuth providers it authenticates with static access keys, so
// there is no consent flow: the connect form validates the keys against the
// endpoint and stores them (encrypted at rest) directly.
const S3Provider = "s3"

// S3ConnectInput is the access-key connect payload.
type S3ConnectInput struct {
	Label     string `json:"label"` // display name (defaults to the bucket)
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

// ConnectS3 validates the credentials against the endpoint (one cheap list)
// and upserts the account row. The access/secret keys ride the same
// AES-256-GCM-encrypted columns as the OAuth tokens; endpoint/region/bucket
// are client-visible Settings (they are not secrets). Reconnecting the same
// label replaces the stored keys (the upsert dedupes on
// tenant+user+provider+label).
func (m *Manager) ConnectS3(ctx context.Context, tenantID, userID string, in S3ConnectInput) (*store.CloudAccount, error) {
	in.Label = strings.TrimSpace(in.Label)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.Region = strings.TrimSpace(in.Region)
	in.Bucket = strings.TrimSpace(in.Bucket)
	in.AccessKey = strings.TrimSpace(in.AccessKey)
	in.SecretKey = strings.TrimSpace(in.SecretKey)

	if in.Bucket == "" || in.AccessKey == "" || in.SecretKey == "" {
		return nil, errors.New("cloud: bucket, access_key and secret_key are required")
	}
	if in.Endpoint != "" && !strings.HasPrefix(strings.ToLower(in.Endpoint), "http://") && !strings.HasPrefix(strings.ToLower(in.Endpoint), "https://") {
		return nil, errors.New("cloud: endpoint must be an http(s) URL (empty = AWS S3)")
	}

	// Validate by exercising the endpoint — a typo in any field fails here,
	// before anything is persisted.
	backend := storage.NewS3Backend(ctx, storage.S3Creds{
		Endpoint: in.Endpoint, Region: in.Region, Bucket: in.Bucket,
		AccessKey: in.AccessKey, SecretKey: in.SecretKey,
	})
	if _, err := backend.List(ctx, "/", 1); err != nil {
		return nil, fmt.Errorf("cloud: S3 validation failed — check endpoint/bucket/keys: %w", err)
	}

	email := in.Label
	if email == "" {
		email = in.Bucket
	}
	settings, _ := json.Marshal(map[string]string{
		"endpoint": in.Endpoint,
		"region":   in.Region,
		"bucket":   in.Bucket,
	})

	sctx, err := scopeContext(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	acct := &store.CloudAccount{
		Provider:    S3Provider,
		Email:       email,
		DisplayName: fmt.Sprintf("S3 · %s", in.Bucket),
		Scopes:      `["readwrite"]`,
		// Static keys reuse the encrypted token columns; RefreshToken is not
		// a token here but the secret key (TokenSource is never used for s3).
		AccessToken:  in.AccessKey,
		RefreshToken: in.SecretKey,
		Status:       "active",
		Settings:     string(settings),
	}
	if err := m.store.Upsert(sctx, acct); err != nil {
		return nil, fmt.Errorf("cloud: persist account: %w", err)
	}
	return acct, nil
}

// s3AccountCreds rebuilds the storage.S3Creds of a connected s3 account.
func s3AccountCreds(acct *store.CloudAccount) (storage.S3Creds, error) {
	var settings struct {
		Endpoint string `json:"endpoint"`
		Region   string `json:"region"`
		Bucket   string `json:"bucket"`
	}
	if acct.Settings != "" {
		if err := json.Unmarshal([]byte(acct.Settings), &settings); err != nil {
			return storage.S3Creds{}, fmt.Errorf("cloud storage: account settings: %w", err)
		}
	}
	if settings.Bucket == "" || acct.AccessToken == "" || acct.RefreshToken == "" {
		return storage.S3Creds{}, errors.New("cloud storage: s3 account is missing bucket or keys — reconnect it")
	}
	return storage.S3Creds{
		Endpoint: settings.Endpoint, Region: settings.Region, Bucket: settings.Bucket,
		AccessKey: acct.AccessToken, SecretKey: acct.RefreshToken,
	}, nil
}
