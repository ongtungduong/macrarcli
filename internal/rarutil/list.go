package rarutil

import (
	"fmt"
	"io"
	"time"

	"github.com/nwaples/rardecode/v2"
)

// EntryInfo describes one archive entry for a read-only listing (--list). Names
// use '/' separators exactly as the archive reports them; this is a preview of
// the archive's own contents, not the sanitized ZIP names a conversion produces.
type EntryInfo struct {
	Name       string    // entry name, '/'-separated
	Size       int64     // unpacked size in bytes; -1 when the archive omits it
	PackedSize int64     // compressed size on disk, as recorded in the archive
	Encrypted  bool      // this entry's contents are password-protected
	Modified   time.Time // zero when the archive recorded no modification time
	IsDir      bool
}

// ListResult reports one archive's List outcome for a batched preview
// (see ListBatch): its entries, or the error that prevented reading it.
type ListResult struct {
	Src     string
	Entries []EntryInfo
	Err     error
}

// headerReader is the minimal slice of *rardecode.ReadCloser the lister needs:
// the per-entry iteration core, shared in spirit with Convert. Defining it as an
// interface lets the cap and field-mapping logic be tested without a RAR fixture
// (the format is proprietary and cannot be generated programmatically).
type headerReader interface {
	Next() (*rardecode.FileHeader, error)
}

// listEntries walks every header read-only and returns one EntryInfo per entry.
// The entry-count cap (maxEntries; 0 = unlimited) bounds allocation here in the
// iteration core — not in the write emitter — so a crafted high-entry archive
// cannot OOM a preview. On the cap trip it returns the bounded slice gathered so
// far alongside errLimitEntries (the same sentinel a conversion reports), giving
// callers a usable partial preview while still signaling the limit was hit.
func listEntries(rr headerReader, maxEntries int) ([]EntryInfo, error) {
	var entries []EntryInfo
	for {
		hdr, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return entries, fmt.Errorf("read rar entry: %w", err)
		}
		if maxEntries > 0 && len(entries) >= maxEntries {
			return entries, errLimitEntries
		}

		size := hdr.UnPackedSize
		if hdr.UnKnownSize {
			size = -1 // a streamed entry has no size until extracted; don't fabricate one
		}
		entries = append(entries, EntryInfo{
			Name:       hdr.Name,
			Size:       size,
			PackedSize: hdr.PackedSize,
			Encrypted:  hdr.Encrypted,
			Modified:   hdr.ModificationTime,
			IsDir:      hdr.IsDir,
		})
	}
	return entries, nil
}

// List opens srcRar read-only and returns its entries without writing anything.
// It honors Password (for encrypted headers) and MaxEntries. Listing uses the
// pure-Go decoder only; there is no fallback shell-out (a preview never extracts).
func List(srcRar string, opts Options) ([]EntryInfo, error) {
	var ropts []rardecode.Option
	if opts.Password != "" {
		ropts = append(ropts, rardecode.Password(opts.Password))
	}
	rr, err := rardecode.OpenReader(srcRar, ropts...)
	if err != nil {
		return nil, fmt.Errorf("open rar %q: %w", srcRar, err)
	}
	defer rr.Close()
	return listEntries(rr, opts.MaxEntries)
}
