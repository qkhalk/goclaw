package vworker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// looksLikeHTML reports whether a payload head is an HTML document — the
// shape hotlink blocks and CDN error pages return instead of media.
func looksLikeHTML(head []byte) bool {
	t := strings.TrimSpace(string(head))
	if t == "" {
		return false
	}
	return strings.HasPrefix(t, "<!DOCTYPE") ||
		strings.HasPrefix(t, "<html") ||
		strings.HasPrefix(t, "<HTML") ||
		strings.HasPrefix(strings.ToLower(t), "<!doctype html")
}

const (
	// maxAssetSize is the maximum size for a single downloaded asset (100 MB).
	maxAssetSize = 100 * 1024 * 1024
	// downloadTimeout is the HTTP client timeout for asset downloads.
	downloadTimeout = 60 * time.Second
)

// Materialize resolves an asset source to a local file path.
// If the source is a local path (relative or absolute), it resolves relative
// to assetsDir. If the source is an HTTP(S) URL, it downloads to assetsDir
// with size cap and SSRF protection.
func Materialize(ctx context.Context, assetsDir, source, assetsBase string) (string, error) {
	if source == "" {
		return "", fmt.Errorf("empty asset source")
	}

	// Determine if it's a URL or a local path
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return downloadAsset(ctx, assetsDir, source)
	}

	// Local path: resolve relative to assetsDir or assetsBase
	resolved := source
	if !filepath.IsAbs(source) && assetsBase != "" {
		resolved = filepath.Join(assetsBase, source)
	} else if !filepath.IsAbs(source) && assetsDir != "" {
		resolved = filepath.Join(assetsDir, source)
	}

	// Verify the file exists
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("asset not found %s: %w", source, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("asset is a directory: %s", source)
	}
	return resolved, nil
}

// downloadAsset downloads a URL to assetsDir with SSRF guard and size cap.
func downloadAsset(ctx context.Context, assetsDir, rawURL string) (string, error) {
	// SSRF guard: block private IPs
	if err := ssrfGuard(rawURL); err != nil {
		return "", err
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL %s: %w", rawURL, err)
	}

	// Derive filename from URL path
	filename := filepath.Base(parsed.Path)
	if filename == "" || filename == "." {
		filename = "asset_download"
	}
	// Avoid path traversal in the filename
	filename = filepath.Base(filename)
	outPath := filepath.Join(assetsDir, filename)

	// Don't re-download if already present
	if _, err := os.Stat(outPath); err == nil {
		return outPath, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	// Bare GETs are rejected by many CDNs (vnecdn returns 401 without a
	// browser UA); send the same header set web_fetch uses.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,*/*;q=0.8")

	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}

	// Sniff the payload: CDN errors and hotlink blocks arrive as HTML pages
	// that ffmpeg would choke on with a cryptic decode error. Reject them
	// here with a message the agent can act on.
	head := make([]byte, 512)
	n, _ := io.ReadFull(resp.Body, head)
	head = head[:n]
	if looksLikeHTML(head) {
		os.Remove(outPath)
		return "", fmt.Errorf("download %s: got an HTML page, not a media asset (broken or hotlink-blocked URL)", rawURL)
	}
	respBody := io.MultiReader(bytes.NewReader(head), resp.Body)

	// Size cap: use Content-Length if available
	if resp.ContentLength > maxAssetSize {
		return "", fmt.Errorf("asset too large: %d bytes (max %d)", resp.ContentLength, maxAssetSize)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create file %s: %w", outPath, err)
	}
	defer f.Close()

	written, err := io.Copy(f, io.LimitReader(respBody, maxAssetSize+1))
	if err != nil {
		os.Remove(outPath)
		return "", fmt.Errorf("write asset: %w", err)
	}
	if written > maxAssetSize {
		os.Remove(outPath)
		return "", fmt.Errorf("asset too large: downloaded %d bytes (max %d)", written, maxAssetSize)
	}

	slog.Info("asset downloaded", "url", rawURL, "path", outPath, "bytes", written)
	return outPath, nil
}

// ssrfGuard checks if a URL targets a private/reserved IP.
func ssrfGuard(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("no host in URL: %s", rawURL)
	}

	// Resolve the host
	ips, err := net.LookupIP(host)
	if err != nil {
		// DNS lookup failure — still block to be safe
		return fmt.Errorf("DNS lookup failed for %s: %w", host, err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("SSRF blocked: %s resolves to private IP %s", host, ip.String())
		}
	}
	return nil
}

// isPrivateIP returns true for loopback, private, link-local, and unspecified IPs.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// RFC 1918 ranges
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	// IPv6 private: ULA fc00::/7
	if len(ip) == 16 && ip[0] == 0xfc {
		return true
	}
	return false
}

// MaterializeBGM resolves the BGM path from AudioMix config.
func MaterializeBGM(ctx context.Context, assetsDir string, bgmPath string) (string, error) {
	if bgmPath == "" {
		return "", nil
	}
	return Materialize(ctx, assetsDir, bgmPath, "")
}
