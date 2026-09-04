package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

func sampleArchive() listedArchive {
	mtime := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	return listedArchive{
		Src: "sample.rar",
		Entries: []rarutil.EntryInfo{
			{Name: "docs", IsDir: true, Modified: mtime},
			{Name: "docs/readme.txt", Size: 128, Modified: mtime},
			{Name: "streamed.bin", Size: -1},
		},
	}
}

func TestPrintList_Human(t *testing.T) {
	var buf bytes.Buffer
	printList(&buf, []listedArchive{sampleArchive()})
	out := buf.String()

	for _, want := range []string{"docs/", "docs/readme.txt", "128", "streamed.bin"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q\n%s", want, out)
		}
	}
	// A directory must be visually marked with a trailing slash, not shown as a
	// 0-byte file.
	if !strings.Contains(out, "docs/") {
		t.Errorf("directory not marked with trailing slash:\n%s", out)
	}
}

// TestPrintList_StripsControlChars defends the operator's terminal: a hostile
// archive entry name carrying ANSI escapes / CR / NUL must not reach a TTY raw,
// where it could rewrite the screen or hide entries. Printable UTF-8 (incl. CJK)
// must survive untouched.
func TestPrintList_StripsControlChars(t *testing.T) {
	var buf bytes.Buffer
	printList(&buf, []listedArchive{{
		Src: "evil.rar",
		Entries: []rarutil.EntryInfo{
			{Name: "safe\x1b[2Khidden\rspoof.txt", Size: 1},
			{Name: "日本語.txt", Size: 2},
		},
	}})
	out := buf.String()

	for _, bad := range []string{"\x1b", "\r", "\x00"} {
		if strings.Contains(out, bad) {
			t.Errorf("human output leaked control char %q to the terminal:\n%q", bad, out)
		}
	}
	// Legitimate non-ASCII names must not be mangled.
	if !strings.Contains(out, "日本語.txt") {
		t.Errorf("printable UTF-8 name was altered:\n%q", out)
	}
}

func TestReportListJSON_Shape(t *testing.T) {
	var buf bytes.Buffer
	code := reportListJSON(&buf, []listedArchive{sampleArchive()})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	var doc struct {
		Archives []struct {
			Src     string `json:"src"`
			Count   int    `json:"count"`
			Entries []struct {
				Name     string `json:"name"`
				Size     int64  `json:"size"`
				Modified string `json:"modified"`
				IsDir    bool   `json:"isDir"`
			} `json:"entries"`
			Error string `json:"error"`
		} `json:"archives"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(doc.Archives) != 1 {
		t.Fatalf("archives = %d, want 1", len(doc.Archives))
	}
	a := doc.Archives[0]
	if a.Src != "sample.rar" || a.Count != 3 || len(a.Entries) != 3 {
		t.Fatalf("archive = %+v", a)
	}
	if !a.Entries[0].IsDir || a.Entries[0].Name != "docs" {
		t.Errorf("dir entry = %+v", a.Entries[0])
	}
	if a.Entries[1].Size != 128 || a.Entries[1].Modified == "" {
		t.Errorf("file entry = %+v", a.Entries[1])
	}
	// Unknown size stays -1; a zero modtime is omitted, not emitted as a date.
	if a.Entries[2].Size != -1 || a.Entries[2].Modified != "" {
		t.Errorf("streamed entry = %+v", a.Entries[2])
	}
}

func TestReportListJSON_ErrorExitsNonZero(t *testing.T) {
	var buf bytes.Buffer
	archives := []listedArchive{{Src: "broken.rar", Err: errors.New("open rar: bad magic")}}
	if code := reportListJSON(&buf, archives); code != 1 {
		t.Errorf("exit code = %d, want 1 when an archive failed", code)
	}
	if !strings.Contains(buf.String(), "bad magic") {
		t.Errorf("JSON did not surface the per-archive error:\n%s", buf.String())
	}
}
