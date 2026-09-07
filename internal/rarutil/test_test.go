package rarutil

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/nwaples/rardecode/v2"
)

// fakeEntryReader feeds a fixed sequence of headers, each paired with its
// entry content, standing in for *rardecode.ReadCloser's combined
// Next()+Read() interface (RAR archives cannot be created programmatically,
// so the read/checksum-surfacing path is exercised through this seam
// instead of a fixture).
type fakeEntryReader struct {
	hdrs    []*rardecode.FileHeader
	content []io.Reader // parallel to hdrs; a nil/short entry means empty content
	err     error       // returned by Next after hdrs are exhausted (defaults to io.EOF)
	i       int
	cur     io.Reader
}

func (f *fakeEntryReader) Next() (*rardecode.FileHeader, error) {
	if f.i >= len(f.hdrs) {
		if f.err != nil {
			return nil, f.err
		}
		return nil, io.EOF
	}
	if f.i < len(f.content) {
		f.cur = f.content[f.i]
	} else {
		f.cur = nil
	}
	h := f.hdrs[f.i]
	f.i++
	return h, nil
}

func (f *fakeEntryReader) Read(p []byte) (int, error) {
	if f.cur == nil {
		return 0, io.EOF
	}
	return f.cur.Read(p)
}

// errReader always fails, simulating rardecode surfacing a checksum mismatch
// mid-stream (real RAR corruption can't be hand-crafted without a fixture).
type errReader struct{ err error }

func (r *errReader) Read(p []byte) (int, error) { return 0, r.err }

func TestTestEntries_ValidArchiveNoError(t *testing.T) {
	rr := &fakeEntryReader{
		hdrs:    []*rardecode.FileHeader{{Name: "dir", IsDir: true}, {Name: "a.txt", UnPackedSize: 5}},
		content: []io.Reader{nil, strings.NewReader("hello")},
	}
	if err := testEntries(rr, limits{}, nil); err != nil {
		t.Fatalf("testEntries: %v", err)
	}
}

func TestTestEntries_CorruptedEntrySurfacesError(t *testing.T) {
	boom := errors.New("crc32 mismatch")
	rr := &fakeEntryReader{
		hdrs:    []*rardecode.FileHeader{{Name: "a.txt", UnPackedSize: 5}},
		content: []io.Reader{&errReader{err: boom}},
	}
	err := testEntries(rr, limits{}, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap %v", err, boom)
	}
}

func TestTestEntries_MaxEntriesEnforced(t *testing.T) {
	hdrs := []*rardecode.FileHeader{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	err := testEntries(&fakeEntryReader{hdrs: hdrs}, limits{maxEntries: 2}, nil)
	if !errors.Is(err, errLimitEntries) {
		t.Fatalf("err = %v, want errLimitEntries", err)
	}
}

func TestTestEntries_MaxBytesEnforced(t *testing.T) {
	rr := &fakeEntryReader{
		hdrs:    []*rardecode.FileHeader{{Name: "a", UnPackedSize: 100}},
		content: []io.Reader{strings.NewReader(strings.Repeat("x", 100))},
	}
	err := testEntries(rr, limits{maxBytes: 10}, nil)
	if !errors.Is(err, errLimitBytes) {
		t.Fatalf("err = %v, want errLimitBytes", err)
	}
}

// TestTestEntries_OnEntryFiresPerCompletedEntry proves Test gives --test the
// same per-entry progress hook Extract already provides via opts.OnEntry —
// without it, a caller passing OnEntry to Test would silently get no
// callbacks at all.
func TestTestEntries_OnEntryFiresPerCompletedEntry(t *testing.T) {
	rr := &fakeEntryReader{
		hdrs:    []*rardecode.FileHeader{{Name: "dir", IsDir: true}, {Name: "a.txt", UnPackedSize: 5}},
		content: []io.Reader{nil, strings.NewReader("hello")},
	}
	var fired []string
	if err := testEntries(rr, limits{}, func(name string) { fired = append(fired, name) }); err != nil {
		t.Fatalf("testEntries: %v", err)
	}
	want := []string{"dir", "a.txt"}
	if len(fired) != len(want) || fired[0] != want[0] || fired[1] != want[1] {
		t.Errorf("onEntry fired for %v, want %v", fired, want)
	}
}

func TestTestEntries_HeaderReadErrorWraps(t *testing.T) {
	boom := errors.New("truncated volume")
	rr := &fakeEntryReader{hdrs: []*rardecode.FileHeader{{Name: "ok"}}, err: boom}
	err := testEntries(rr, limits{}, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap %v", err, boom)
	}
}
