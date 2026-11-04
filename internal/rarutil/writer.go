package rarutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
)

// errLimitBytes and errLimitEntries report that a conversion exceeded the
// configured decompression-bomb caps. They are distinct sentinels (never
// io.EOF) so a tripped cap propagates as a real error, triggering the
// caller's cleanup, instead of silently truncating the archive.
var (
	errLimitBytes   = errors.New("archive exceeds maximum total uncompressed size")
	errLimitEntries = errors.New("archive exceeds maximum entry count")
)

// limits caps how far an archive may expand. A zero field means that dimension
// is unlimited; caps are opt-in so default conversions behave exactly as before.
type limits struct {
	maxBytes   int64 // total uncompressed bytes across all entries
	maxEntries int   // number of entries (files + directories)
}

// fsEmitter writes into a private, initially empty staging root. Files and
// new directory spellings are created exclusively: the filesystem decides
// whether case or normalization variants alias, without silent overwrites.
type fsEmitter struct {
	root    *os.Root
	rootErr error
	ownRoot bool
	lim     limits
	onEntry func(string)
	origin  map[string]string // final name -> first raw name that produced it
	next    map[string]int    // base name -> next suffix to try (never rescan)
	dirs    map[string]bool   // exact spellings of explicit and implicit dirs
	flat    bool
	sources map[string]string // sanitized source -> raw name, before flattening
	bytes   int64
	count   int
}

func newFsEmitter(root string, lim limits, onEntry func(string)) *fsEmitter {
	r, err := os.OpenRoot(root)
	e := newFsEmitterRoot(r, lim, onEntry)
	e.rootErr, e.ownRoot = err, true
	return e
}

// newFsEmitterRoot borrows a pinned staging root; its caller owns the lifetime.
func newFsEmitterRoot(root *os.Root, lim limits, onEntry func(string)) *fsEmitter {
	return &fsEmitter{root: root, lim: lim, onEntry: onEntry,
		origin: map[string]string{}, next: map[string]int{},
		dirs: map[string]bool{".": true}, sources: map[string]string{}}
}

func (e *fsEmitter) close() error {
	if e.ownRoot && e.root != nil {
		return e.root.Close()
	}
	return nil
}

// checkCount enforces the entry-count cap before an entry is written.
func (e *fsEmitter) checkCount() error {
	if e.rootErr != nil {
		return fmt.Errorf("open staging root: %w", e.rootErr)
	}
	if e.lim.maxEntries > 0 && e.count >= e.lim.maxEntries {
		return errLimitEntries
	}
	return nil
}

// mkdir tracks implicit parents too. A new spelling that already exists on
// disk is an alias (or unexpected entry), not a directory we may silently use.
func (e *fsEmitter) mkdir(name string) error {
	if e.dirs[name] {
		return nil
	}
	if err := e.mkdir(path.Dir(name)); err != nil {
		return err
	}
	if err := e.root.Mkdir(name, 0o755); err != nil {
		return err
	}
	e.dirs[name] = true
	return nil
}

func (e *fsEmitter) fire(name string) {
	if e.onEntry != nil {
		e.onEntry(name)
	}
}

// emitDir creates a directory entry in the staging tree. name is the
// sanitized entry name without a trailing slash. Directories are always
// created with a permissive 0755 regardless of the archive's recorded mode —
// unlike files, a directory must stay writable for the rest of this run to
// populate it, so exact mode restoration is intentionally not attempted here
// (matches the existing codebase's precedent: only file mode/mtime were ever
// asserted by tests).
func (e *fsEmitter) emitDir(raw, name string) error {
	if err := e.checkCount(); err != nil {
		return err
	}
	final, err := e.resolveName(raw, name+"/")
	if err != nil {
		return err
	}
	if err := e.mkdir(strings.TrimSuffix(final, "/")); err != nil {
		return fmt.Errorf("stage dir %q: %w", raw, err)
	}
	e.count++
	e.fire(final)
	return nil
}

// emitFile streams a file entry into the staging tree, enforcing the
// total-bytes cap, then restores its POSIX permission bits and modification
// time so the staged copy is already fully correct before it is ever moved
// into destDir.
func (e *fsEmitter) emitFile(raw, name string, mode fs.FileMode, modTime time.Time, content io.Reader) error {
	if err := e.checkCount(); err != nil {
		return err
	}
	final, err := e.resolveName(raw, name)
	if err != nil {
		return err
	}
	if err := e.mkdir(path.Dir(final)); err != nil {
		return fmt.Errorf("stage dir for %q: %w", raw, err)
	}
	f, err := e.root.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("stage file %q: %w", raw, err)
	}
	// RAR readers implement WriterTo, so io.CopyBuffer's buffer was unused.
	_, copyErr := io.Copy(&cappedWriter{w: f, total: &e.bytes, limit: e.lim.maxBytes}, content)
	if copyErr == nil {
		copyErr = f.Chmod(mode.Perm())
	}
	if copyErr == nil && !modTime.IsZero() {
		copyErr = setFileTimes(f, modTime)
	}
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("stage file %q: %w", raw, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("stage file %q: %w", raw, closeErr)
	}
	e.count++
	e.fire(final)
	return nil
}
