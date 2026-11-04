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
		{"case distinctions preserved", "Docs/REPORT.txt", "Docs/REPORT.txt"},
		{"unicode preserved", "cafe\u0301.txt", "cafe\u0301.txt"},
		{"not a reserved device", "auxiliary/COM10.txt", "auxiliary/COM10.txt"},
		{"zero is not a device number", "COM0/LPT0", "COM0/LPT0"},
		{"internal space allowed", "my file.txt", "my file.txt"},
		{"dotfile allowed", ".config", ".config"},
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
		// A drive prefix without a slash is relative to that drive's current
		// directory, not to our extraction root. In particular a:b is unsafe.
		{"lowercase drive relative", "a:b"},
		{"drive relative", `C:Windows\file`},
		{"bare drive", "C:"},
		{"drive after dot", "./a:b"},
		{"drive after cleaning", "dir/../C:file"},
		{"UNC", `\\server\share\file`},
		{"device namespace", `\\?\C:\file`},
		{"alternate data stream", "file.txt:secret"},
		{"default data stream", "file.txt::$DATA"},
		{"nested stream", "dir/file:stream"},
		{"reserved device", "NUL"},
		{"nested device extension", "dir/con.txt"},
		{"reserved printer", "PRN"},
		{"reserved auxiliary", "a/AuX.log"},
		{"reserved serial port", "COM1.txt"},
		{"reserved parallel port", "LPT9"},
		{"superscript serial port", "COM\u00b9.log"},
		{"superscript parallel port", "LPT\u00b2"},
		{"superscript three", "COM\u00b3"},
		{"console input", "CONIN$"},
		{"console output", "conout$"},
		{"space before device extension", "NUL .txt"},
		{"trailing dot alias", "file."},
		{"trailing space alias", "dir /file"},
		{"NUL byte", "bad\x00name"},
		{"control character", "bad\x1fname"},
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
