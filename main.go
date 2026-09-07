// Command macrarcli extracts .rar archives directly: list contents, extract
// (preserving structure or flat), or validate integrity — no ZIP output.
//
// Usage:
//
//	macrarcli [flags] <input.rar> [more.rar ...]
//
// By default each input is extracted into the destination directory (cwd
// unless -o/--dest is given), preserving its internal directory structure.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/nwaples/rardecode/v2"
	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

// version and commit are overridden at build time via -ldflags.
var (
	version = "dev"
	commit  = "none"
)

// maxDefaultJobs caps the batch concurrency default. RAR decode is largely
// I/O-bound, so fanning out past a handful of workers mostly adds disk
// contention and memory (one pooled copy buffer per in-flight job) for no gain.
const maxDefaultJobs = 4

// passwordEnvVar is the environment-variable fallback for --password, so
// scripted/automated use doesn't have to put a secret in argv (visible via
// `ps`/`/proc/<pid>/cmdline` to any local user for the life of the process).
const passwordEnvVar = "MACRARCLI_PASSWORD"

// defaultMaxSize and defaultMaxEntries are the out-of-the-box decompression-
// bomb caps: unlimited-by-default left a crafted small archive free to
// exhaust disk or inodes on extract/test with no flag required to trigger it.
// Pass --max-size 0 / --max-entries 0 explicitly to opt back into unlimited.
const (
	defaultMaxSize    = "20G"
	defaultMaxEntries = 200000
)

// defaultJobs is the out-of-the-box --jobs value: multi-core but capped, so a
// `*.rar` batch uses available cores without unbounded fan-out. Always >= 1.
func defaultJobs() int {
	n := runtime.NumCPU()
	if n > maxDefaultJobs {
		return maxDefaultJobs
	}
	if n < 1 {
		return 1
	}
	return n
}

func main() {
	os.Exit(run(os.Args[1:]))
}

