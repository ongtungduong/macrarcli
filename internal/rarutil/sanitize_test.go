package rarutil

import "testing"

// TestSanitize covers the Zip-Slip / absolute-path defense: every entry name
// pulled from an untrusted archive must resolve to a relative path that stays
// within the archive root. Malicious or malformed names must be rejected.
func TestSanitize(t *testing.T) {
	safe := []struct {
		name string
		in   string
		want string
	}{
		{"plain file", "readme.txt", "readme.txt"},
		{"nested file", "docs/guide.md", "docs/guide.md"},
		{"interior dotdot resolves inside", "a/b/../c.txt", "a/c.txt"},
		{"leading dot slash", "./notes.txt", "notes.txt"},
		{"backslash separators normalized", `docs\guide.md`, "docs/guide.md"},
		{"redundant slashes collapsed", "a//b///c.txt", "a/b/c.txt"},
	}
	for _, tc := range safe {
		t.Run("safe/"+tc.name, func(t *testing.T) {
			got, err := sanitize(tc.in)
			if err != nil {
				t.Fatalf("sanitize(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	malicious := []struct {
		name string
		in   string
	}{
		{"absolute unix", "/etc/passwd"},
		{"parent escape", "../secret"},
		{"deep escape", "a/../../etc/passwd"},
		{"windows backslash escape", `..\..\windows\system32`},
		{"windows drive absolute", `C:\Windows\system32`},
		{"drive forward slash", "C:/Windows"},
		{"bare parent", ".."},
		{"empty", ""},
		{"only dot", "."},
	}
	for _, tc := range malicious {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if got, err := sanitize(tc.in); err == nil {
				t.Errorf("sanitize(%q) = %q, want rejection error", tc.in, got)
			}
		})
	}
}
