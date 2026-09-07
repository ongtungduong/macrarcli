package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

// cliMode selects what each input archive is used for.
type cliMode int

const (
	modeExtract cliMode = iota // default: preserve directory structure
	modeFlat                   // -e/--flat: extract, discarding structure
	modeList                   // -l/--list: read-only preview
	modeTest                   // -t/--test: checksum-only validation, no writes
)

// resolveMode enforces the -e/-l/-t mutual exclusion (they select different,
// incompatible things to do with an archive) and returns the selected mode,
// defaulting to modeExtract when none is given.
func resolveMode(flat, list, test bool) (cliMode, error) {
	n := 0
	m := modeExtract
	if flat {
		n++
		m = modeFlat
	}
	if list {
		n++
		m = modeList
	}
	if test {
		n++
		m = modeTest
	}
	if n > 1 {
		return 0, fmt.Errorf("-e/--flat, -l/--list, -t/--test are mutually exclusive")
	}
	return m, nil
}

// resolveOverwritePolicy enforces the --overwrite/--skip/--rename mutual
// exclusion and reports whether any of the three was explicitly given (the
// zero policy, OverwriteFail, is otherwise indistinguishable from "not set").
func resolveOverwritePolicy(overwrite, skip, rename bool) (policy rarutil.OverwritePolicy, explicitlySet bool, err error) {
	n := 0
	if overwrite {
		n++
		policy = rarutil.OverwriteOverwrite
	}
	if skip {
		n++
		policy = rarutil.OverwriteSkip
	}
	if rename {
		n++
		policy = rarutil.OverwriteRename
	}
	if n > 1 {
		return 0, false, fmt.Errorf("--overwrite, --skip, --rename are mutually exclusive")
	}
	return policy, n == 1, nil
}

// validateArgs returns a usage exit code (1) for a malformed invocation, or 0.
// It runs before any archive is opened.
func validateArgs(inputs []string, mode cliMode, dest string, overwriteSet bool, jobs int) int {
	usage := func(format string, a ...any) int {
		fmt.Fprintf(os.Stderr, "macrarcli: "+format+"\n", a...)
		return 1
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: macrarcli [flags] <input.rar> [more.rar ...]")
		return 1
	}
	if (mode == modeList || mode == modeTest) && (dest != "" || overwriteSet) {
		return usage("-o/--dest and --overwrite/--skip/--rename write output; -l/--list and -t/--test are read-only")
	}
	if jobs < 1 {
		return usage("--jobs must be >= 1")
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

// resolveExplicitPassword picks the password to use before any TTY prompt is
// considered: the --password flag always wins if set (matching
// ResolvePassword's "explicit always wins" contract), otherwise the
// MACRARCLI_PASSWORD environment variable is used. getenv is injected so this
// stays unit-testable without mutating process environment.
func resolveExplicitPassword(flagVal string, getenv func(string) string) string {
	if flagVal != "" {
		return flagVal
	}
	return getenv(passwordEnvVar)
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
