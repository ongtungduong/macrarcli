package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

// runList previews each input archive read-only (no output is written) and
// reports the result as a human table or, with --json, a structured
// document. -o/--dest and the overwrite-policy flags are rejected upstream in
// validateArgs since --list writes nothing. The entry-count cap bounds each
// listing. Returns the aggregate exit code per the 5-code scheme.
func runList(inputs []string, password string, maxEntries int, jsonOut bool) int {
	opts := rarutil.Options{Password: password, MaxEntries: maxEntries}
	archives := make([]listedArchive, 0, len(inputs))
	for _, src := range inputs {
		entries, err := rarutil.List(src, opts)
		archives = append(archives, listedArchive{Src: src, Entries: entries, Err: err})
	}

	if jsonOut {
		return reportListJSON(os.Stdout, archives)
	}
	printList(os.Stdout, archives)
	for _, a := range archives {
		if a.Err != nil {
			fmt.Fprintf(os.Stderr, "macrarcli: %s: %v\n", a.Src, a.Err)
		}
	}
	return aggregateExit(listExitCodes(archives))
}

// listExitCodes maps each archive's List error to its exit code, reusing the
// same classification extract/test use (e.g. a wrong password on a RAR5-
// encrypted archive still reports exit 2 here).
func listExitCodes(archives []listedArchive) []int {
	codes := make([]int, len(archives))
	for i, a := range archives {
		codes[i] = classifyErr(a.Err)
	}
	return codes
}

// listedArchive is one archive's --list outcome: its entries, or the error that
// prevented reading it. Err and Entries are reported together so a multi-archive
// listing continues past a single unreadable input.
type listedArchive struct {
	Src     string
	Entries []rarutil.EntryInfo
	Err     error
}

// printList writes a human-readable preview of each archive's contents to w.
// Directories are shown with a trailing slash and a "-" size; unknown sizes (a
// streamed entry the archive did not size) also render as "-". A per-archive
// header is printed only when more than one archive is listed. Archives that
// failed to read are skipped here — the caller reports their error on stderr so
// stdout carries only listings.
func printList(w io.Writer, archives []listedArchive) {
	multi := len(archives) > 1
	printed := false
	for _, a := range archives {
		if a.Err != nil {
			continue
		}
		if multi {
			if printed {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "%s:\n", a.Src)
		}
		printed = true

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SIZE\tPACKED\tENC\tMODIFIED\tNAME")
		for _, e := range a.Entries {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", listSize(e), listPackedSize(e), listEncrypted(e), listTime(e.Modified), listName(e))
		}
		tw.Flush()
	}
}

// listName renders an entry name for the human table: control characters are
// replaced with '?' so a hostile archive cannot smuggle ANSI escapes, CR, or NUL
// into the operator's terminal (printable UTF-8, including CJK, is preserved),
// and directories get a trailing slash so a preview reads like a file tree. The
// --json path keeps the raw name — encoding/json escapes control characters.
func listName(e rarutil.EntryInfo) string {
	name := displaySafe(e.Name)
	if e.IsDir {
		return name + "/"
	}
	return name
}

// displaySafe replaces control runes (C0, C1, DEL) with '?' for TTY-safe output.
func displaySafe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}

// listSize renders a directory or unknown-size entry as "-" and any other entry
// as its byte count.
func listSize(e rarutil.EntryInfo) string {
	if e.IsDir || e.Size < 0 {
		return "-"
	}
	return fmt.Sprintf("%d", e.Size)
}

// listPackedSize renders a directory entry's packed size as "-" (a directory
// carries no compressed payload) and any other entry as its byte count.
func listPackedSize(e rarutil.EntryInfo) string {
	if e.IsDir {
		return "-"
	}
	return fmt.Sprintf("%d", e.PackedSize)
}

// listEncrypted renders "yes"/"-" so the column stays legible in a
// tab-aligned table (an empty string would misalign the tabwriter).
func listEncrypted(e rarutil.EntryInfo) string {
	if e.Encrypted {
		return "yes"
	}
	return "-"
}

// listTime renders a zero modtime as "-" rather than the Go zero date.
func listTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

// listEntryJSON is one entry in the --list --json document. modified is omitted
// when the archive recorded none; size stays -1 for unknown-size entries.
type listEntryJSON struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	PackedSize int64  `json:"packedSize"`
	Encrypted  bool   `json:"encrypted,omitempty"`
	Modified   string `json:"modified,omitempty"`
	IsDir      bool   `json:"isDir"`
}

// listArchiveJSON is one archive's entry in the --list --json document.
type listArchiveJSON struct {
	Src     string          `json:"src"`
	Count   int             `json:"count"`
	Entries []listEntryJSON `json:"entries"`
	Error   string          `json:"error,omitempty"`
}

// listSummaryJSON is the top-level --list --json document.
type listSummaryJSON struct {
	Mode     string            `json:"mode"`
	Archives []listArchiveJSON `json:"archives"`
}

// reportListJSON writes a JSON listing of every archive to w and returns the
// aggregate exit code per the 5-code scheme (classifyErr/aggregateExit).
func reportListJSON(w io.Writer, archives []listedArchive) int {
	doc := listSummaryJSON{Mode: "list", Archives: make([]listArchiveJSON, 0, len(archives))}
	for _, a := range archives {
		aj := listArchiveJSON{
			Src:     a.Src,
			Count:   len(a.Entries),
			Entries: make([]listEntryJSON, 0, len(a.Entries)),
		}
		if a.Err != nil {
			aj.Error = a.Err.Error()
		}
		for _, e := range a.Entries {
			ej := listEntryJSON{Name: e.Name, Size: e.Size, PackedSize: e.PackedSize, Encrypted: e.Encrypted, IsDir: e.IsDir}
			if !e.Modified.IsZero() {
				ej.Modified = e.Modified.UTC().Format(time.RFC3339)
			}
			aj.Entries = append(aj.Entries, ej)
		}
		doc.Archives = append(doc.Archives, aj)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return 4
	}
	return aggregateExit(listExitCodes(archives))
}
