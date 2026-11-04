package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/nwaples/rardecode/v2"
	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

type countingCloser struct{ closed *int }

func (c countingCloser) Close() error { *c.closed++; return nil }

func TestProbeEncryptedHeaders_AllInputsAndImmediateClose(t *testing.T) {
	closed, called := 0, 0
	found := probeEncryptedHeaders([]string{"plain", "broken", "encrypted", "plain2"}, func(src string) (io.Closer, error) {
		called++
		if called > 1 && closed != 1 {
			t.Fatalf("first reader not closed immediately: %d", closed)
		}
		switch src {
		case "encrypted":
			return nil, fmt.Errorf("wrapped: %w", rardecode.ErrArchiveEncrypted)
		case "broken":
			return nil, errors.New("bad archive")
		default:
			return countingCloser{&closed}, nil
		}
	})
	if !found || called != 4 || closed != 2 {
		t.Fatalf("encrypted=%v calls=%d closed=%d", found, called, closed)
	}
}

func TestProgressCallback_ArchiveOrderAndSafeNames(t *testing.T) {
	entries := []rarutil.EntryInfo{
		{Name: "dir", IsDir: true},
		{Name: "a/./same", Size: 1},
		{Name: "b/same", Size: 3},
		{Name: "b/same", Size: 6},
	}
	var out bytes.Buffer
	callback := progressCallback(entries, true, &out)
	for i, name := range []string{"same", "same (1)", "same (2)\x1b]0;bad\a"} {
		out.Reset()
		callback(name)
		want := []string{"[ 10%]", "[ 40%]", "[100%]"}[i]
		if !strings.Contains(out.String(), want) {
			t.Fatalf("progress %q missing %q", out.String(), want)
		}
		body := strings.TrimPrefix(out.String(), "\r\033[K")
		if strings.ContainsAny(body, "\x1b\a\r\n") {
			t.Fatalf("terminal injection: %q", body)
		}
	}
	out.Reset()
	callback("extra")
	if out.Len() != 0 {
		t.Fatal("unexpected extra callback output")
	}
}

func TestProgressCallback_UnknownEmptyAndOverflow(t *testing.T) {
	for _, sizes := range [][]int64{{-1, 3}, {0, 0}, {math.MaxInt64, 1}} {
		var out bytes.Buffer
		callback := progressCallback([]rarutil.EntryInfo{{Size: sizes[0]}, {Size: sizes[1]}}, false, &out)
		callback("first")
		if !strings.Contains(out.String(), "[ 50%]") {
			t.Fatalf("count fallback: %q", out.String())
		}
		out.Reset()
		callback("last")
		if !strings.Contains(out.String(), "[100%]") {
			t.Fatalf("completion: %q", out.String())
		}
	}
	var out bytes.Buffer
	progressCallback(nil, false, &out)("ignored")
	if out.Len() != 0 {
		t.Fatal("empty progress wrote output")
	}
}

func TestDictionarySizeValidation(t *testing.T) {
	for _, input := range []string{"0", "", "-1", "9223372036854775807G", "invalid"} {
		if _, err := parseDictionarySize(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	for input, want := range map[string]int64{"256M": 256 << 20, "1G": 1 << 30, "1": 1} {
		if got, err := parseDictionarySize(input); err != nil || got != want {
			t.Fatalf("%s: %d %v", input, got, err)
		}
	}
	if _, err := parseSize("9223372036854775807G"); err == nil {
		t.Fatal("overflow accepted")
	}
}

func TestHumanOutputSafeJSONPreservesNames(t *testing.T) {
	hostile := "evil\x1b[2J\n\r\a"
	results := []rarutil.Result{{Job: rarutil.Job{Src: hostile, Dst: hostile}}}
	human := captureStdout(t, func() { report(results, true) })
	if strings.ContainsAny(strings.TrimSuffix(human, "\n"), "\x1b\r\n\a") {
		t.Fatalf("unsafe report %q", human)
	}
	var out bytes.Buffer
	printVerbose(&out, hostile)
	if strings.ContainsAny(strings.TrimSuffix(out.String(), "\n"), "\x1b\r\n\a") {
		t.Fatal("unsafe verbose")
	}
	out.Reset()
	printList(&out, []listedArchive{{Src: hostile}, {Src: "second"}})
	if strings.ContainsAny(out.String(), "\x1b\r\a") {
		t.Fatal("unsafe listing header")
	}
	out.Reset()
	if code := reportJSON(results, &out); code != 0 || !strings.Contains(out.String(), `\u001b`) {
		t.Fatalf("JSON must escape original: %d %s", code, out.String())
	}
}

func TestRun_DictionaryFlagValidation(t *testing.T) {
	for _, value := range []string{"0", "-2", "999999999999999999G"} {
		if code := run([]string{"--max-dict", value, "missing.rar"}); code != 1 {
			t.Fatalf("value %q: exit %d", value, code)
		}
	}
}

func TestRun_FlagErrorSafe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = w
	code := run([]string{"--bad\x1b[2J"})
	os.Stderr = original
	w.Close()
	out, _ := io.ReadAll(r)
	r.Close()
	if code != 1 || bytes.Contains(out, []byte{27}) {
		t.Fatalf("unsafe flag diagnostic: %d %q", code, out)
	}
}
