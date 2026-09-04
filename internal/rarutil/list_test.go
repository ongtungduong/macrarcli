package rarutil

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/nwaples/rardecode/v2"
)

// fakeHeaders feeds a fixed sequence of headers (then io.EOF) to listEntries,
// standing in for a real *rardecode.ReadCloser. RAR archives cannot be created
// programmatically (the format is proprietary; rardecode is decode-only), so the
// read-only iteration core is exercised through this seam instead of a fixture.
type fakeHeaders struct {
	hdrs []*rardecode.FileHeader
	err  error // returned after the headers are exhausted (defaults to io.EOF)
	i    int
}

func (f *fakeHeaders) Next() (*rardecode.FileHeader, error) {
	if f.i >= len(f.hdrs) {
		if f.err != nil {
			return nil, f.err
		}
		return nil, io.EOF
	}
	h := f.hdrs[f.i]
	f.i++
	return h, nil
}

func TestListEntries_MapsFields(t *testing.T) {
	mtime := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	rr := &fakeHeaders{hdrs: []*rardecode.FileHeader{
		{Name: "dir", IsDir: true, ModificationTime: mtime},
		{Name: "dir/file.txt", UnPackedSize: 42, ModificationTime: mtime},
		{Name: "streamed.bin", UnPackedSize: 99, UnKnownSize: true},
	}}

	got, err := listEntries(rr, 0)
	if err != nil {
		t.Fatalf("listEntries: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("entry count = %d, want 3", len(got))
	}

	if !got[0].IsDir || got[0].Name != "dir" || got[0].Size != 0 {
		t.Errorf("dir entry = %+v", got[0])
	}
	if got[1].Name != "dir/file.txt" || got[1].Size != 42 || !got[1].Modified.Equal(mtime) {
		t.Errorf("file entry = %+v", got[1])
	}
	// An unknown-size entry maps to -1 rather than a misleading concrete number.
	if got[2].Size != -1 {
		t.Errorf("unknown-size entry Size = %d, want -1", got[2].Size)
	}
}

// TestListEntries_EntryCap proves the read-only lister inherits the entry-count
// cap: it never accumulates more than maxEntries entries (no OOM on a crafted
// high-entry archive) and reports the same sentinel a conversion would.
func TestListEntries_EntryCap(t *testing.T) {
	hdrs := make([]*rardecode.FileHeader, 1000)
	for i := range hdrs {
		hdrs[i] = &rardecode.FileHeader{Name: "f"}
	}
	rr := &fakeHeaders{hdrs: hdrs}

	got, err := listEntries(rr, 10)
	if !errors.Is(err, errLimitEntries) {
		t.Fatalf("err = %v, want errLimitEntries", err)
	}
	if len(got) > 10 {
		t.Errorf("accumulated %d entries past the cap of 10 (unbounded allocation)", len(got))
	}
}

// TestListEntries_NoCap confirms maxEntries==0 means unlimited (default behavior).
func TestListEntries_NoCap(t *testing.T) {
	hdrs := make([]*rardecode.FileHeader, 50)
	for i := range hdrs {
		hdrs[i] = &rardecode.FileHeader{Name: "f"}
	}
	got, err := listEntries(&fakeHeaders{hdrs: hdrs}, 0)
	if err != nil {
		t.Fatalf("listEntries: %v", err)
	}
	if len(got) != 50 {
		t.Errorf("entry count = %d, want 50 (no cap)", len(got))
	}
}

// TestListEntries_ReadError surfaces a mid-stream decode error instead of
// silently returning a short list.
func TestListEntries_ReadError(t *testing.T) {
	boom := errors.New("corrupt header")
	rr := &fakeHeaders{
		hdrs: []*rardecode.FileHeader{{Name: "ok"}},
		err:  boom,
	}
	_, err := listEntries(rr, 0)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap %v", err, boom)
	}
}
