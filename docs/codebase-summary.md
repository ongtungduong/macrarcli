# Codebase Summary

## High-Level Architecture

rar2zip is organized as a single-responsibility CLI with a pure-Go conversion engine:

```
rar2zip (CLI)
    ├── CLI input parsing & orchestration (main.go, cli_args.go)
    ├── Output formatting (json_output.go, list_output.go)
    └── Conversion Engine (internal/convert/)
```

The CLI layer handles flags, batch orchestration, and human output. The engine handles the actual RAR→ZIP conversion with bomb defense, Zip-Slip protection, and optional fallback to system tools.

## Package Layout

### `main` Package (Root)

**Responsibility**: CLI entry point, batch orchestration, human output formatting.

| File | Purpose | ~LOC |
|------|---------|------|
| `main.go` | Entry point, flag parsing, batch orchestration, progress reporting | ~200 |
| `cli_args.go` | Argument validation, job building, output path resolution, `--max-size` parsing | ~150 |
| `json_output.go` | Encode results as JSON for `--json` flag | ~80 |
| `list_output.go` | Format archive listings as human table or JSON for `--list` flag | ~100 |

**Key Functions**
- `defaultJobs()` — returns `min(NumCPU, 4)` for concurrent batch limit
- `newJob()` / `parseDestination()` — resolve output paths from `-o`/`--out-dir` flags
- `parseSize()` — parse `--max-size` with K/M/G suffixes
- `summaryJSON()` / `summaryTable()` — format result summaries

### `internal/convert` Package

**Responsibility**: RAR decode, ZIP encode, bomb caps, Zip-Slip defense, atomic writes, fallback tool integration.

**Design Principle**: Keep files focused and under ~200 lines each. Use interfaces to decouple concerns (e.g., `headerReader` for synthetic testing).

#### Core Files

| File | Purpose | ~LOC |
|------|---------|------|
| `convert.go` | Entry point `Convert()` — orchestrates native decode, shared emitter, atomic finalization, fallback flow | ~100 |
| `emit.go` | `zipEmitter` — shared ZIP write path, bomb caps enforcement, post-sanitize dedup, pooled buffers | ~180 |
| `sanitize.go` | `sanitize()` (Zip-Slip defense), `safeMode()` (Unix permission hardening) | ~80 |
| `fallback.go` | System tool fallback (`unrar`/`7z`), temp dir extraction, argv hardening, symlink neutralization | ~120 |
| `batch.go` | `RunBatch()` — bounded concurrency fan-out, continue-on-error, ordered result return | ~60 |
| `verify.go` | `verify()` — reopen ZIP, check entry count/sizes, force CRC32 validation per entry | ~70 |
| `list.go` | `List()` — read-only header iteration, same bomb caps, no fallback | ~90 |
| `compress.go` | `entryMethod()` (Store vs Deflate), `registerCompressor()` (compression levels 1-9) | ~50 |
| `freespace_unix.go` / `freespace_other.go` | `availableBytes()` — platform-specific free-space check for fallback pre-flight | ~40 each |

#### Shared Constants & Types

| Item | Purpose |
|------|---------|
| `errLimitBytes` / `errLimitEntries` | Decompression bomb thresholds from `--max-size` / `--max-entries` |
| `Job` | Input/output paths + flags (password, compression, etc.) |
| `Result` / `ErrSkipped` | Per-archive result status |
| `cappedWriter` | I/O wrapper enforcing write-size limits |
| `resolveName()` / `dedupVariant()` | Entry-name dedup logic (rename repeats, error on collisions) |

## Data Flow: Single Archive Conversion

