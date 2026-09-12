// Package cloud implements per-user OAuth connections to cloud providers
// (Google first: Gmail + Drive). It is provider-agnostic by design — adding
// Microsoft later means adding one provider file and a scope list.
package cloud

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/crypto"
)

// stateTTL bounds how long a signed authorize-URL state stays valid — long
// enough for a user to click through Google's consent screen, short enough
// that a leaked link has a tight blast radius (bitrix24 pattern).
const stateTTL = 10 * time.Minute

// StatePayload is the identity + routing context carried through the OAuth
// redirect round-trip. Stateless by design (no DB row): the state itself is
// self-verifying (HMAC + embedded expiry). The PKCE verifier rides inside the
// payload so a restart can't orphan an in-flight exchange; leaking it is
// acceptable for confidential clients because the exchange additionally needs
// the client_secret which never leaves the server.
type StatePayload struct {
	Provider string `json:"p"`
	TenantID string `json:"t"`
	UserID   string `json:"u"`
	Verifier string `json:"v"`          // PKCE code_verifier (S256)
	Redirect string `json:"r,omitempty"` // redirect_uri used in the auth request (RFC 6749 §4.1.3: exchange must repeat it)
	Nonce    string `json:"n"`
	ExpiresAt int64 `json:"exp"` // unix seconds
}

// EncodeState serializes the payload to base64url(json) + "." + hex(HMAC-SHA256).
// key is derived from the instance encryption key with a distinct context
// label so the state-signing key never equals the AES token key directly.
func EncodeState(p StatePayload, encryptionKey string) (string, error) {
	if p.Nonce == "" {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return "", fmt.Errorf("cloud state: nonce: %w", err)
		}
		p.Nonce = hex.EncodeToString(nonce)
	}
	if p.ExpiresAt == 0 {
		p.ExpiresAt = time.Now().Add(stateTTL).Unix()
	}
	body, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("cloud state: marshal payload: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	sig := signStatePart(encoded, encryptionKey)
	return encoded + "." + hex.EncodeToString(sig), nil
}

// DecodeState verifies the HMAC signature (constant-time) and expiry BEFORE
// returning the payload, so tampered or expired states never reach a caller
// that might act on them.
func DecodeState(state, encryptionKey string) (*StatePayload, error) {
	idx := -1
	for i := len(state) - 1; i >= 0; i-- {
		if state[i] == '.' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, errors.New("cloud state: malformed (missing signature)")
	}
	encoded, sigHex := state[:idx], state[idx+1:]

	gotSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, errors.New("cloud state: malformed signature encoding")
	}
	wantSig := signStatePart(encoded, encryptionKey)
	if !hmac.Equal(gotSig, wantSig) {
		return nil, errors.New("cloud state: signature mismatch")
	}

	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("cloud state: decode payload: %w", err)
	}
	var p StatePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("cloud state: unmarshal payload: %w", err)
	}
	if time.Now().Unix() > p.ExpiresAt {
		return nil, errors.New("cloud state: expired")
	}
	return &p, nil
}

// stateKeyContext derives a signing key from the instance encryption key so
// the HMAC key is distinct from the AES-GCM token key (key separation).
func stateKeyContext(encryptionKey string) []byte {
	derived, err := crypto.DeriveKey(encryptionKey)
	if err != nil {
		return []byte("goclaw:cloud-oauth-state:" + encryptionKey)
	}
	mac := hmac.New(sha256.New, derived)
	mac.Write([]byte("goclaw:cloud-oauth-state"))
	return mac.Sum(nil)
}

func signStatePart(encodedPayload, encryptionKey string) []byte {
	mac := hmac.New(sha256.New, stateKeyContext(encryptionKey))
	mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}
