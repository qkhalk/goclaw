package telegram

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestAttemptMediaDownload_MidStreamAbortProducesError verifies a mid-stream
// abort surfaces as an error (the production failure mode: Telegram DC reset
// the connection mid-"save file").
func TestAttemptMediaDownload_MidStreamAbortProducesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte{0x01}, 1024))
		panic(http.ErrAbortHandler) // kill the connection mid-stream
	}))
	defer srv.Close()

	client := srv.Client()
	client.Timeout = 5 * time.Second

	_, err := attemptMediaDownload(context.Background(), client, srv.URL, ".mp4", 1024*1024)
	if err == nil {
		t.Fatalf("expected an error when the stream dies mid-download")
	}
	if errors.Is(err, errMediaTooLarge) {
		t.Errorf("mid-stream abort must not be classified as too-large: %v", err)
	}
}

// TestMediaDownloadRetryLoop_RecoversOnSecondAttempt drives the retry loop
// semantics used by downloadMedia: attempt 1 dies mid-stream, attempt 2
// delivers the full body — the file must be saved despite the first failure.
func TestMediaDownloadRetryLoop_RecoversOnSecondAttempt(t *testing.T) {
	var attempts int
	payload := strings.Repeat("goclaw-video-payload;", 2000) // ~42 KB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		if attempts == 1 {
			_, _ = w.Write([]byte("partial"))
			panic(http.ErrAbortHandler)
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Timeout = 5 * time.Second

	var path string
	var err error
	for attempt := 1; attempt <= downloadMaxRetries; attempt++ {
		path, err = attemptMediaDownload(context.Background(), client, srv.URL, ".mp4", 1024*1024)
		if err == nil {
			break
		}
		if attempt < downloadMaxRetries {
			time.Sleep(time.Duration(attempt) * 10 * time.Millisecond)
		}
	}
	if err != nil {
		t.Fatalf("retry loop did not recover: %v", err)
	}
	defer os.Remove(path)

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read saved file: %v", readErr)
	}
	if string(data) != payload {
		t.Errorf("saved file differs from payload: %d bytes vs %d", len(data), len(payload))
	}
	if attempts < 2 {
		t.Errorf("expected at least 2 handler attempts, got %d", attempts)
	}
}

// TestMediaDownloadRetryLoop_TooLargeIsTerminal verifies the size guard stops
// the retry loop immediately (retrying cannot shrink a file).
func TestMediaDownloadRetryLoop_TooLargeIsTerminal(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Timeout = 5 * time.Second

	var lastErr error
	for attempt := 1; attempt <= downloadMaxRetries; attempt++ {
		var err error
		_, err = attemptMediaDownload(context.Background(), client, srv.URL, ".bin", 1024)
		if err == nil {
			t.Fatalf("expected too-large error")
		}
		lastErr = err
		if errors.Is(err, errMediaTooLarge) {
			break
		}
	}
	if !errors.Is(lastErr, errMediaTooLarge) {
		t.Fatalf("expected errMediaTooLarge, got %v", lastErr)
	}
	if attempts != 1 {
		t.Errorf("too-large must be terminal (1 attempt), got %d", attempts)
	}
}
