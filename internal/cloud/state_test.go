package cloud

import (
	"strings"
	"testing"
	"time"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestStateRoundtrip(t *testing.T) {
	payload := StatePayload{
		Provider: GoogleProvider,
		TenantID: "11111111-1111-1111-1111-111111111111",
		UserID:   "user-42",
		Verifier: "test-verifier-string",
	}
	state, err := EncodeState(payload, testKey)
	if err != nil {
		t.Fatalf("EncodeState: %v", err)
	}
	got, err := DecodeState(state, testKey)
	if err != nil {
		t.Fatalf("DecodeState: %v", err)
	}
	if got.Provider != payload.Provider || got.TenantID != payload.TenantID ||
		got.UserID != payload.UserID || got.Verifier != payload.Verifier {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.Nonce == "" {
		t.Fatal("nonce not auto-filled")
	}
	if got.ExpiresAt <= time.Now().Unix() {
		t.Fatal("expiry not auto-filled")
	}
}

func TestStateTamperRejected(t *testing.T) {
	state, err := EncodeState(StatePayload{Provider: GoogleProvider, TenantID: "t", UserID: "u"}, testKey)
	if err != nil {
		t.Fatalf("EncodeState: %v", err)
	}
	// Flip a payload byte (base64 part).
	flipped := []byte(state)
	if flipped[0] == 'A' {
		flipped[0] = 'B'
	} else {
		flipped[0] = 'A'
	}
	if _, err := DecodeState(string(flipped), testKey); err == nil {
		t.Fatal("tampered state accepted")
	}
	// Wrong key.
	if _, err := DecodeState(state, strings.Repeat("ab", 32)); err == nil {
		t.Fatal("state accepted under wrong key")
	}
	// Truncated (no signature).
	if _, err := DecodeState(state[:len(state)/2], testKey); err == nil {
		t.Fatal("truncated state accepted")
	}
}

func TestStateExpiry(t *testing.T) {
	payload := StatePayload{Provider: GoogleProvider, TenantID: "t", UserID: "u"}
	payload.ExpiresAt = time.Now().Add(-time.Minute).Unix()
	state, err := EncodeState(payload, testKey)
	if err != nil {
		t.Fatalf("EncodeState: %v", err)
	}
	if _, err := DecodeState(state, testKey); err == nil {
		t.Fatal("expired state accepted")
	}
}

func TestVerifierChallengeS256(t *testing.T) {
	// RFC 7636 appendix B vector.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := VerifierChallenge(verifier); got != want {
		t.Fatalf("challenge = %s, want %s", got, want)
	}
}

func TestGoogleScopesNoFullMailAccess(t *testing.T) {
	for _, s := range GoogleScopes {
		if s == "https://mail.google.com/" {
			t.Fatal("full mail scope must never be requested in v1 (no permanent delete)")
		}
	}
}

func TestRedirectURI(t *testing.T) {
	if got := RedirectURI("https://goclaw.example.com"); got != "https://goclaw.example.com/v1/cloud/oauth/callback" {
		t.Fatalf("RedirectURI = %q", got)
	}
	if got := RedirectURI("https://goclaw.example.com/"); got != "https://goclaw.example.com/v1/cloud/oauth/callback" {
		t.Fatalf("RedirectURI trailing slash = %q", got)
	}
}
