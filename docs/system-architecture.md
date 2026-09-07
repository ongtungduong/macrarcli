# System Architecture

## Component Diagram

```
User / CLI
    ↓
main.go (Flag parsing, mode selection, batch orchestration)
    ↓
cli_args.go (Argument validation, job building)
    ↓
┌─────────────────────────────────────────────┐
│ Dispatch by Mode (-l, -t, extract, or flat) │
└─────────────────────────────────────────────┘
    ↙           ↓           ↓           ↘
List()      Test()    Extract()      Extract()
(read-only)  (r/o)     (preserve)      (flat)
    ↓           ↓           ↓           ↓
rardecode    rardecode   stage.go      stage.go
(open, iter) (validate)  extract.go    extract.go
    ↓           ↓           ↓           ↓
    ├──→ sanitize.go (Zip-Slip defense) ←──┤
    │   safeMode() (Permission hardening)   │
    │   writer.go (Bomb-cap enforcement)    │
    └────────────────────────────────────────┘
            ↓
    os.Rename() — atomic commit
            ↓
    Output formatting (JSON or human)
            ↓
    Exit code: 0/1/2/3/4
```

## Data Flow: Single Archive

```
1. CLI Parsing (main.go, cli_args.go)
   ├─ Parse flags: -o, -e, -l, -t, --password, --max-size, --jobs, etc.
   ├─ Validate arguments (at least one input, mode exclusion, job count)
   ├─ Resolve password once (TTY prompt if needed, before batch dispatch)
   └─ Build Job{Src, Dst, ...}

2. Mode Dispatch
   ├─ -l (List): rarutil.List()
   │  ├─ Open RAR read-only
   │  ├─ Iterate headers
   │  ├─ Apply --max-entries cap
   │  └─ Return []EntryInfo with name, size, packed size, encrypted, modtime
   │
   ├─ -t (Test): rarutil.Test()
   │  ├─ Open RAR
   │  ├─ Iterate entries via cappedWriter (enforces --max-size)
   │  ├─ Read to EOF to force CRC32 validation
   │  └─ Return success or corruption error
   │
   └─ Extract (default, -e for flat): rarutil.Extract()
      ├─ makeStagingDir() → create private temp directory
      ├─ extractToStaging():
      │  ├─ Open RAR via rardecode.OpenReader()
      │  ├─ For each entry:
      │  │  ├─ sanitize(name) → reject traversal/absolute paths (Zip-Slip)
      │  │  ├─ safeMode(mode) → strip symlink/device/setuid bits
      │  │  ├─ If Flat: name = path.Base(name)
      │  │  └─ Stream to staging via cappedWriter (--max-size enforcement)
      │  └─ Any error: return immediately, Extract cleans up defer
      │
      ├─ commitStaged():
      │  ├─ Walk staged entries
      │  ├─ For each: check destination collision
      │  ├─ Per overwrite policy:
      │  │  ├─ Fail (default): error if exists
      │  │  ├─ Overwrite: Replace existing
      │  │  ├─ Skip: Leave destination, track as skipped
      │  │  └─ Rename: Keep both, suffix collider with " (n)"
      │  ├─ os.Rename() each entry into destination (atomic per-entry)
      │  └─ Return []string of skipped names
      │
      └─ defer os.RemoveAll(stagingDir) on any error

3. Output Formatting
   ├─ If --json: reportJSON() or reportTestJSON() to stdout
   ├─ If human: Print per-entry status to stdout/stderr
   └─ Batch summary: "{N} succeeded, {M} partial, {F} failed"

4. Exit Code Aggregation
   ├─ Per-job: classifyErr() → 0/1/2/3/4
   ├─ Batch: aggregateExit() → highest priority (2 > 3 > 4)
   └─ Return process exit code
```

## Batch Processing Flow

```
Multiple input files (*.rar)
    ↓
RunBatch(jobs []Job, opts, maxConcurrent)
    ├─ Create semaphore with maxConcurrent slots
    ├─ For each Job (concurrently up to limit):
    │  ├─ Acquire semaphore slot
    │  ├─ Call Extract(job.Src, job.Dst, opts)
    │  ├─ Store result in order (maintain input order)
    │  └─ Release semaphore slot
    └─ Return results []Result in input order

Key properties:
• Concurrent by default: --jobs defaults to min(NumCPU, 4)
• Continue-on-error: failed Job doesn't abort batch
• Deterministic output: results printed in input order, not completion order
• Exit code: highest-priority error code wins (2 > 3 > 4)
```

