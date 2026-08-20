package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// Package crypto: ed25519 signature helpers for signed skill packages
// (Phase 3 W2). Kept separate from the AES-256-GCM secret path in aes.go —
// publisher signatures are a public-key trust anchor, not symmetric secrets.

// ErrSignatureInvalid is returned by VerifyPackage when the signature does
// not authenticate the manifest for the given public key.
var ErrSignatureInvalid = errors.New("invalid signature")

// ErrSignatureKeyMismatch is returned by VerifyPackage when the public key
// does not have the expected fingerprint (defense against key/substitution).
var ErrSignatureKeyMismatch = errors.New("public key does not match expected fingerprint")

// GeneratePublisherKeypair generates a fresh ed25519 publisher keypair.
func GeneratePublisherKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519 keypair: %w", err)
	}
	return pub, priv, nil
}

// SignPackage signs a skill manifest (arbitrary bytes) with the publisher's
// ed25519 private key and returns the 64-byte signature.
func SignPackage(priv ed25519.PrivateKey, manifest []byte) ([]byte, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid ed25519 private key length")
	}
	return ed25519.Sign(priv, manifest), nil
}

// VerifyPackage verifies that sig authenticates manifest for pub.
// Returns ErrSignatureInvalid on a tampered manifest or bogus signature.
func VerifyPackage(pub ed25519.PublicKey, manifest []byte, sig []byte) error {
	if len(pub) != ed25519.PublicKeySize {
		return ErrSignatureInvalid
	}
	if len(sig) != ed25519.SignatureSize {
		return ErrSignatureInvalid
	}
	if !ed25519.Verify(pub, manifest, sig) {
		return ErrSignatureInvalid
	}
	return nil
}

// VerifyPackageWithFingerprint verifies the signature AND that pub hashes to
// the expected fingerprint (hex(sha256(pub))). This is the trust-anchor check:
// a signature from an unregistered key fails even if the signature itself is
// valid, because the publisher key is not in publisher_keys.
func VerifyPackageWithFingerprint(pub ed25519.PublicKey, expectedFingerprint string, manifest []byte, sig []byte) error {
	if Fingerprint(pub) != expectedFingerprint {
		return ErrSignatureKeyMismatch
	}
	return VerifyPackage(pub, manifest, sig)
}

// Fingerprint returns the identifier for a public key:
// hex(sha256(public_key)) lowercase — matches publisher_keys.fingerprint.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}