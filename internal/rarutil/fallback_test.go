package rarutil

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeTool drops an executable shell script named tool into dir that, when
// run, creates a known file inside the last argument (the extraction dir). It
// returns dir so the caller can prepend it to PATH.
func writeFakeTool(t *testing.T, dir, tool, marker string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-tool test uses a POSIX shell script")
	}
	script := "#!/bin/sh\n" +
		// The last positional arg is the destination dir; POSIX way to grab it.
		"for dest do :; done\n" +
		"dest=\"${dest%/}\"\n" +
		"mkdir -p \"$dest/sub\"\n" +
		"printf '%s' '" + marker + "' > \"$dest/extracted.txt\"\n" +
		"printf '%s' 'nested' > \"$dest/sub/inner.txt\"\n"
	p := filepath.Join(dir, tool)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestLookFallbackTool(t *testing.T) {
	dir := t.TempDir()
	writeFakeTool(t, dir, "unrar", "x")
	t.Setenv("PATH", dir)

	name, path := lookFallbackTool()
	if name != "unrar" {
		t.Errorf("tool name = %q, want unrar", name)
	}
	if path == "" {
		t.Error("tool path is empty")
	}
}

func TestLookFallbackTool_None(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir, no tools
	if name, _ := lookFallbackTool(); name != "" {
		t.Errorf("expected no tool, got %q", name)
	}
}

func TestZipDir(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "out.zip")
	if err := zipDir(src, dst, Options{Verify: true}); err != nil {
		t.Fatalf("zipDir: %v", err)
	}

	got := map[string]string{}
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		got[f.Name] = string(b)
	}
	if got["a.txt"] != "hello" || got["sub/b.txt"] != "world" {
		t.Errorf("zipped content = %v, want a.txt=hello sub/b.txt=world", got)
	}
}

// TestZipDir_ByteCapLeavesNoOutput proves a tripped size cap aborts with NO
// output file (the bomb is never silently truncated into a usable ZIP).
func TestZipDir_ByteCapLeavesNoOutput(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "big.bin"), make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out.zip")

	err := zipDir(src, dst, Options{Store: true, MaxTotalBytes: 100})
	if err == nil {
		t.Fatal("expected size-cap error")
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Errorf("output should not exist after a tripped cap; stat err = %v", statErr)
	}
}

// TestZipDir_EntryCapLeavesNoOutput proves a tripped entry-count cap aborts
// with no output file.
func TestZipDir_EntryCapLeavesNoOutput(t *testing.T) {
	src := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(src, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dst := filepath.Join(t.TempDir(), "out.zip")

	err := zipDir(src, dst, Options{MaxEntries: 1})
	if err == nil {
		t.Fatal("expected entry-cap error")
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Errorf("output should not exist after a tripped cap; stat err = %v", statErr)
	}
}

// TestZipDir_NeutralizesSymlink proves the emitter refactor preserved symlink
// neutralization on the fallback path: a symlink is stored as inert content
// (its target text), never as a link mode that an extractor could follow out
// of the destination tree.
func TestZipDir_NeutralizesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	src := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out.zip")
	if err := zipDir(src, dst, Options{}); err != nil {
		t.Fatalf("zipDir: %v", err)
	}

	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var checked bool
	for _, f := range zr.File {
		if f.Name != "link" {
			continue
		}
		checked = true
		if f.Mode()&os.ModeSymlink != 0 {
			t.Error("symlink entry retained link mode; should be neutralized to a regular file")
		}
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if string(b) != "/etc/passwd" {
			t.Errorf("symlink content = %q, want the target text /etc/passwd", b)
		}
	}
	if !checked {
		t.Error("no 'link' entry found in output")
	}
}

func TestConvertViaFallback_Integration(t *testing.T) {
	dir := t.TempDir()
	writeFakeTool(t, dir, "unrar", "FROMTOOL")
	// Prepend the fake-tool dir so our unrar wins, but keep the system bins on
	// PATH so the script's own mkdir/printf resolve.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/bin:/usr/bin")

	dst := filepath.Join(t.TempDir(), "out.zip")
	// srcRar need not exist; the fake tool ignores it and writes fixed output.
	err := convertViaFallback("whatever.rar", dst, Options{}, errors.New("native decode failed"))
	if err != nil {
		t.Fatalf("convertViaFallback: %v", err)
	}

	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatalf("open result zip: %v", err)
	}
	defer zr.Close()
	var found bool
	for _, f := range zr.File {
		if f.Name == "extracted.txt" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			if string(b) == "FROMTOOL" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected extracted.txt with tool content in fallback output")
	}
}

func TestExtractArgs(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		src      string
		password string
		want     []string
	}{
		{"unrar no password", "unrar", "a.rar", "", []string{"x", "-o+", "-p-", "--", "a.rar", "out/"}},
		{"unrar with password", "unrar", "a.rar", "secret", []string{"x", "-o+", "-psecret", "--", "a.rar", "out/"}},
		{"7z no password", "7z", "a.rar", "", []string{"x", "-y", "-oout", "--", "a.rar"}},
		{"7z with password", "7z", "a.rar", "secret", []string{"x", "-y", "-psecret", "-oout", "--", "a.rar"}},
		// A source starting with '@' is rewritten so it is not read as a list file.
		{"7z at-prefixed source", "7z", "@list.rar", "", []string{"x", "-y", "-oout", "--", "./@list.rar"}},
		{"unrar at-prefixed source", "unrar", "@list.rar", "", []string{"x", "-o+", "-p-", "--", "./@list.rar", "out/"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractArgs(tc.tool, tc.src, "out", tc.password)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("extractArgs = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestExtractArgs_DashSourceNotASwitch asserts the real invariant: a source
// path beginning with '-' lands after the "--" terminator (so the extractor
// treats it as a filename, never a switch).
func TestExtractArgs_DashSourceNotASwitch(t *testing.T) {
	for _, tool := range []string{"unrar", "7z"} {
		got := extractArgs(tool, "-foo.rar", "out", "")
		var sawTerm bool
		for i, a := range got {
			if a == "--" {
				sawTerm = true
				rest := got[i+1:]
				found := false
				for _, r := range rest {
					if r == "-foo.rar" {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: source not found after -- terminator: %v", tool, got)
				}
				// Nothing before "--" should be the source path.
				for _, b := range got[:i] {
					if b == "-foo.rar" {
						t.Errorf("%s: source appears before -- (would parse as switch): %v", tool, got)
					}
				}
			}
		}
		if !sawTerm {
			t.Errorf("%s: no -- terminator in %v", tool, got)
		}
	}
}

func TestConvertViaFallback_NoTool_ReturnsNativeError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no unrar/7z
	nativeErr := errors.New("native decode failed")
	dst := filepath.Join(t.TempDir(), "out.zip")

	err := convertViaFallback("x.rar", dst, Options{}, nativeErr)
	if err == nil {
		t.Fatal("expected an error when no fallback tool is present")
	}
	if !errors.Is(err, nativeErr) {
		t.Errorf("error %v does not wrap the original native error", err)
	}
}