// run executes the CLI and returns a process exit code:
// 0 success, 1 usage error, 2 wrong/missing password, 3 corrupted entry,
// 4 other runtime error. A batch aggregates per-job codes with 2 > 3 > 4
// precedence when outcomes differ across jobs.
func run(args []string) int {
	var (
		dest        string
		flatMode    bool
		listMode    bool
		testMode    bool
		overwrite   bool
		skip        bool
		rename      bool
		quiet       bool
		password    string
		jobs        int
		jsonOut     bool
		showVersion bool
		maxSize     string
		maxEntries  int
		verbose     bool
	)

	fs := flag.NewFlagSet("macrarcli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&dest, "o", "", "destination directory for extracted files (default: cwd)")
	fs.StringVar(&dest, "dest", "", "destination directory for extracted files (default: cwd)")
	fs.BoolVar(&flatMode, "e", false, "extract without preserving directory structure")
	fs.BoolVar(&flatMode, "flat", false, "extract without preserving directory structure")
	fs.BoolVar(&listMode, "l", false, "preview archive contents without extracting (read-only)")
	fs.BoolVar(&listMode, "list", false, "preview archive contents without extracting (read-only)")
	fs.BoolVar(&testMode, "t", false, "validate archive integrity without extracting (read-only)")
	fs.BoolVar(&testMode, "test", false, "validate archive integrity without extracting (read-only)")
	fs.BoolVar(&overwrite, "overwrite", false, "replace existing destination files")
	fs.BoolVar(&skip, "skip", false, "skip individual entries whose destination already exists")
	fs.BoolVar(&rename, "rename", false, "write colliding entries under a \" (n)\" suffixed name instead")
	fs.BoolVar(&quiet, "q", false, "suppress progress output")
	fs.BoolVar(&quiet, "quiet", false, "suppress progress output")
	fs.StringVar(&password, "password", "", "password for encrypted archives (or $"+passwordEnvVar+")")
	fs.IntVar(&jobs, "jobs", defaultJobs(), "number of archives to process concurrently (default: min(NumCPU,4))")
	fs.BoolVar(&jsonOut, "json", false, "emit a machine-readable JSON summary on stdout")
	fs.StringVar(&maxSize, "max-size", defaultMaxSize, "cap total uncompressed size (default 20G; 0 = unlimited; accepts K/M/G suffix)")
	fs.IntVar(&maxEntries, "max-entries", defaultMaxEntries, "cap number of entries per archive (default 200000; 0 = unlimited)")
	fs.BoolVar(&verbose, "verbose", false, "print extra diagnostics (decode path, per-archive timing) to stderr")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: macrarcli [flags] <input.rar> [more.rar ...]\n\n"+
			"Extract RAR archives directly (list/extract/extract-flat/test). No ZIP output.\n\n"+
			"Exit codes: 0 success, 1 usage error, 2 wrong/missing password, 3 corrupted\n"+
			"entry, 4 other runtime error. Exit code 2 is only guaranteed for RAR5-\n"+
			"encrypted archives: a wrong password on a legacy RAR3/4 archive is not\n"+
			"reliably distinguishable from corruption and may report exit 3 instead.\n\n"+
			"flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp { // -h / --help
			return 0
		}
		return 1
	}

	if showVersion {
		fmt.Printf("macrarcli v%s (%s)\n", version, commit)
		return 0
	}

	mode, err := resolveMode(flatMode, listMode, testMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "macrarcli: %v\n", err)
		return 1
	}
	overwritePolicy, overwriteSet, err := resolveOverwritePolicy(overwrite, skip, rename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "macrarcli: %v\n", err)
		return 1
	}

	inputs := fs.Args()
	if code := validateArgs(inputs, mode, dest, overwriteSet, jobs); code != 0 {
		return code
	}

	maxBytes, err := parseSize(maxSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "macrarcli: invalid --max-size %q: %v\n", maxSize, err)
		return 1
	}
	if maxEntries < 0 {
		fmt.Fprintln(os.Stderr, "macrarcli: --max-entries must be >= 0")
		return 1
	}

	// Password is resolved ONCE here, before any job/List/Test call, and the
	// single result is reused for every input — never re-resolved per job,
	// which would re-prompt and race on shared stdin/tty state under
	// concurrent --jobs. See rarutil.ResolvePassword's doc comment.
	// The --password flag always wins; MACRARCLI_PASSWORD is the fallback for
	// scripted use that would otherwise expose the secret via argv.
	password = resolveExplicitPassword(password, os.Getenv)
	headerEncrypted := false
	if password == "" && len(inputs) > 0 {
		if _, openErr := rardecode.OpenReader(inputs[0]); errors.Is(openErr, rardecode.ErrArchiveEncrypted) {
			headerEncrypted = true
		}
	}
	resolvedPassword, err := rarutil.ResolvePasswordStdin(password, headerEncrypted)
	if err != nil {
		fmt.Fprintf(os.Stderr, "macrarcli: %v\n", err)
		return 2
	}

	opts := rarutil.Options{
		Password:        resolvedPassword,
		OverwritePolicy: overwritePolicy,
		MaxTotalBytes:   maxBytes,
		MaxEntries:      maxEntries,
		Flat:            mode == modeFlat,
	}
	// Verbose diagnostics go to stderr, and only when stdout isn't owned by --json.
	if verbose && !jsonOut {
		opts.OnVerbose = func(msg string) {
			fmt.Fprintf(os.Stderr, "[verbose] %s\n", msg)
		}
	}
	// --json owns stdout for machine output, so silence the human decoration.
	human := !quiet && !jsonOut
	// Live progress only makes sense for a single archive: a concurrent batch
	// would interleave per-job redraws nondeterministically on one line, so a
	// batch instead relies on the final per-job report lines (emitted in job
	// order). Must run AFTER password resolution: the pre-pass List() call it
	// makes needs the password already resolved to open an encrypted archive.
	if human && mode != modeList && len(inputs) == 1 {
		attachProgress(&opts, inputs[0])
	}

	switch mode {
	case modeList:
		return runList(inputs, opts, jobs, jsonOut)
	case modeTest:
		return runTest(inputs, opts, jobs, jsonOut, quiet)
	default: // modeExtract, modeFlat
		if dest == "" {
			dest = "."
		}
		if err := os.MkdirAll(dest, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "macrarcli: %v\n", err)
			return 4
		}
		jobList := make([]rarutil.Job, len(inputs))
		for i, src := range inputs {
			jobList[i] = rarutil.Job{Src: src, Dst: dest}
		}
		results := rarutil.RunBatch(jobList, opts, jobs, nil)
		if jsonOut {
			return reportJSON(results, os.Stdout)
		}
		return report(results, quiet)
	}
}

// attachProgress wires a two-pass progress display into opts for a single-
// archive human-output run: a List() pre-pass (run here, after password
// resolution) supplies the totals, then opts.OnEntry renders a live
// percentage + throughput line via the existing \r\033[K redraw convention.
// Best-effort: if the pre-pass List() fails, progress is silently skipped
// rather than surfacing an error here — the real error (if any) still
// surfaces from the subsequent Extract/Test call.
func attachProgress(opts *rarutil.Options, src string) {
	entries, err := rarutil.List(src, *opts)
	if err != nil {
		return
	}
	sizeByName := make(map[string]int64, len(entries))
	var total int64
	for _, e := range entries {
		if e.Size > 0 {
			sizeByName[e.Name] = e.Size
			total += e.Size
		}
	}
	tracker := rarutil.NewProgressTracker(total, len(entries))
	opts.OnEntry = func(name string) {
		tracker.OnEntry(sizeByName[name])
		fmt.Fprintf(os.Stderr, "\r\033[K[%3.0f%%] %s (%s/s)", tracker.Percent(), name, humanRate(tracker.ThroughputBPS()))
	}
}

