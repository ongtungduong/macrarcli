package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

// validateArgs returns a usage exit code (2) for malformed invocations, or 0.
func validateArgs(inputs []string, output, outDir string, jobs int, store bool, level int) int {
	usage := func(format string, a ...any) int {
		fmt.Fprintf(os.Stderr, "rar2zip: "+format+"\n", a...)
		return 2
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: rar2zip [flags] <input.rar> [more.rar ...]")
		return 2
	}
	if output != "" && outDir != "" {
		return usage("-o/--output and --out-dir are mutually exclusive")
	}
	if output != "" && len(inputs) > 1 {
		return usage("-o/--output targets a single file; use --out-dir for multiple inputs")
	}
	if jobs < 1 {
		return usage("--jobs must be >= 1")
	}
	if store && level != 0 {
		return usage("cannot combine --store and --level")
	}
	if level != 0 && (level < 1 || level > 9) {
		return usage("--level must be between 1 and 9 (use --store for no compression)")
	}
	for _, src := range inputs {
		if !strings.EqualFold(filepath.Ext(src), ".rar") {
			return usage("input must be a .rar file: %s", src)
		}
		if fi, err := os.Stat(src); err == nil && fi.IsDir() {
			return usage("input is a directory, not a .rar file: %s", src)
		}
	}
	return 0
}

// buildJobs resolves each input to its destination ZIP and rejects a batch in
// which two distinct inputs resolve to the same output. Without this guard a
// concurrent batch would race to last-writer-wins, silently losing one input's
// data — the same data-loss class as intra-archive name collisions.
func buildJobs(inputs []string, output, outDir string) ([]rarutil.Job, error) {
	jobs := make([]rarutil.Job, 0, len(inputs))
	seen := make(map[string]string, len(inputs)) // dst -> first src that claimed it
	for _, src := range inputs {
		dst := resolveDst(src, output, outDir)
		if prev, dup := seen[dst]; dup {
			return nil, fmt.Errorf("inputs %q and %q both map to output %q; rename one or convert them separately", prev, src, dst)
		}
		seen[dst] = src
		jobs = append(jobs, rarutil.Job{Src: src, Dst: dst})
	}
	return jobs, nil
}

// parseSize converts a byte-size string into a count of bytes. It accepts a
// plain integer or one with a K/M/G (1024-based) suffix; "0" means unlimited.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'k', 'K':
		mult = 1 << 10
	case 'm', 'M':
		mult = 1 << 20
	case 'g', 'G':
		mult = 1 << 30
	}
	if mult != 1 {
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("not a byte size (use e.g. 500, 10M, 2G)")
	}
	if n < 0 {
		return 0, fmt.Errorf("must be >= 0")
	}
	return n * mult, nil
}

// resolveDst computes the destination ZIP path for one input. --out-dir places
// <base>.zip in that directory; -o names a file (or, if it is an existing
// directory, places <base>.zip inside it); otherwise the output is the sibling
// <input>.zip.
func resolveDst(src, output, outDir string) string {
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)) + ".zip"
	switch {
	case outDir != "":
		return filepath.Join(outDir, base)
	case output != "":
		if fi, err := os.Stat(output); err == nil && fi.IsDir() {
			return filepath.Join(output, base)
		}
		return output
	default:
		return filepath.Join(filepath.Dir(src), base)
	}
}
