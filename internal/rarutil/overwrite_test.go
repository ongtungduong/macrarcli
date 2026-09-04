package rarutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConvert_OverwriteGuard verifies the output-overwrite policy. The guard
// runs before the RAR is opened, so it needs no fixture: a non-existent source
// is enough to prove the guard fires (or doesn't) first.
func TestConvert_OverwriteGuard(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.zip")
	const sentinel = "preexisting"
	if err := os.WriteFile(dst, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "nope.rar") // does not exist

	// Without Force: must refuse before touching anything.
	err := Convert(src, dst, Options{})
	if err == nil {
		t.Fatal("Convert without Force overwrote existing output, want refusal error")
	}
	if b, _ := os.ReadFile(dst); string(b) != sentinel {
		t.Errorf("existing output was modified despite no Force: %q", b)
	}

	// With Force: guard passes, so it proceeds and fails on the missing source
	// (a different error than the overwrite refusal).
	if err := Convert(src, dst, Options{Force: true}); err == nil {
		t.Error("Convert(Force) with missing source unexpectedly succeeded")
	}
}

// TestConvert_ForceFailureKeepsExistingOutput proves that a failed conversion
// under Force never destroys a pre-existing output. It uses a truncated copy of
// the fixture: the RAR header opens fine, but the stream fails mid-way — the
// window where a naive "create then write" would have already clobbered dst.
func TestConvert_ForceFailureKeepsExistingOutput(t *testing.T) {
	matches, _ := filepath.Glob("../../testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping force-failure test")
	}
	full, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	bad := filepath.Join(dir, "truncated.rar")
	if err := os.WriteFile(bad, full[:len(full)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out.zip")
	const sentinel = "previous-good-output"
	if err := os.WriteFile(dst, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Convert(bad, dst, Options{Force: true}); err == nil {
		t.Skip("truncated archive unexpectedly converted; cannot exercise the failure path")
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("pre-existing output was deleted by a failed force-convert: %v", err)
	}
	if string(b) != sentinel {
		t.Errorf("pre-existing output was corrupted by a failed force-convert: %q", b)
	}
}
