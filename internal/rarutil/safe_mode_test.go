package rarutil

import (
	"io/fs"
	"testing"
)

// TestSafeMode asserts that non-permission type bits are stripped from an
// archive entry's mode, and that group/other-write bits are always masked
// out. Stripping type bits neutralizes symlink/device/pipe/socket entries:
// stored with a plain mode, their body becomes inert file content rather than
// being recreated as a link or special file that could escape the extract
// root. Masking write bits prevents an archive-controlled mode (e.g. 0777)
// from making an extracted file writable by other local users.
func TestSafeMode(t *testing.T) {
	tests := []struct {
		name  string
		in    fs.FileMode
		isDir bool
		want  fs.FileMode
	}{
		{"plain file perms kept", 0o644, false, 0o644},
		{"executable kept", 0o755, false, 0o755},
		{"symlink bit stripped", fs.ModeSymlink | 0o777, false, 0o755},
		{"device bit stripped", fs.ModeDevice | 0o660, false, 0o640},
		{"pipe bit stripped", fs.ModeNamedPipe | 0o644, false, 0o644},
		{"socket bit stripped", fs.ModeSocket | 0o644, false, 0o644},
		{"setuid stripped", fs.ModeSetuid | 0o755, false, 0o755},
		{"dir keeps dir bit", fs.ModeDir | 0o755, true, fs.ModeDir | 0o755},
		{"symlink-as-dir reduces to dir", fs.ModeSymlink | 0o777, true, fs.ModeDir | 0o755},
		{"group/other write bits masked", 0o777, false, 0o755},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeMode(tc.in, tc.isDir); got != tc.want {
				t.Errorf("safeMode(%v, %v) = %v, want %v", tc.in, tc.isDir, got, tc.want)
			}
		})
	}
}