// humanRate renders a bytes/second rate with a 1024-based unit suffix.
func humanRate(bps float64) string {
	switch {
	case bps >= 1<<30:
		return fmt.Sprintf("%.1fG", bps/(1<<30))
	case bps >= 1<<20:
		return fmt.Sprintf("%.1fM", bps/(1<<20))
	case bps >= 1<<10:
		return fmt.Sprintf("%.1fK", bps/(1<<10))
	default:
		return fmt.Sprintf("%.0fB", bps)
	}
}

// classifyErr maps a single job's error to its exit code per the 5-code
// scheme: rardecode's own exported sentinels are checked directly (no custom
// classification type), matching whatever Extract/Test/List returns.
func classifyErr(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, rardecode.ErrBadPassword):
		return 2
	case errors.Is(err, rardecode.ErrBadFileChecksum):
		return 3
	default:
		return 4
	}
}

// aggregateExit combines per-job exit codes into one batch result using the
// documented precedence: 2 (password) beats 3 (corruption) beats 4 (other).
func aggregateExit(codes []int) int {
	for _, want := range []int{2, 3, 4} {
		for _, c := range codes {
			if c == want {
				return want
			}
		}
	}
	return 0
}

// reportHuman prints per-job outcomes and a batch summary to stdout/stderr,
// returning the aggregate exit code per the 5-code scheme. successLine
// renders the human success line for a job with no error and no skips
// (extract: "extracted src -> dst", test: "src: OK"). showPartial selects the
// summary line's shape: extract's batches can have partial (some-entries-
// skipped) outcomes, test's never can (Test never skips an entry).
// quietGatesSuccess preserves a pre-existing per-mode difference: -q was
// never documented to suppress extract's stdout result line (only stderr
// progress output — see README's --quiet entry), so extract's success line
// prints unconditionally; test's success line was already gated by -q before
// this helper existed, and stays that way.
func reportHuman(results []rarutil.Result, quiet, showPartial, quietGatesSuccess bool, successLine func(rarutil.Result) string) int {
	codes := make([]int, len(results))
	failed, skipped := 0, 0
	for i, r := range results {
		codes[i] = classifyErr(r.Err)
		switch {
		case r.Err != nil:
			failed++
			fmt.Fprintf(os.Stderr, "macrarcli: %s: %v\n", r.Src, r.Err)
		case len(r.SkippedEntries) > 0:
			skipped++
			if !quiet {
				fmt.Fprintf(os.Stderr, "extracted %s (%d entries skipped)\n", r.Dst, len(r.SkippedEntries))
			}
		default:
			if !quietGatesSuccess || !quiet {
				fmt.Println(successLine(r))
			}
		}
	}

	if len(results) > 1 && !quiet {
		if showPartial {
			fmt.Fprintf(os.Stderr, "%d succeeded, %d partial, %d failed\n", len(results)-failed-skipped, skipped, failed)
		} else {
			fmt.Fprintf(os.Stderr, "%d succeeded, %d failed\n", len(results)-failed, failed)
		}
	} else if len(results) == 1 && !quiet {
		// Terminate the single-archive progress line.
		fmt.Fprintln(os.Stderr)
	}

	return aggregateExit(codes)
}

// report is reportHuman for extract/flat mode.
func report(results []rarutil.Result, quiet bool) int {
	return reportHuman(results, quiet, true, false, func(r rarutil.Result) string {
		return fmt.Sprintf("extracted %s -> %s", r.Src, r.Dst)
	})
}

// runTest validates each input archive (checksum-only, no filesystem writes),
// up to maxParallel concurrently, and reports outcomes as a human summary or,
// with --json, jsonSummary (mode "test"). Returns the aggregate exit code per
// the 5-code scheme.
func runTest(inputs []string, opts rarutil.Options, maxParallel int, jsonOut, quiet bool) int {
	results := rarutil.TestBatch(inputs, opts, maxParallel)

	if jsonOut {
		return reportTestJSON(results, os.Stdout)
	}
	return reportHuman(results, quiet, false, true, func(r rarutil.Result) string {
		return fmt.Sprintf("%s: OK", r.Src)
	})
}
