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
// It rejects: empty names, absolute paths (unix '/' or Windows drive letters),
// and any path that — after resolving '.'/'..' segments — points at or above
// the archive root.
func sanitize(name string) (string, error) {
	// Archives may use Windows '\' separators; normalize before reasoning.
	s := strings.ReplaceAll(name, `\`, "/")
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("invalid empty entry name")
	}
	// Windows drive-letter absolute path, e.g. "C:/..." or "C:".
	if len(s) >= 2 && s[1] == ':' {
		return "", fmt.Errorf("absolute path not allowed: %q", name)
	}
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("absolute path not allowed: %q", name)
	}

	clean := path.Clean(s)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes archive root: %q", name)
	}
	return clean, nil
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
