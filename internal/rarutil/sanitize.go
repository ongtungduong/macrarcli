package rarutil

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// sanitize validates and normalizes an archive entry name so it cannot escape
// the extraction root (Zip Slip). It works on archive-style '/' separated paths
// regardless of host OS, returning a clean relative path with no trailing slash.
//
// It rejects empty/absolute/drive-relative paths and paths resolving at or above
// the root. Windows device names, streams, and ambiguous trailing dots/spaces
// are rejected on every host so accepted paths stay safe across platforms.
func sanitize(name string) (string, error) {
	// Archives may use Windows '\' separators; normalize before reasoning.
	s := strings.ReplaceAll(name, `\`, "/")
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("invalid empty entry name")
	}
	// A drive prefix is unsafe even without '/': "a:b" is relative to drive
	// A's current directory, not our extraction root (as are "C:" and "C:foo").
	if len(s) >= 2 && s[1] == ':' {
		return "", fmt.Errorf("drive-qualified path not allowed: %q", name)
	}
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("absolute path not allowed: %q", name)
	}

	clean := path.Clean(s)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes archive root: %q", name)
	}
	for _, part := range strings.Split(clean, "/") {
		// ':' also introduces NTFS alternate data streams, including in a
		// nested component or a drive prefix exposed only after path.Clean.
		if strings.ContainsAny(part, `:<>"|?*`) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || windowsDeviceName(part) {
			return "", fmt.Errorf("unsafe Windows path component in %q", name)
		}
		for _, c := range part {
			if c < 32 {
				return "", fmt.Errorf("control character in entry name: %q", name)
			}
		}
	}
	return clean, nil
}

// Devices remain special in subdirectories and, on some Windows versions,
// with extensions. Console handles and superscript port digits count too.
func windowsDeviceName(name string) bool {
	base, _, _ := strings.Cut(name, ".")
	base = strings.ToUpper(strings.TrimRight(base, " "))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		switch base[3:] {
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
			return true
		}
	}
	return false
}

// safeMode reduces an archive entry's mode to permission bits only (plus the
// directory bit for directories), stripping symlink, device, pipe, socket and
// setuid/setgid/sticky bits. This is a security measure: a preserved symlink
// mode would make a compliant extractor recreate a link whose (unsanitized)
// target could point outside the extraction root, defeating the name-based
// Zip-Slip defense. Stripped to a plain mode, a symlink entry's body is stored
// as inert regular-file content instead. Group/other-write bits are also
// masked out regardless of what the archive recorded: an attacker-controlled
// mode of e.g. 0777 must never make an extracted file writable by other local
// users on a shared destination directory.
func safeMode(m fs.FileMode, isDir bool) fs.FileMode {
	perm := m.Perm() &^ 0o022
	if isDir {
		return perm | fs.ModeDir
	}
	return perm
}
