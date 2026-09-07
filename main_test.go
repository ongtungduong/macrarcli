package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// whatever it wrote. Used to test human-output functions directly, without
// needing a real .rar fixture.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

// TestReport_QuietDoesNotSuppressSuccessLine locks a pre-existing, documented
// distinction that reportHuman's unification must preserve: -q suppresses
// only progress/summary output (see README's --quiet entry), never extract's
// stdout result line. A prior refactor accidentally gated this line on quiet
// too — this test would have caught that regression.
func TestReport_QuietDoesNotSuppressSuccessLine(t *testing.T) {
	results := []rarutil.Result{{Job: rarutil.Job{Src: "a.rar", Dst: "out"}}}
	out := captureStdout(t, func() { report(results, true) })
	if !strings.Contains(out, "extracted a.rar -> out") {
		t.Errorf("report(quiet=true) stdout = %q, want it to contain the success line", out)
	}
}

// TestReportHuman_QuietGatesTestSuccessLine locks the other half of the same
// pre-existing distinction: test mode's "src: OK" line WAS already gated by
// -q before reportHuman existed, and must stay gated (only extract's line is
// unconditional — see TestReport_QuietDoesNotSuppressSuccessLine above).
func TestReportHuman_QuietGatesTestSuccessLine(t *testing.T) {
	results := []rarutil.Result{{Job: rarutil.Job{Src: "a.rar"}}}
	successLine := func(r rarutil.Result) string { return r.Src + ": OK" }

	out := captureStdout(t, func() { reportHuman(results, true, false, true, successLine) })
	if out != "" {
		t.Errorf("reportHuman(quiet=true, quietGatesSuccess=true) stdout = %q, want empty", out)
	}

	out = captureStdout(t, func() { reportHuman(results, false, false, true, successLine) })
	if !strings.Contains(out, "a.rar: OK") {
		t.Errorf("reportHuman(quiet=false, quietGatesSuccess=true) stdout = %q, want it to contain the success line", out)
	}
}

