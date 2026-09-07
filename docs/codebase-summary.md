# Codebase Summary

## High-Level Architecture

macrarcli is organized as a single-responsibility CLI with a pure-Go extraction engine:

```
macrarcli (CLI)
├── CLI input parsing & orchestration (main.go, cli_args.go)
├── Output formatting (json_output.go, list_output.go)
└── Extraction Engine (internal/rarutil/)
    ├── Core operations: extract, list, test (integrity)
    ├── Security: sanitization, staging, atomic commits
    └── Utilities: password resolution, progress tracking, batch orchestration
```

The CLI layer handles flags, batch orchestration, and output formatting. The engine handles RAR decoding with decompression-bomb defense, Zip-Slip protection, and atomic write semantics.

## Package Layout

### `main` Package (Root)

**Responsibility**: CLI entry point, flag parsing, batch orchestration, output formatting.

| File | Purpose | ~LOC |
|------|---------|------|
| `main.go` | Entry point, flag parsing, mode selection, password resolution, batch dispatch | ~220 |
| `cli_args.go` | Argument validation, mode/overwrite policy enforcement, size parsing | ~130 |
| `json_output.go` | JSON formatting for results and listings | ~60 |
| `list_output.go` | Table formatting for archive listings | ~50 |

**Key Functions**
- `run()` — orchestrates mode dispatch and error aggregation
- `resolveMode()` — enforces -e/-l/-t mutual exclusion
- `resolveOverwritePolicy()` — enforces overwrite flag mutual exclusion
- `parseSize()` — parses `--max-size` with K/M/G suffixes
- `attachProgress()` — wires live progress display for single-archive runs

### `internal/rarutil` Package

**Responsibility**: RAR decoding, entry validation, staging-directory extraction, atomic commit, decompression-bomb enforcement, Zip-Slip defense.

**Design Principle**: Keep concerns focused. Use interfaces to enable synthetic testing without real RAR files (which are proprietary).

#### Core Files

| File | Purpose | ~LOC |
|------|---------|------|
| `extract.go` | `Extract()` — stages to temp dir, commits via atomic rename, rolls back on failure | ~80 |
| `stage.go` | `commitStaged()` — per-entry mv/collision handling, `makeStagingDir()` | ~170 |
| `list.go` | `List()` — read-only header iteration with bomb-cap bounding | ~80 |
| `test.go` | `Test()` — checksum-only validation without writes | ~70 |
| `writer.go` | `cappedWriter` — stream decompression enforcement; respects `--max-size` | ~130 |
| `sanitize.go` | `sanitize()` (Zip-Slip defense), `safeMode()` (permission hardening) | ~50 |
| `password.go` | `ResolvePassword()`, `ResolvePasswordStdin()` — TTY prompt or flag | ~60 |
| `progress.go` | `ProgressTracker` — live throughput/percentage calculation | ~60 |
| `batch.go` | `RunBatch()`, `TestBatch()`, `ListBatch()` — bounded concurrency, continue-on-error, ordered results | ~86 |
| `overwrite.go` | `OverwritePolicy` enum + enforcement (fail/overwrite/skip/rename) | ~80 |

#### Shared Types & Constants

| Item | Purpose |
|------|---------|
| `Options` | Configuration tuple: password, overwrite policy, bomb caps, flat mode |
| `Result`, `Job` | Per-archive result and input specification |
| `OverwritePolicy` enum | Collision handling: Fail, Overwrite, Skip, Rename |
| `EntryInfo` | Read-only listing data (name, size, packed size, encrypted, mod time) |
| `ProgressTracker` | Percentage + throughput calculation from processed size/entry count |
| `cappedWriter` | Stream wrapper enforcing decompression-bomb size cap |

## Data Flow: Single Archive Extraction

```
1. main.go: Parse flags, build Job(src, dst, opts)
2. Extract(srcRar, destDir, opts):
   a. makeStagingDir() → create private temp directory
   b. extractToStaging():
      - Open RAR via rardecode
      - For each entry:
        * sanitize(name) → Zip-Slip defense (reject traversal, absolute paths)
        * safeMode(perms) → strip dangerous bits (symlinks, devices)
        * emit to staging via cappedWriter (enforces --max-size)
        * callback OnEntry for progress
      - On any error: return, let Extract clean up
   c. commitStaged(stagingDir, destDir, opts):
      - Walk staged entries
      - Check destination collisions
      - Per-entry os.Rename() to destination
      - On collision per overwrite policy: fail, overwrite, skip, or rename
      - Return list of skipped entries on OverwriteSkip
   d. defer os.RemoveAll(stagingDir) cleans up on exit
3. Format result (human or JSON), print summary
```