```
1. main.go: Parse CLI flags, build Job(inputPath, outputPath, flags)
2. convert.Convert(job):
   a. Check destination writable, doesn't exist (unless --force)
   b. Try convertNative():
      - Open RAR via rardecode
      - For each entry:
        * sanitize(name) → Zip-Slip defense
        * check bomb caps (size/entry count)
        * safeMode(perms) → strip dangerous bits
        * stream to temp ZIP file via emitter
      - Finalize: atomic os.Rename(tempFile, destination)
   c. If native fails and --allow-fallback:
      - Check free space in TMPDIR (early exit if too tight)
      - Shell out to unrar/7z, extract to temp dir
      - Walk temp dir, same sanitization/emitter/rename flow
   d. Return result (success/error)
3. If --verify:
   - verify(): reopen output ZIP, iterate all entries, read to EOF (forces CRC check)
4. Format result (human or JSON), print summary
```

## Key Abstractions

### `headerReader` Interface

```go
type headerReader interface {
    Next() (*tar.Header, error)
}
```

Allows synthetic testing of sanitization and bomb-cap logic without real RAR files (which are proprietary and can't be generated).

### `zipEmitter` Struct

Centralizes the ZIP write path for native and fallback flows:
- Enforces `--max-size` / `--max-entries` caps
- Applies `sanitize()` to all entry names
- Handles per-name dedup via `resolveName()`
- Uses a pooled 512 KB copy buffer

This shared emitter ensures both paths have identical decompression-bomb and Zip-Slip protection.

### Atomic Write Pattern

All paths follow the same pattern:
```go
1. Open temp file in output directory (same filesystem for atomic rename)
2. Write ZIP to temp via emitter
3. os.Chmod(tempFile, 0644)
4. os.Rename(tempFile, destination) — atomic
5. If any error before step 4: os.Remove(tempFile)
```

Result: destination is never truncated or partial; failed conversions leave no trace.

## Security Invariants

These must be preserved in any change to `emit.go`, `sanitize.go`, or `fallback.go`:

1. **Zip-Slip defense**: Every entry name passes `sanitize()` before packing
   - Rejects empty, absolute, or traversal names (`..`, `../../etc/passwd`)
   - Applies both on native and fallback paths

2. **Symlink/device neutralization**: `safeMode()` strips S_IFLNK, S_IFBLK, S_IFCHR
   - Symlink targets stored as file content instead of links
   - Devices skipped entirely

3. **Decompression-bomb caps**: Applied at stream time via `cappedWriter`
   - `--max-size`: abort if total uncompressed bytes exceed limit
   - `--max-entries`: abort if entry count exceeds limit
   - Also bounds `--list` (read-only path)
   - **Gap**: `--allow-fallback` extracts entire archive BEFORE caps apply (documented in README)

4. **Post-sanitize collision guard**: `dedupVariant()` prevents silent data loss
   - Same raw name (e.g., multi-volume re-streaming): renamed to `name (1)`, `name (2)`, ...
   - Different raw names colliding after sanitization: hard error (indicates hostile input)

5. **Atomic writes**: Never partial/truncated outputs
   - Write to temp file, then atomic rename
   - Any failure removes temp

## Testing Approach

**Fixture-gated tests**: Core logic tested via interface seams + synthetic inputs. Real RAR fixtures (if present in `testdata/`) are converted end-to-end and verified entry-by-entry.

**Coverage areas**:
- Sanitization (traversal names, absolute paths, symlinks)
- Bomb caps enforcement
- Fallback tool argv hardening
- Batch concurrency & ordering
- CRC32 verification
- ZIP64 handling

**Benchmark hot paths**: Large-entry streaming, `--verify` over many entries, native convert (with fixtures).

## Dependency Map

**Production**
- `github.com/nwaples/rardecode/v2` — pure-Go RAR decode (native path only)
- Go standard library (`archive/zip`, `io`, `os`, `flag`, `encoding/json`, etc.)

**Development/Optional**
- `unrar` / `7z` — external tools, only for `--allow-fallback` and fallback tests
- Fixtures in `testdata/*.rar` — optional, tests skip if absent

**No dynamic imports** — all dependencies are declared upfront.

## Module Info

- **Module**: `github.com/ongtungduong/rar2zip`
- **Go version**: 1.26.2 (see `go.mod`)
- **Binary name**: `rar2zip`
- **Latest version**: 0.2.1 (2026-06-18)
