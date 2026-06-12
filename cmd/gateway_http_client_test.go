package cmd

import "testing"

func TestResolveGatewayBaseURLUsesGOCLAWServer(t *testing.T) {
	t.Setenv("GOCLAW_SERVER", "http://127.0.0.1:19999/")

	got := resolveGatewayBaseURL()
	if got != "http://127.0.0.1:19999" {
		t.Fatalf("resolveGatewayBaseURL()=%q, want GOCLAW_SERVER without trailing slash", got)
	}
}
