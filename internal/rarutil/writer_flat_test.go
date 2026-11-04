package rarutil

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFsEmitter_FlatDeduplicatesDistinctSources(t *testing.T) {
	root := t.TempDir()
	e := newTestFsEmitter(t, root, limits{}, nil)
	e.flat = true
	for _, tc := range []struct{ raw, final string }{
		{"a/file.txt", "file.txt"},
		{"b/file (1).txt", "file (1).txt"},
		{"b/file.txt", "file (2).txt"},
		{"a/file.txt", "file (3).txt"},
		{"c/file (2).txt", "file (2) (1).txt"},
	} {
		if err := e.emitFile(tc.raw, flattenName(tc.raw), 0o600, time.Time{}, strings.NewReader(tc.raw)); err != nil {
			t.Fatal(err)
		}
		assertStagedContent(t, filepath.Join(root, tc.final), tc.raw)
	}
}

func TestFsEmitter_FlatRejectsSanitizedSourceCollision(t *testing.T) {
	for _, raw := range []string{"a/./file", `a\file`, "a/sub/../file"} {
		t.Run(raw, func(t *testing.T) {
			root := t.TempDir()
			e := newTestFsEmitter(t, root, limits{}, nil)
			e.flat = true
			if err := e.emitFile("a/file", "file", 0o600, time.Time{}, strings.NewReader("original")); err != nil {
				t.Fatal(err)
			}
			if err := e.emitFile(raw, "file", 0o600, time.Time{}, strings.NewReader("bad")); err == nil {
				t.Fatal("flattening concealed a sanitize collision")
			}
			assertStagedContent(t, filepath.Join(root, "file"), "original")
		})
	}
}
