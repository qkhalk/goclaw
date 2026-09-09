package nodes

import (
	"strings"
	"testing"
)

func TestGenerateKeyFormat(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if !strings.HasPrefix(key, NodeKeyPrefix) {
		t.Fatalf("key %q missing prefix %q", key, NodeKeyPrefix)
	}
	body := strings.TrimPrefix(key, NodeKeyPrefix)
	if len(body) != 64 {
		t.Fatalf("key body length = %d, want 64 hex chars", len(body))
	}
	if !ValidateKeyFormat(key) {
		t.Fatalf("ValidateKeyFormat rejected a generated key")
	}
}

func TestGenerateKeyUnique(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		key, err := GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey: %v", err)
		}
		if seen[key] {
			t.Fatalf("duplicate key generated")
		}
		seen[key] = true
	}
}

func TestHashKey(t *testing.T) {
	key, _ := GenerateKey()
	h1 := HashKey(key)
	h2 := HashKey(key)
	if h1 != h2 {
		t.Fatalf("HashKey not deterministic")
	}
	if len(h1) != 64 {
		t.Fatalf("hash length = %d, want 64", len(h1))
	}
	// Lowercase hex digest.
	for _, c := range h1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("hash %q contains non-lowercase-hex char %q", h1, string(c))
		}
	}
	// Whitespace-insensitive.
	if HashKey(" "+key) != h1 {
		t.Fatalf("HashKey should trim surrounding whitespace")
	}
	// Different keys → different hashes.
	other, _ := GenerateKey()
	if HashKey(other) == h1 {
		t.Fatalf("different keys produced identical hashes")
	}
}

func TestValidateKeyFormat(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"gnk_", false},
		{"gnk_" + strings.Repeat("a", 63), false}, // 63 chars
		{"gnk_" + strings.Repeat("a", 65), false}, // 65 chars
		{"gnk_" + strings.Repeat("G", 64), false}, // uppercase not hex-lower
		{"gnk_" + strings.Repeat("0", 64), true},  // valid
		{"gnk_" + strings.Repeat("f", 64), true},  // valid
		{"bad_" + strings.Repeat("a", 64), false}, // wrong prefix
		{strings.Repeat("a", 64), false},          // no prefix
		{" gnk_" + strings.Repeat("a", 64), true}, // surrounding whitespace tolerated
	}
	for _, tc := range cases {
		if got := ValidateKeyFormat(tc.in); got != tc.want {
			t.Errorf("ValidateKeyFormat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
