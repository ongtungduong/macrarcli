package rarutil

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// publishFile atomically claims a vacant destination. A hard link avoids copying
// staged bytes on the usual same-filesystem path. Filesystems without hard links
// use an exclusive open; its identity is returned even on failure for rollback.
func publishFile(root *os.Root, src, dst string) (fs.FileInfo, error) {
	return publishFrom(root, src, root, dst)
}

func publishFrom(source *os.Root, src string, destination *os.Root, dst string) (fs.FileInfo, error) {
	from, err := source.OpenRoot(filepath.Dir(src))
	if err != nil {
		return nil, err
	}
	defer from.Close()
	to, err := destination.OpenRoot(filepath.Dir(dst))
	if err != nil {
		return nil, err
	}
	defer to.Close()
	src, dst = filepath.Base(src), filepath.Base(dst)
	info, err := from.Lstat(src)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("publication source is not a regular file")
	}
	if err := linkBetweenRoots(from, src, to, dst); err == nil {
		return info, nil
	} else if errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	in, err := from.Open(src)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return copyExclusive(in, info, to, dst)
}

// Return the created file's identity even when copying fails, so rollback can
// remove precisely our partial output and restore the original backup.
func copyExclusive(in io.Reader, info fs.FileInfo, to *os.Root, dst string) (fs.FileInfo, error) {
	out, err := to.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	identity, err := out.Stat()
	if err != nil {
		out.Close()
		return nil, err
	}
	_, copyErr := io.Copy(out, in)
	if copyErr == nil {
		copyErr = out.Chmod(info.Mode().Perm())
	}
	if copyErr == nil {
		copyErr = setFileTimes(out, info.ModTime())
	}
	return identity, errors.Join(copyErr, out.Close())
}
