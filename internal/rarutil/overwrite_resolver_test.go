package rarutil

import (
	"errors"
	"io/fs"
	"testing"
)

func TestOverwriteResolver_ReusesSuffixAndPropagatesErrors(t *testing.T) {
	calls := 0
	r := newOverwriteResolver(func(name string) (fs.FileInfo, error) {
		calls++
		if name == "file" {
			return nil, nil
		}
		return nil, fs.ErrNotExist
	})
	for i := 1; i <= 10000; i++ {
		name, skip, err := r.resolve("file", OverwriteRename)
		if err != nil || skip || name != dedupVariant("file", i) {
			t.Fatalf("iteration %d: %s %v %v", i, name, skip, err)
		}
	}
	if calls > 20001 {
		t.Fatalf("quadratic probing: %d stat calls", calls)
	}
	denied := errors.New("permission denied")
	r = newOverwriteResolver(func(name string) (fs.FileInfo, error) {
		if name == "file" {
			return nil, nil
		}
		return nil, denied
	})
	if _, _, err := r.resolve("file", OverwriteRename); !errors.Is(err, denied) {
		t.Fatalf("probe error lost: %v", err)
	}
}
