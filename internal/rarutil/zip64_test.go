package rarutil

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// TestConvert_ZIP64 proves entries past the 4 GiB ZIP64 boundary round-trip with
// correct size accounting. It targets the fallback re-pack path (writeZipFromDir)
// because the native path takes a concrete *rardecode.ReadCloser that cannot be
// fed a synthetic >4 GiB stream, and a real >4 GiB RAR cannot be created here —
// that native-path ZIP64 coverage gap is intentional and noted (RT-9).
//
// The source is a sparse file (Truncate, no real disk used) and the entry is
// Deflate-compressed (zeros shrink to almost nothing), so only ~4 GiB of read +
// deflate work happens — gated behind -short for CI time budgets.
func TestConvert_ZIP64(t *testing.T) {
	if testing.Short() {
		t.Skip("ZIP64 >4GiB round-trip skipped in -short mode")
	}

	const size = int64(4)<<30 + (64 << 10) // just past the 4 GiB ZIP64 threshold

	srcDir := t.TempDir()
	big := filepath.Join(srcDir, "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil { // sparse: no blocks allocated
		f.Close()
		t.Skipf("cannot create a %d-byte sparse file on this filesystem: %v", size, err)
	}
	f.Close()

	out, err := os.CreateTemp(t.TempDir(), "zip64-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	zipPath := out.Name()

	// writeZipFromDir closes out on success.
	expected, err := writeZipFromDir(out, srcDir, Options{})
	if err != nil {
		t.Fatalf("writeZipFromDir: %v", err)
	}
	if expected["big.bin"] != size {
		t.Fatalf("expected size accounting = %d, want %d", expected["big.bin"], size)
	}

	// --verify must accept a >4 GiB entry.
	if err := verify(zipPath, expected); err != nil {
		t.Fatalf("verify of ZIP64 archive: %v", err)
	}

	// Confirm the reader sees the full 64-bit size, not a 32-bit truncation.
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip64 result: %v", err)
	}
	defer zr.Close()
	var found bool
	for _, zf := range zr.File {
		if zf.Name == "big.bin" {
			found = true
			if zf.UncompressedSize64 != uint64(size) {
				t.Errorf("UncompressedSize64 = %d, want %d (ZIP64 size lost)", zf.UncompressedSize64, size)
			}
		}
	}
	if !found {
		t.Fatal("big.bin entry missing from ZIP64 result")
	}
}