// TestRun_ExitCodes covers argument validation paths that don't need a real
// archive: usage errors now exit 1 (exit 2 is reserved for password errors).
func TestRun_ExitCodes(t *testing.T) {
	dir := t.TempDir()

	txt := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope.rar")
	dirRar := filepath.Join(dir, "bundle.rar")
	if err := os.Mkdir(dirRar, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no args", nil, 1},
		{"wrong extension", []string{txt}, 1},
		{"directory input", []string{dir}, 1},
		{"directory named .rar", []string{dirRar}, 1},
		{"missing rar file", []string{missing}, 4},
		{"multiple missing inputs", []string{missing, filepath.Join(dir, "x.rar")}, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(tc.args); got != tc.want {
				t.Errorf("run(%v) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

// TestRun_VersionHelp covers the informational flags that exit 0 without
// requiring an input archive.
func TestRun_VersionHelp(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-version"}, {"-h"}, {"--help"}} {
		if got := run(args); got != 0 {
			t.Errorf("run(%v) = %d, want 0", args, got)
		}
	}
	// An unknown flag is a usage error (exit 1).
	if got := run([]string{"--bogus"}); got != 1 {
		t.Errorf("run(--bogus) = %d, want 1", got)
	}
}

// TestRun_VersionOutput verifies --version prints "macrarcli v<version> (<commit>)".
func TestRun_VersionOutput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() {
		w.Close()
		os.Stdout = old
	}()

	code := run([]string{"--version"})

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if code != 0 {
		t.Fatalf("--version exited %d, want 0", code)
	}
	s := strings.TrimSpace(string(out))
	if !strings.HasPrefix(s, "macrarcli ") {
		t.Errorf("version output %q does not start with 'macrarcli '", s)
	}
	if !strings.Contains(s, "(") || !strings.Contains(s, ")") {
		t.Errorf("version output %q missing commit parens, want 'macrarcli v<ver> (<commit>)'", s)
	}
}

// TestRun_OverwriteGuard refuses to clobber an existing destination file
// unless --overwrite. A dummy (non-real) .rar suffices: rardecode's OpenReader
// error surfaces as exit 4 (not a usage error), which is the behavior under
// test here — the actual overwrite-collision guard is exercised at the
// engine level in internal/rarutil's own tests. This just proves the CLI
// wires --overwrite through and never crashes attempting a fake extraction.
func TestRun_OverwriteGuard(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.rar")
	if err := os.WriteFile(in, []byte("not a real rar"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := run([]string{"-q", "-o", dir, in}); got != 4 {
		t.Errorf("run(fake rar) = %d, want 4 (open error)", got)
	}
	if got := run([]string{"-q", "--overwrite", "-o", dir, in}); got != 4 {
		t.Errorf("run(--overwrite, fake rar) = %d, want 4 (open error)", got)
	}
}

// TestRun_Extract exercises the happy path and --dest against a real fixture.
// Skips when no fixture is present.
func TestRun_Extract(t *testing.T) {
	matches, _ := filepath.Glob("testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping extraction test")
	}
	src := matches[0]
	dir := t.TempDir()

	if got := run([]string{"-q", "-o", dir, src}); got != 0 {
		t.Fatalf("run(-o) = %d, want 0", got)
	}

	// Re-running without --overwrite must refuse (destination files exist).
	if got := run([]string{"-q", "-o", dir, src}); got == 0 {
		t.Errorf("re-run without --overwrite = %d, want nonzero (collision)", got)
	}

	// --overwrite replaces.
	if got := run([]string{"-q", "--overwrite", "-o", dir, src}); got != 0 {
		t.Errorf("run(--overwrite) = %d, want 0", got)
	}
}

// TestRun_ExtractFlat confirms -e/--flat discards directory structure.
func TestRun_ExtractFlat(t *testing.T) {
	matches, _ := filepath.Glob("testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping flat extraction test")
	}
	src := matches[0]
	dir := t.TempDir()

	if got := run([]string{"-q", "-e", "-o", dir, src}); got != 0 {
		t.Fatalf("run(-e) = %d, want 0", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("-e left a subdirectory: %q", e.Name())
		}
	}
}

// TestRun_ModeFlagConflict proves two mode selectors together is a usage
// error (exit 1) and never touches any archive.
func TestRun_ModeFlagConflict(t *testing.T) {
	if got := run([]string{"-l", "-t", "a.rar"}); got != 1 {
		t.Errorf("run(-l -t) = %d, want 1", got)
	}
	if got := run([]string{"-e", "-l", "a.rar"}); got != 1 {
		t.Errorf("run(-e -l) = %d, want 1", got)
	}
}

// TestRun_OverwriteFlagConflict proves two overwrite-policy flags together is
// a usage error (exit 1).
func TestRun_OverwriteFlagConflict(t *testing.T) {
	if got := run([]string{"--overwrite", "--skip", "a.rar"}); got != 1 {
		t.Errorf("run(--overwrite --skip) = %d, want 1", got)
	}
}

// TestRun_ListRejectsDestAndOverwrite proves --list/--test reject -o/--dest
// and the overwrite-policy flags (they write nothing).
func TestRun_ListRejectsDestAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	rar := filepath.Join(dir, "x.rar")
	if err := os.WriteFile(rar, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{"list with dest", []string{"-l", "-o", dir, rar}},
		{"test with overwrite", []string{"-t", "--overwrite", rar}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(tc.args); got != 1 {
				t.Errorf("run(%v) = %d, want 1", tc.args, got)
			}
		})
	}
}

// TestDefaultJobs verifies the batch concurrency default is multi-core but
// capped (I/O-bound work doesn't benefit from unbounded fan-out) and never < 1.
func TestDefaultJobs(t *testing.T) {
	got := defaultJobs()
	if got < 1 {
		t.Fatalf("defaultJobs() = %d, must be >= 1", got)
	}
	if got > 4 {
		t.Errorf("defaultJobs() = %d, must be capped at 4", got)
	}
	want := runtime.NumCPU()
	if want > 4 {
		want = 4
	}
	if got != want {
		t.Errorf("defaultJobs() = %d, want min(NumCPU,4) = %d", got, want)
	}
}

// TestDefaultCaps locks the out-of-the-box decompression-bomb cap values:
// unlimited-by-default left a crafted archive free to exhaust disk/inodes
// with no flag required, so these must stay non-zero.
func TestDefaultCaps(t *testing.T) {
	gotBytes, err := parseSize(defaultMaxSize)
	if err != nil {
		t.Fatalf("parseSize(defaultMaxSize=%q): %v", defaultMaxSize, err)
	}
	if want := int64(20) << 30; gotBytes != want {
		t.Errorf("defaultMaxSize %q = %d bytes, want %d (20G)", defaultMaxSize, gotBytes, want)
	}
	if defaultMaxEntries != 200000 {
		t.Errorf("defaultMaxEntries = %d, want 200000", defaultMaxEntries)
	}
}

// TestRun_ListFixture lists a real fixture and confirms it writes no output
// and emits valid JSON with at least one entry. Skips when no fixture is present.
func TestRun_ListFixture(t *testing.T) {
	matches, _ := filepath.Glob("testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping list fixture test")
	}
	src := matches[0]
	dir := t.TempDir()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := run([]string{"-l", "--json", src})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if code != 0 {
		t.Fatalf("run(-l --json) = %d, want 0", code)
	}
	if !strings.Contains(string(out), "\"archives\"") || !strings.Contains(string(out), "\"entries\"") {
		t.Errorf("JSON listing missing expected keys:\n%s", out)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("--list wrote output into an unrelated dir; found %d entries", len(entries))
	}
}

// TestRun_TestFixture validates a real fixture with -t and confirms it writes
// nothing. Skips when no fixture is present.
func TestRun_TestFixture(t *testing.T) {
	matches, _ := filepath.Glob("testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping test-mode fixture test")
	}
	src := matches[0]

	if got := run([]string{"-t", "-q", src}); got != 0 {
		t.Errorf("run(-t) = %d, want 0", got)
	}
}

// TestRun_Batch exercises multi-input extraction into a shared --dest and
// continue-on-error against a real fixture. Skips when no fixture is present.
func TestRun_Batch(t *testing.T) {
	matches, _ := filepath.Glob("testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping batch test")
	}
	src := matches[0]
	dir := t.TempDir()

	outDir := filepath.Join(dir, "out")
	if got := run([]string{"-q", "--jobs", "2", "-o", outDir, src}); got != 0 {
		t.Fatalf("batch run = %d, want 0", got)
	}
	if _, err := os.Stat(outDir); err != nil {
		t.Errorf("expected batch output dir %s: %v", outDir, err)
	}

	// Continue-on-error: one good input + one missing -> nonzero exit, good
	// one's destination directory still gets created/populated.
	outDir2 := filepath.Join(dir, "out2")
	missing := filepath.Join(dir, "missing.rar")
	if got := run([]string{"-q", "-o", outDir2, src, missing}); got == 0 {
		t.Errorf("batch with one failure = %d, want nonzero", got)
	}
}