## Security Invariants

These must be preserved in any change affecting extraction:

### 1. Zip-Slip Defense

Every entry name sanitized before staging:
- `sanitize()` rejects: empty names, absolute paths (Unix or Windows), `..` patterns
- Applied to both extract and list modes
- No exception for flat extraction (base name of path is already sanitized)

**Verification**: Read `sanitize.go`, check all `rr.Next()` callers pass through `sanitize()`

### 2. Symlink/Device Neutralization

`safeMode()` strips dangerous mode bits:
- `S_IFLNK` (symlinks stored as regular files)
- `S_IFBLK`, `S_IFCHR` (block/character devices skipped)
- `S_ISUID`, `S_ISGID`, `S_ISVTX` (setuid/setgid/sticky removed)

**Rationale**: Symlink with unsanitized target could escape extraction root; devices bypass filesystem checks.

**Verification**: All `emit()` calls in `extractToStaging()` pass through `safeMode()`

### 3. Decompression-Bomb Enforcement

Enforced at two points:

**Stream-time** (per-entry size):
- `cappedWriter` in `writer.go` wraps destination
- Each `Write()` accumulates bytes
- If total exceeds `--max-size`, return error, abandon entry
- No partial data reaches disk

**Header-time** (entry count):
- During `listEntries()` iteration, check `len(entries) >= maxEntries`
- If cap hit, return partial list + `errLimitEntries`
- Early exit before allocation spirals

**Gaps**: None on native path. Documented limit: entries staged before cap-check are written; cap enforces stream-time cutoff, not pre-staging validation.

### 4. Atomic Writes

Pattern applied universally:
1. Open temp file in destination directory (same filesystem)
2. Write entry to staging or ZIP
3. On entry success: `os.Rename(staged, destination)` — atomic
4. On any error: `defer os.RemoveAll(stagingDir)` cleans up

**Guarantees**: Destination is never truncated or partial; failed extraction leaves no trace.

### 5. Post-Sanitize Collision Guard

Collision handling via overwrite policies:
- **Same raw name repeated** (multi-volume re-streaming): Renamed to `name (1)`, `name (2)`, …
- **Different raw names colliding after sanitization**: Hard error (indicates hostile input, e.g., `a/b` + `a%2Fb` both sanitizing to `a/b`)

**Verification**: Read `stage.go`, check dedup logic in `commitStaged()`

## Mode Semantics

| Mode | Flag | Behavior | Writes | Exit Code |
|------|------|----------|--------|-----------|
| Extract | (default) | Preserve directory structure | Yes | 0/1/2/3/4 |
| Flat | -e, --flat | Extract to single directory | Yes | 0/1/2/3/4 |
| List | -l, --list | Preview contents only | No | 0/1/2/3/4 |
| Test | -t, --test | Validate integrity only | No | 0/1/2/3/4 |

**Mutual exclusion**: Only one of -e, -l, -t may be specified; all are mutually exclusive with write flags.

## Overwrite Policies

Determines behavior when destination file exists:

| Policy | Behavior |
|--------|----------|
| OverwriteFail (default) | Error immediately, abort this archive |
| OverwriteOverwrite | Replace destination with staged entry |
| OverwriteSkip | Keep destination, skip this entry, track skipped count |
| OverwriteRename | Keep destination, rename entry to `name (n)` |

**Mutual exclusion**: Only one of `--overwrite`, `--skip`, `--rename` may be set.

## Password Handling

Password resolved **once per invocation**, before batch dispatch:

1. If `--password` provided: use it
2. If encrypted archive detected and no password: prompt on TTY (masked input)
3. If non-TTY (CI, pipe): fail fast with clear error
4. Single resolved password reused for all archives in batch

**Rationale**: Prevents multiple TTY prompts in batch; prevents concurrent password races on shared stdin.

## Error Propagation

Errors classified into exit codes (per `main.go:classifyErr`):

| Code | Meaning | rardecode Sentinel |
|------|---------|-------------------|
| 0 | Success | — |
| 1 | Usage error | — |
| 2 | Wrong/missing password | `ErrBadPassword` (RAR5 only) |
| 3 | Corrupted entry | `ErrBadFileChecksum` |
| 4 | Other runtime error | — |

**Batch aggregation** (per `aggregateExit`): Highest-priority code wins (2 > 3 > 4).

**Known gap**: Legacy RAR3/4 archives with wrong passwords may report exit 3 (indistinguishable from corruption in `rardecode`). This is a library limitation, not a bug.