## Key Abstractions

### Staging Directory Pattern

All extraction goes to a private temp directory first, then atomically committed:
- **Advantages**: Clean separation of concerns, safe rollback on any error, no partial destination
- **Guarantees**: Destination is never partial/truncated; failed extraction leaves no trace
- **Implementation**: `makeStagingDir()` creates `.macrarcli-staging-*` in destination; each entry streamed to staging via `cappedWriter`; `commitStaged()` then per-entry renames entries into place

### `cappedWriter` Stream Enforcement

```go
type cappedWriter struct {
    w         io.Writer
    cap       int64
    written   int64
}
```

Wraps the destination stream and enforces `--max-size` at write time:
- Each Write() adds to total
- If total exceeds cap, Write() returns error
- Entry is abandoned, no partial write reaches disk

### Sanitization & Collision Guard

- `sanitize(name)` rejects: empty, absolute, `..`, `../../etc/passwd` (Zip-Slip defense)
- `safeMode(mode)` strips: `S_IFLNK`, `S_IFBLK`, `S_IFCHR`, setuid/setgid/sticky bits
- Post-sanitize collision handling: same name repeated renamed to `(1)`, `(2)`, …; different names colliding after sanitization is a hard error

### OverwritePolicy Enum

Determines destination collision behavior:
- **OverwriteFail** (0): Error if destination exists (default, fail-closed)
- **OverwriteOverwrite**: Replace existing file
- **OverwriteSkip**: Silently skip entry, continue batch
- **OverwriteRename**: Keep both, rename collider to `name (n)`

## Batch Processing

Three batch functions share a common pattern (bounded concurrency, continue-on-error, ordered results):

- `RunBatch(jobs []Job, opts, maxParallel, onStart)`: Extracts each job via `Extract()`
- `TestBatch(srcs []string, opts, maxParallel)`: Validates each archive via `Test()`
- `ListBatch(srcs []string, opts, maxParallel)`: Lists each archive via `List()`

All three:
- Dispatch to a bounded worker pool (semaphore) with `maxParallel` slots
- Collect results in input order (not completion order) for deterministic output
- Continue on error: failed input doesn't abort the batch
- Return results matching input order

## Security Invariants

Must be preserved in any change to `sanitize.go`, `writer.go`, `stage.go`:

1. **Zip-Slip defense**: Every entry name passes `sanitize()` before write
   - Rejects empty, absolute, traversal names
   - Applies uniformly to all modes (extract, list, test)

2. **Symlink/device neutralization**: `safeMode()` strips dangerous mode bits
   - Symlink entries stored as regular files (content preserved, links defeated)
   - Device entries skipped (cannot traverse filesystem)

3. **Decompression-bomb caps**: Enforced at write time via `cappedWriter`
   - `--max-size`: abort if total uncompressed bytes exceed limit
   - `--max-entries`: abort if entry count exceeds limit (checked per-header iteration)
   - Also bounds `--list` and `--test` (read-only paths)

4. **Atomic writes**: Never partial/truncated outputs
   - Extract to staging dir first
   - Each entry only committed via os.Rename on success
   - Any error before final commit removes staging, leaves destination unchanged

5. **Post-sanitize collision guard**: `dedupVariant()` prevents data loss
   - Same archive entry (multi-volume re-streaming): renamed to `(1)`, `(2)`, …
   - Different entries colliding after sanitization: hard error (hostile input)

## Testing Approach

**Interface-driven testing**: Core logic (sanitization, bomb caps) tested via interface seams + synthetic inputs. No real RAR files required for critical tests.

**Coverage areas**:
- Sanitization (traversal, absolute paths, encoding)
- Bomb-cap enforcement (entry count, total size)
- Batch concurrency (ordering, continue-on-error)
- Overwrite policies (collision handling)
- Password resolution (TTY, flag, environment)

**Fixture-gated tests**: If `testdata/*.rar` exists, end-to-end extraction tests verify real archives.

## Dependency Map

**Production**
- `github.com/nwaples/rardecode/v2` — pure-Go RAR decoder
- `golang.org/x/term` — masked password TTY prompt
- Go standard library (`io`, `os`, `flag`, `path`, `encoding/json`, etc.)

**Development/Testing**
- Test fixtures in `testdata/` (optional, tests skip if absent)

**No dynamic imports** — all dependencies declared in `go.mod`.

## Module Info

- **Module**: `github.com/ongtungduong/macrarcli`
- **Go version**: 1.26.2
- **Binary name**: `macrarcli`
- **Latest version**: 0.3.0 (2026-09-07)
