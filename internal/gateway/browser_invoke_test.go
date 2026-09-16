package gateway

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// registerTestClient creates a client, marks it authenticated for the given
// tenant/user, and registers it with the server so the bridge can find it.
func registerTestClient(t *testing.T, server *Server, tenantID uuid.UUID, userID string) *Client {
	t.Helper()
	client := NewClient(nil, server, "127.0.0.1")
	client.authenticated = true
	client.userID = userID
	client.tenantID = tenantID
	server.registerClient(client)
	return client
}

func TestInvokeBrowserPanelDeliversResult(t *testing.T) {
	server := NewServer(configForTest(), bus.New(), nil, nil)
	tenantID := uuid.New()
	client := registerTestClient(t, server, tenantID, "user-1")

	type invokeOutcome struct {
		result map[string]any
		err    error
	}
	done := make(chan invokeOutcome, 1)
	go func() {
		res, err := server.BrowserPanelBridge().InvokeBrowserPanel(
			t.Context(), tenantID, "user-1",
			map[string]any{"url": "https://example.com"}, 2*time.Second)
		done <- invokeOutcome{res, err}
	}()

	evt := readEvent(t, client)
	if evt.Event != protocol.EventBrowserPanelInvoke {
		t.Fatalf("event = %q, want %q", evt.Event, protocol.EventBrowserPanelInvoke)
	}
	browseID, _ := evt.Payload.(map[string]any)["browseId"].(string)
	if browseID == "" {
		t.Fatal("invoke event missing browseId")
	}

	if !server.BrowserPanelBridge().DeliverBrowserResult(browseID, client, map[string]any{
		"content": "# hello", "title": "Example", "finalUrl": "https://example.com",
	}) {
		t.Fatal("DeliverBrowserResult = false, want true")
	}

	outcome := <-done
	if outcome.err != nil {
		t.Fatalf("invoke error: %v", outcome.err)
	}
	if outcome.result["content"] != "# hello" {
		t.Fatalf("content = %v, want '# hello'", outcome.result["content"])
	}
}

func TestInvokeBrowserPanelTimeout(t *testing.T) {
	server := NewServer(configForTest(), bus.New(), nil, nil)
	tenantID := uuid.New()
	registerTestClient(t, server, tenantID, "user-1")

	_, err := server.BrowserPanelBridge().InvokeBrowserPanel(
		t.Context(), tenantID, "user-1",
		map[string]any{"url": "https://example.com"}, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
}

func TestInvokeBrowserPanelNoClient(t *testing.T) {
	server := NewServer(configForTest(), bus.New(), nil, nil)
	_, err := server.BrowserPanelBridge().InvokeBrowserPanel(
		t.Context(), uuid.New(), "ghost",
		map[string]any{"url": "https://example.com"}, time.Second)
	if err != ErrBrowserClientOffline {
		t.Fatalf("err = %v, want ErrBrowserClientOffline", err)
	}
}

func TestDeliverBrowserResultWrongSender(t *testing.T) {
	server := NewServer(configForTest(), bus.New(), nil, nil)
	tenantID := uuid.New()
	target := registerTestClient(t, server, tenantID, "user-1")
	intruder := registerTestClient(t, server, tenantID, "user-2")

	done := make(chan error, 1)
	go func() {
		_, err := server.BrowserPanelBridge().InvokeBrowserPanel(
			t.Context(), tenantID, "user-1",
			map[string]any{"browseId": "browse-1", "url": "https://example.com"}, 2*time.Second)
		done <- err
	}()

	evt := readEvent(t, target) // blocks until the invoke event reaches target
	if evt.Event != protocol.EventBrowserPanelInvoke {
		t.Fatalf("event = %q", evt.Event)
	}

	if server.BrowserPanelBridge().DeliverBrowserResult("browse-1", intruder, map[string]any{"content": "spoof"}) {
		t.Fatal("intruder delivery accepted")
	}
	if !server.BrowserPanelBridge().DeliverBrowserResult("browse-1", target, map[string]any{"content": "real"}) {
		t.Fatal("target delivery rejected after intruder attempt")
	}
	if err := <-done; err != nil {
		t.Fatalf("invoke error: %v", err)
	}
}

func TestDeliverBrowserResultCapsContent(t *testing.T) {
	server := NewServer(configForTest(), bus.New(), nil, nil)
	tenantID := uuid.New()
	client := registerTestClient(t, server, tenantID, "user-1")

	done := make(chan map[string]any, 1)
	go func() {
		res, err := server.BrowserPanelBridge().InvokeBrowserPanel(
			t.Context(), tenantID, "user-1",
			map[string]any{"browseId": "browse-cap", "url": "https://example.com"}, 2*time.Second)
		if err != nil {
			t.Errorf("invoke error: %v", err)
		}
		done <- res
	}()

	readEvent(t, client)

	huge := strings.Repeat("x", MaxBrowserResultBytes+1024)
	server.BrowserPanelBridge().DeliverBrowserResult("browse-cap", client, map[string]any{"content": huge})

	res := <-done
	content, _ := res["content"].(string)
	if len(content) != MaxBrowserResultBytes {
		t.Fatalf("content len = %d, want %d", len(content), MaxBrowserResultBytes)
	}
	if truncated, _ := res["truncated"].(bool); !truncated {
		t.Fatal("truncated flag missing")
	}
}
