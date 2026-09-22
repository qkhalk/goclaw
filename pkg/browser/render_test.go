package browser

import (
	"context"
	"strings"
	"testing"
)

func TestRenderHTMLRejectsEmptyURL(t *testing.T) {
	m := &Manager{}
	if _, err := m.RenderHTML(context.Background(), ""); err == nil ||
		!strings.Contains(err.Error(), "empty url") {
		t.Fatalf("RenderHTML(\"\") error = %v, want empty url", err)
	}
}

func TestRenderHTMLRequiresRunningBackend(t *testing.T) {
	m := &Manager{}
	if _, err := m.RenderHTML(context.Background(), "http://example.com/"); err == nil ||
		!strings.Contains(err.Error(), "browser not running") {
		t.Fatalf("RenderHTML on stopped manager error = %v, want not running", err)
	}
}

func TestRenderHTMLRejectsLightpandaBackend(t *testing.T) {
	// Running lightpanda (cdpURL set) must not attempt a "full JS" render:
	// its engine cannot honor the contract, so the caller keeps the fetch.
	m := &Manager{backend: BackendLightpanda, cdpURL: "ws://127.0.0.1:9222"}
	if _, err := m.RenderHTML(context.Background(), "http://example.com/"); err == nil ||
		!strings.Contains(err.Error(), "chrome backend") {
		t.Fatalf("RenderHTML on lightpanda error = %v, want chrome backend", err)
	}
}
