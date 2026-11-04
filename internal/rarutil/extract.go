// Package rarutil performs the core RAR extraction operations: list, extract,
// and test-integrity.
package rarutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwaples/rardecode/v2"
)

// Options tunes an Extract, Test, or List run.
type Options struct {
	// OnEntry, if non-nil, is called once per archive entry (files and
	// directories) for progress reporting. Extract supplies sanitized output
	// names; Test supplies raw archive names. Callers must escape terminal output.
	// Flat extraction omits callbacks for discarded directory entries.
	OnEntry func(name string)
	// OnVerbose, if non-nil, receives extra diagnostics. It may run from
	// multiple goroutines in a batch, so it must be safe for concurrent use.
	OnVerbose func(msg string)
	// Password, if set, decrypts password-protected RAR archives.
	Password string
	// OverwritePolicy controls what happens when a commit-time destination
	// path already exists. Zero value is OverwriteFail (abort).
	OverwritePolicy OverwritePolicy
	// MaxTotalBytes caps the total uncompressed size across all entries; 0
	// (the default) means unlimited. Exceeding it aborts with no output
	// (decompression-bomb defense).
	MaxTotalBytes int64
	// MaxEntries caps the number of archive entries; 0 (the default) means
	// unlimited.
	MaxEntries int
	// MaxDictionaryBytes caps each decoder's dictionary allocation. Zero uses
	// DefaultMaxDictionaryBytes (256 MiB); a positive value overrides it.
	// Negative values are invalid. This is separate from MaxTotalBytes.
	MaxDictionaryBytes int64
	// Flat, when true, discards the archive's directory structure: every file
	// is written directly under destDir by its base name, and no directory
	// entries are staged. Name collisions across different source directories
	// are resolved by fsEmitter's existing dedup-variant logic, the same as
	// any other post-sanitize name collision.
	Flat bool
}

func (o Options) limits() limits {
	return limits{maxBytes: o.MaxTotalBytes, maxEntries: o.MaxEntries}
}

func (o Options) vlog(format string, a ...any) {
	if o.OnVerbose != nil {
		o.OnVerbose(fmt.Sprintf(format, a...))
	}
}

// Extract reads the RAR archive at srcRar and writes its contents as real
// files under destDir, preserving the sanitized relative path structure and
// restoring each file's POSIX permissions and modification time. Entries are
// staged in a private temporary directory first and committed into destDir
// only once the whole archive has decoded successfully. Commit failures restore
// overwritten files; if restoration itself fails, the error identifies retained
// backups for recovery. This is not a crash-durable filesystem transaction.
// It returns the archive-relative paths of any entries skipped under
// OverwriteSkip (nil on full success with no skips).
func Extract(srcRar, destDir string, opts Options) (skipped []string, err error) {
	start := time.Now()
	base := filepath.Base(srcRar)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create destination %q: %w", destDir, err)
	}
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	staging, err := makeRootTempDir(root, ".macrarcli-staging-")
	if err != nil {
		return nil, err
	}
	stageRoot, err := root.OpenRoot(staging)
	if err != nil {
		return nil, err
	}
	defer stageRoot.Close()
	defer func() {
		if cleanupErr := cleanupStaging(root, staging, stageRoot); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("cleanup staging %q: %w", filepath.Join(destDir, staging), cleanupErr))
		}
	}()
	if err := extractToStagingRoot(srcRar, stageRoot, opts); err != nil {
		return nil, err
	}
	skipped, err = commitPinned(root, staging, stageRoot, opts)
	if err != nil {
		return nil, err
	}
	opts.vlog("%s: extracted in %s", base, time.Since(start).Round(time.Millisecond))
	return skipped, nil
}

// extractToStaging performs the pure-Go RAR decode, streaming every entry
// into stagingDir (never destDir directly).
func extractToStaging(srcRar, stagingDir string, opts Options) error {
	stageRoot, err := os.OpenRoot(stagingDir)
	if err != nil {
		return fmt.Errorf("open staging directory: %w", err)
	}
	defer stageRoot.Close()
	return extractToStagingRoot(srcRar, stageRoot, opts)
}

// extractToStagingRoot decodes into a staging directory pinned by the caller.
// The caller owns stageRoot and must keep it open throughout decoding.
func extractToStagingRoot(srcRar string, stageRoot *os.Root, opts Options) error {
	ropts, err := opts.readerOptions()
	if err != nil {
		return err
	}
	rr, err := rardecode.OpenReader(srcRar, ropts...)
	if err != nil {
		return fmt.Errorf("open rar %q: %w", srcRar, err)
	}
	defer rr.Close()

	em := newFsEmitterRoot(stageRoot, opts.limits(), opts.OnEntry)
	em.flat = opts.Flat
	for {
		hdr, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read rar entry: %w", err)
		}

		// Reject any entry that would escape the archive root before it can
		// be written anywhere. rardecode reports names with '/' separators.
		name, err := sanitize(hdr.Name)
		if err != nil {
			return fmt.Errorf("unsafe rar entry %q: %w", hdr.Name, err)
		}

		if hdr.IsDir {
			// A flat extraction discards structure entirely, so an empty
			// directory entry has nowhere meaningful to land.
			if opts.Flat {
				if err := em.checkCount(); err != nil {
					return err
				}
				em.count++
				continue
			}
			if err := em.emitDir(hdr.Name, strings.TrimRight(name, "/")); err != nil {
				return err
			}
			continue
		}
		if opts.Flat {
			name = flattenName(name)
		}
		mode := safeMode(hdr.Mode(), hdr.IsDir)
		if err := em.emitFile(hdr.Name, name, mode, hdr.ModificationTime, rr); err != nil {
			return err
		}
	}
	return nil
}

// flattenName reduces a sanitized, '/'-separated entry name to its base
// component for Options.Flat extraction. name is already sanitize()d, so no
// further traversal check is needed here.
func flattenName(name string) string {
	return path.Base(name)
}
