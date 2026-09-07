package rarutil

import (
	"fmt"
	"io"

	"github.com/nwaples/rardecode/v2"
)

// entryReader is the slice of *rardecode.ReadCloser Test needs: iterate
// headers and read the current entry's content from the same value. Defining
// it as an interface lets checksum-failure handling be tested without a real
// RAR fixture (the format is proprietary and cannot be generated
// programmatically), matching the fakeHeaders seam list_test.go already uses.
type entryReader interface {
	Next() (*rardecode.FileHeader, error)
	Read(p []byte) (int, error)
}

// testEntries streams every entry to a discard sink through the same
// byte/entry cap enforcement Extract uses (cappedWriter, shared from
// writer.go — no duplicated bomb-defense logic), relying on rr.Next()/Read()
// to surface whatever checksum or read error the archive's own CRC32
// validation returns. sanitize() is not needed here: nothing is ever written
// to a path, so there is no Zip-Slip risk to guard against. onEntry fires
// once per completed entry (files and directories, matching fsEmitter's
// count semantics) so a --test run gets the same progress hook --extract
// already gets via Options.OnEntry — without it a caller relying on that
// hook for a progress bar would see it silently never fire.
func testEntries(rr entryReader, lim limits, onEntry func(string)) error {
	var total int64
	var count int
	bufp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bufp)

	for {
		hdr, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read rar entry: %w", err)
		}
		if lim.maxEntries > 0 && count >= lim.maxEntries {
			return errLimitEntries
		}
		count++
		if hdr.IsDir {
			if onEntry != nil {
				onEntry(hdr.Name)
			}
			continue
		}
		w := &cappedWriter{w: io.Discard, total: &total, limit: lim.maxBytes}
		if _, err := io.CopyBuffer(w, rr, *bufp); err != nil {
			return fmt.Errorf("test rar entry %q: %w", hdr.Name, err)
		}
		if onEntry != nil {
			onEntry(hdr.Name)
		}
	}
	return nil
}

// Test streams every entry of the RAR archive at srcRar to a discard sink and
// returns the first error encountered — a checksum failure, a bomb-cap trip,
// or a read error — or nil once every entry has been fully read. It never
// creates a file or directory. It honors Password and the same
// MaxTotalBytes/MaxEntries caps Extract enforces: a --test run on an
// untrusted archive is still attacker-controlled decompression, just
// discarded instead of written, so skipping the cap here would reopen the
// same decompression-bomb exposure the caps close for Extract.
//
// Known limitation: rardecode/v2's checksum validation is CRC32-only (no
// BLAKE2sp in this version, for either legacy or RAR5 archives), and a wrong
// password on a RAR3/4 archive decrypts to garbage that fails this same
// checksum check — it is not reliably distinguishable from real corruption.
func Test(srcRar string, opts Options) error {
	var ropts []rardecode.Option
	if opts.Password != "" {
		ropts = append(ropts, rardecode.Password(opts.Password))
	}
	rr, err := rardecode.OpenReader(srcRar, ropts...)
	if err != nil {
		return fmt.Errorf("open rar %q: %w", srcRar, err)
	}
	defer rr.Close()
	return testEntries(rr, opts.limits(), opts.OnEntry)
}
