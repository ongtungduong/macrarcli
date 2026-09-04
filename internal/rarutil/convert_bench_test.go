package rarutil

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// plainReader hides any WriterTo a source might implement so io.CopyBuffer
// actually exercises the pooled copy buffer — mirroring a real os.File / decoder
// stream (which has no WriterTo) rather than an in-memory bytes.Reader (which
// does, and would bypass the buffer entirely).
type plainReader struct{ r io.Reader }

func (p plainReader) Read(b []byte) (int, error) { return p.r.Read(b) }

// zeroReader yields n zero bytes without allocating a backing slice, so a
// large-entry benchmark isn't dominated by source setup.
type zeroReader struct{ remaining int64 }

func (z *zeroReader) Read(b []byte) (int, error) {
	if z.remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(b))
	if n > z.remaining {
		n = z.remaining
	}
	for i := int64(0); i < n; i++ {
		b[i] = 0
	}
	z.remaining -= n
	return int(n), nil
}

// BenchmarkEmitFile_LargeEntry measures the streaming copy hot path: one large
// entry pushed through the emitter into a Store (no-deflate) ZIP written to a
// real temp file, so per-write syscall cost (where the 512 KB pooled buffer
// beats io.Copy's default 32 KB) is reflected, not masked by io.Discard.
func BenchmarkEmitFile_LargeEntry(b *testing.B) {
	const size = 64 << 20 // 64 MB
	out, err := os.Create(filepath.Join(b.TempDir(), "bench.zip"))
	if err != nil {
		b.Fatal(err)
	}
	defer out.Close()

	b.SetBytes(size)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := out.Seek(0, io.SeekStart); err != nil {
			b.Fatal(err)
		}
		zw := zip.NewWriter(out)
		em := newZipEmitter(zw, limits{}, nil)
		fh := &zip.FileHeader{Method: zip.Store}
		src := plainReader{&zeroReader{remaining: size}}
		if err := em.emitFile("big.bin", "big.bin", fh, src); err != nil {
			b.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerify_LargeMap measures the --verify reopen+scan over an archive
// with many entries (the verify accounting cost, not conversion).
func BenchmarkVerify_LargeMap(b *testing.B) {
	const entries = 5000
	dir := b.TempDir()
	zipPath := filepath.Join(dir, "many.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		b.Fatal(err)
	}
	zw := zip.NewWriter(f)
	expected := make(map[string]int64, entries)
	for i := 0; i < entries; i++ {
		name := filepath.Join("d", "f"+itoa(i)+".txt")
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			b.Fatal(err)
		}
		expected[name] = 1
	}
	if err := zw.Close(); err != nil {
		b.Fatal(err)
	}
	f.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := verify(zipPath, expected); err != nil {
			b.Fatal(err)
		}
	}
}

// itoa is a tiny dependency-free int->string for benchmark entry names.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// BenchmarkConvertNative converts a real fixture end-to-end. Skips when no
// testdata/*.rar is present (RAR cannot be generated programmatically).
func BenchmarkConvertNative(b *testing.B) {
	matches, _ := filepath.Glob("../../testdata/*.rar")
	if len(matches) == 0 {
		b.Skip("no testdata/*.rar fixture present; skipping native convert benchmark")
	}
	src := matches[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst := filepath.Join(b.TempDir(), "out.zip")
		if err := Convert(src, dst, Options{Force: true}); err != nil {
			b.Fatal(err)
		}
	}
}
