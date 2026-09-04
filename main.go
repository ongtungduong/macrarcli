// Command rar2zip converts one or more .rar archives into .zip archives.
//
// Usage:
//
//	rar2zip [flags] <input.rar> [more.rar ...]
//
// By default each <input>.zip is written alongside its input file.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

// version and commit are overridden at build time via -ldflags.
var (
	version = "dev"
	commit  = "none"
)

// maxDefaultJobs caps the batch concurrency default. RAR decode + ZIP write is
// largely I/O-bound, so fanning out past a handful of workers mostly adds disk
// contention and memory (one pooled copy buffer per in-flight job) for no gain.
const maxDefaultJobs = 4

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
// 0 success, 1 runtime error, 2 usage error.
func run(args []string) int {
	var (
		output        string
		outDir        string
		force         bool
		quiet         bool
		password      string
		jobs          int
		store         bool
		level         int
		verify        bool
		jsonOut       bool
		allowFallback bool
		showVersion   bool
		maxSize       string
		maxEntries    int
		list          bool
		skipExisting  bool
		verbose       bool
	)

	fs := flag.NewFlagSet("rar2zip", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&output, "o", "", "write the single output to this file or directory")
	fs.StringVar(&output, "output", "", "write the single output to this file or directory")
	fs.StringVar(&outDir, "out-dir", "", "write all outputs into this directory")
	fs.BoolVar(&force, "f", false, "overwrite outputs that already exist")
	fs.BoolVar(&force, "force", false, "overwrite outputs that already exist")
	fs.BoolVar(&quiet, "q", false, "suppress progress output")
	fs.BoolVar(&quiet, "quiet", false, "suppress progress output")
	fs.StringVar(&password, "password", "", "password for encrypted archives")
	fs.IntVar(&jobs, "jobs", defaultJobs(), "number of archives to convert concurrently (default: min(NumCPU,4))")
	fs.BoolVar(&store, "store", false, "store entries without compression")
	fs.IntVar(&level, "level", 0, "Deflate compression level 1..9 (default: stdlib default)")
	fs.BoolVar(&verify, "verify", false, "reopen each output ZIP and validate it after writing")
	fs.BoolVar(&jsonOut, "json", false, "emit a machine-readable JSON summary on stdout")
	fs.BoolVar(&allowFallback, "allow-fallback", false, "use system unrar/7z when the pure-Go decoder fails (unsafe vs untrusted archives)")
	fs.StringVar(&maxSize, "max-size", "0", "cap total uncompressed size (0 = unlimited; accepts K/M/G suffix)")
	fs.IntVar(&maxEntries, "max-entries", 0, "cap number of entries per archive (0 = unlimited)")
	fs.BoolVar(&list, "list", false, "preview archive contents without converting (read-only)")
	fs.BoolVar(&skipExisting, "skip-existing", false, "skip inputs whose output already exists (instead of failing)")
	fs.BoolVar(&verbose, "verbose", false, "print extra diagnostics (decode path, per-archive timing) to stderr")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: rar2zip [flags] <input.rar> [more.rar ...]\n\n"+
			"Convert RAR archives to ZIP. By default writes <input>.zip alongside each input.\n\n"+
			"flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp { // -h / --help
			return 0
		}
		return 2
	}

	if showVersion {
		fmt.Printf("rar2zip v%s (%s)\n", version, commit)
		return 0
	}

	inputs := fs.Args()
	if code := validateArgs(inputs, output, outDir, jobs, store, level); code != 0 {
		return code
	}

	maxBytes, err := parseSize(maxSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rar2zip: invalid --max-size %q: %v\n", maxSize, err)
		return 2
	}
	if maxEntries < 0 {
		fmt.Fprintln(os.Stderr, "rar2zip: --max-entries must be >= 0")
		return 2
	}

	if list {
		return runList(inputs, output, outDir, password, maxEntries, jsonOut)
	}

	jobList, err := buildJobs(inputs, output, outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rar2zip: %v\n", err)
		return 2
	}

	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "rar2zip: %v\n", err)
			return 1
		}
	}

	opts := rarutil.Options{
		Password:      password,
		Force:         force,
		Store:         store,
		Level:         level,
		Verify:        verify,
		AllowFallback: allowFallback,
		MaxTotalBytes: maxBytes,
		MaxEntries:    maxEntries,
		SkipExisting:  skipExisting,
	}
	// Verbose diagnostics go to stderr, and only when stdout isn't owned by --json.
	if verbose && !jsonOut {
		opts.OnVerbose = func(msg string) {
			fmt.Fprintf(os.Stderr, "[verbose] %s\n", msg)
		}
	}
	// --json owns stdout for machine output, so silence the human decoration.
	human := !quiet && !jsonOut
	// Live per-entry progress only makes sense for a single archive. A batch now
	// runs concurrently by default, so a live per-job start line would interleave
	// nondeterministically across workers; batches instead rely on report()'s
	// final per-job lines, which are emitted in job order.
	if human && len(jobList) == 1 {
		n := 0
		opts.OnEntry = func(name string) {
			n++
			fmt.Fprintf(os.Stderr, "\r\033[K[%d] %s", n, name)
		}
	}

	results := rarutil.RunBatch(jobList, opts, jobs, nil)
	if jsonOut {
		return reportJSON(results, os.Stdout)
	}
	return report(results, quiet)
}

// report prints per-job outcomes and a batch summary, returning the aggregate
// exit code: 1 if any job failed, else 0.
func report(results []rarutil.Result, quiet bool) int {
	failed, skipped := 0, 0
	for _, r := range results {
		switch {
		case r.Err != nil:
			failed++
			fmt.Fprintf(os.Stderr, "rar2zip: %s: %v\n", r.Src, r.Err)
		case r.Skipped:
			skipped++
			if !quiet {
				fmt.Fprintf(os.Stderr, "skipped %s (output exists)\n", r.Dst)
			}
		default:
			fmt.Printf("wrote %s\n", r.Dst)
		}
	}

	if len(results) > 1 && !quiet {
		fmt.Fprintf(os.Stderr, "%d succeeded, %d skipped, %d failed\n", len(results)-failed-skipped, skipped, failed)
	} else if len(results) == 1 && !quiet {
		// Terminate the single-archive progress line.
		fmt.Fprintln(os.Stderr)
	}

	if failed > 0 {
		return 1
	}
	return 0
}
