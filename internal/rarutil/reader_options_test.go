package rarutil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwaples/rardecode/v2"
)

// Minimal RAR5 headers exercise the actual decoder without external tools or
// optional fixtures. Compressed entries are used only for dictionary checks at
// Next, before their payload is decoded; stored entries carry real contents.
type readerFixtureEntry struct {
	name        string
	data        string
	isDir       bool
	unknownSize bool
	compression uint64
}

func readerFixture(entries ...readerFixtureEntry) []byte {
	archive := []byte("Rar!\x1a\x07\x01\x00")
	block := func(header, data []byte) {
		header = append(binary.AppendUvarint(nil, uint64(len(header))), header...)
		archive = binary.LittleEndian.AppendUint32(archive, crc32.ChecksumIEEE(header))
		archive = append(archive, header...)
		archive = append(archive, data...)
	}
	block([]byte{1, 0, 0}, nil) // archive header
	for _, e := range entries {
		h := binary.AppendUvarint([]byte{2, 2}, uint64(len(e.data))) // file, has data
		var flags uint64
		if e.isDir {
			flags |= 1
		}
		if e.unknownSize {
			flags |= 8
		}
		h = binary.AppendUvarint(h, flags)
		h = binary.AppendUvarint(h, uint64(len(e.data)))
		h = binary.AppendUvarint(h, 0o644)
		h = binary.AppendUvarint(h, e.compression)
		h = binary.AppendUvarint(h, 1) // Unix host
		h = binary.AppendUvarint(h, uint64(len(e.name)))
		h = append(h, e.name...)
		block(h, []byte(e.data))
	}
	block([]byte{5, 0, 0}, nil) // end of archive
	return archive
}

func writeReaderFixture(t *testing.T, entries ...readerFixtureEntry) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "input.rar")
	if err := os.WriteFile(src, readerFixture(entries...), 0o600); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestReaderOptionsDictionaryLimit(t *testing.T) {
	if DefaultMaxDictionaryBytes != 256<<20 {
		t.Fatalf("default dictionary limit = %d, want 256 MiB", DefaultMaxDictionaryBytes)
	}
	for _, tc := range []struct {
		name string
		max  int64
		exp  uint64 // RAR5 dictionary = 128 KiB << exp
		fail bool
	}{
		{"zero uses bounded default", 0, 12, true}, // 512 MiB; rejected before allocation
		{"explicit default", DefaultMaxDictionaryBytes, 12, true},
		{"positive override rejects larger", 128 << 10, 1, true},
		{"positive override accepts exact", 256 << 10, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := (Options{MaxDictionaryBytes: tc.max}).readerOptions()
			if err != nil {
				t.Fatal(err)
			}
			rr, err := rardecode.NewReader(bytes.NewReader(readerFixture(readerFixtureEntry{
				name: "compressed", unknownSize: true, compression: 1<<7 | tc.exp<<10,
			})), opts...)
			if err != nil {
				t.Fatal(err)
			}
			_, err = rr.Next()
			if tc.fail && !errors.Is(err, rardecode.ErrDictionaryTooLarge) {
				t.Fatalf("Next = %v, want ErrDictionaryTooLarge", err)
			}
			if !tc.fail && err != nil {
				t.Fatalf("Next at exact dictionary limit: %v", err)
			}
		})
	}
}

func TestBackendReaderOptions(t *testing.T) {
	largeDictionary := writeReaderFixture(t, readerFixtureEntry{
		name: "compressed", unknownSize: true, compression: 1<<7 | 12<<10,
	})
	for name, run := range map[string]func(string, Options) error{
		"List": func(src string, opts Options) error { _, err := List(src, opts); return err },
		"Test": Test,
		"Extract": func(src string, opts Options) error {
			_, err := Extract(src, t.TempDir(), opts)
			return err
		},
		"extractToStaging": func(src string, opts Options) error {
			return extractToStaging(src, t.TempDir(), opts)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(largeDictionary, Options{}); !errors.Is(err, rardecode.ErrDictionaryTooLarge) {
				t.Fatalf("zero Options: %v, want ErrDictionaryTooLarge", err)
			}
			if err := run("missing.rar", Options{MaxDictionaryBytes: -1}); err == nil || !strings.Contains(err.Error(), "MaxDictionaryBytes") {
				t.Fatalf("negative dictionary limit = %v, want validation before archive open", err)
			}
		})
	}
}

func TestFlatDirectoriesCountWithoutProgress(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []readerFixtureEntry
		cap     int
		fail    bool
		fired   int
	}{
		{"directories exceed cap", []readerFixtureEntry{{name: "a", isDir: true}, {name: "b", isDir: true}}, 1, true, 0},
		{"directory consumes file budget", []readerFixtureEntry{{name: "a", isDir: true}, {name: "a/file", data: "x"}}, 1, true, 0},
		{"file consumes directory budget", []readerFixtureEntry{{name: "file", data: "x"}, {name: "a", isDir: true}}, 1, true, 1},
		{"exact cap", []readerFixtureEntry{{name: "a", isDir: true}, {name: "a/file", data: "x"}}, 2, false, 1},
		{"unlimited", []readerFixtureEntry{{name: "a", isDir: true}, {name: "a/file", data: "x"}}, 0, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, dest := writeReaderFixture(t, tc.entries...), t.TempDir()
			var fired []string
			_, err := Extract(src, dest, Options{Flat: true, MaxEntries: tc.cap, OnEntry: func(name string) { fired = append(fired, name) }})
			if tc.fail && !errors.Is(err, errLimitEntries) || !tc.fail && err != nil {
				t.Fatalf("Extract = %v, want entry cap failure %v", err, tc.fail)
			}
			if len(fired) != tc.fired {
				t.Fatalf("progress callbacks = %v, want %d files only", fired, tc.fired)
			}
			entries, err := os.ReadDir(dest)
			if err != nil {
				t.Fatal(err)
			}
			if tc.fail && len(entries) != 0 {
				t.Fatalf("failed extraction left output: %v", entries)
			}
			for _, e := range entries {
				if e.IsDir() {
					t.Fatalf("flat extraction created directory %q", e.Name())
				}
			}
		})
	}
}
