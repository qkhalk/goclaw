package cloud

import "testing"

func TestCleanRemotePath(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		fails bool
	}{
		{in: "docs", want: "docs"},
		{in: "/docs/", want: "docs"},
		{in: "a/b/c.txt", want: "a/b/c.txt"},
		{in: "\\a\\b.txt", want: "a/b.txt"}, // Windows separators normalized
		{in: "/a//b/", fails: true},         // empty segment rejected (strict)
		{in: "a/./b", fails: true},          // dot segment
		{in: "a/../b", fails: true},           // traversal
		{in: "..", fails: true},               // traversal root
		{in: "../etc/passwd", fails: true},    // traversal
		{in: "a/b/../c", fails: true},         // traversal mid-path
		{in: "", fails: true},                 // empty
		{in: "/", fails: true},                // root only
		{in: ".", fails: true},                // dot only
	}
	for _, tc := range cases {
		got, err := CleanRemotePath(tc.in)
		if tc.fails {
			if err == nil {
				t.Errorf("CleanRemotePath(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("CleanRemotePath(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("CleanRemotePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanRemoteDir(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		fails bool
	}{
		{in: "", want: ""},       // root is valid for upload targets
		{in: "/", want: ""},      // root is valid for upload targets
		{in: "docs/", want: "docs"},
		{in: "a/b", want: "a/b"},
		{in: "../x", fails: true},
	}
	for _, tc := range cases {
		got, err := CleanRemoteDir(tc.in)
		if tc.fails {
			if err == nil {
				t.Errorf("CleanRemoteDir(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("CleanRemoteDir(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("CleanRemoteDir(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRemoteJoin(t *testing.T) {
	if got := RemoteJoin("", "f.txt"); got != "f.txt" {
		t.Errorf("RemoteJoin(root) = %q", got)
	}
	if got := RemoteJoin("docs", "f.txt"); got != "docs/f.txt" {
		t.Errorf("RemoteJoin(docs) = %q", got)
	}
}
