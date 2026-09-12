package methods

import "testing"

// TestPermissionModeFor proves the chat.send permissionMode param accepts
// exactly the composer modes and rejects unknown values to "" (agent default
// policies apply — the safe direction).
func TestPermissionModeFor(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"plan", "plan"},
		{"full_access", "full_access"},
		{"write_approval", "write_approval"},
		{"always_ask", "always_ask"},
		{"bogus", ""},
		{"PLAN", ""},
	}
	for _, tc := range cases {
		if got := permissionModeFor(tc.in); got != tc.want {
			t.Fatalf("permissionModeFor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
