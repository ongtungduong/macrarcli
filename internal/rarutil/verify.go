package rarutil

import (
	"archive/zip"
	"fmt"
	"io"
	"strings"
)

// verify reopens the ZIP at path and confirms it contains exactly the expected
// entries (keyed by ZIP name, directories with a trailing slash), that every
// file entry's uncompressed size matches, AND that every file entry's content
// decompresses with a matching CRC. Reading each entry to EOF makes the stdlib
// reader validate the stored CRC32, so a same-size-but-corrupted entry (which a
// size-only check would miss) is caught. It is the self-check behind --verify.
func verify(path string, expected map[string]int64) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("verify: reopen %q: %w", path, err)
	}
	defer zr.Close()

	if len(zr.File) != len(expected) {
		return fmt.Errorf("verify: entry count mismatch: wrote %d, found %d", len(expected), len(zr.File))
	}

	for _, f := range zr.File {
		wantSize, ok := expected[f.Name]
		if !ok {
			return fmt.Errorf("verify: unexpected entry %q in output", f.Name)
		}
		if int64(f.UncompressedSize64) != wantSize {
			return fmt.Errorf("verify: size mismatch for %q: wrote %d, found %d",
				f.Name, wantSize, f.UncompressedSize64)
		}
		if strings.HasSuffix(f.Name, "/") {
			continue // directory entries carry no content/CRC
		}
		if err := checkEntryCRC(f); err != nil {
			return err
		}
	}
	return nil
}

// checkEntryCRC reads one file entry to EOF, forcing the stdlib zip reader to
// validate its CRC32; a mismatch surfaces as an error from Close (ErrChecksum).
func checkEntryCRC(f *zip.File) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("verify: open %q: %w", f.Name, err)
	}
	// A CRC mismatch surfaces from the final Read (as ErrChecksum) or from Close.
	_, copyErr := io.Copy(io.Discard, rc)
	closeErr := rc.Close()
	if copyErr != nil {
		return fmt.Errorf("verify: content check failed for %q: %w", f.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("verify: checksum failed for %q: %w", f.Name, closeErr)
	}
	return nil
}
