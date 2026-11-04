# Codebase Summary

macrarcli is a pure-Go RAR CLI: extract with structure, extract flat, list,
and test integrity. There is no shell-out decoder or ZIP output.

## Packages

| Area | Files | Responsibility |
|------|-------|----------------|
| CLI | `main.go`, `cli_args.go` | Flags, password probes, ordered progress, batch dispatch, exit codes |
| Output | `json_output.go`, `list_output.go` | JSON results and control-character-safe human output |
| Decoder | `extract.go`, `list.go`, `test.go`, `reader_options.go` | Shared password/dictionary options and archive iteration |
| Staging | `writer.go`, `names.go`, `limits.go`, `sanitize.go` | Exclusive creation, duplicate suffixes, byte caps, safe paths/modes |
| Commit | `stage.go`, `overwrite.go`, `publish.go`, `rollback.go` | Root-confined publication, collision policies, overwrite backups/recovery |
| Metadata | `filetimes_*.go` | Apply modification time through an open file handle |
| Utilities | `batch.go`, `password.go`, `progress.go` | Bounded concurrency, terminal prompt, progress arithmetic |

## Extraction flow

1. Resolve password once. When needed, inspect every input's header and close
   each successful probe immediately.
2. Open the destination with `os.OpenRoot`; create a private staging directory
   under that root and borrow its opened root for decoding.
3. Sanitize every entry name before flattening. Strip special modes and
   group/other-write permissions. Stream files with `io.Copy` through the byte
   limiter; the decoder's `WriterTo` handles its own buffers.
4. Create staged files exclusively. Directory spelling tracking also rejects
   filesystem aliases, including implicit parents. Cache duplicate suffixes.
5. After successful decoding, serialize commits across in-process jobs.
   Back up overwritten files/symlinks, then publish files using hard links or
   an exclusive-create copy fallback. All paths remain relative to `os.Root`.
6. On commit failure, remove this run's published files and restore backups.
   Report rollback errors and preserve originals that need manual recovery.
7. Clean up staging through the pinned root; format ordered results.

## Limits and contracts

- CLI defaults: 20 GiB decoded output, 200,000 entries, 256 MiB dictionary per
  decoder, and `min(NumCPU, 4)` jobs. Dictionary size must be positive at the CLI;
  zero-value backend `Options` selects the 256 MiB default.
- Byte caps apply to extract/test. Entry and dictionary caps apply to all modes.
  Flat-mode directories consume the entry budget even though they are not
  written and do not fire file-progress callbacks.
- `List` remains header iteration. Solid archives may decompress while advancing;
  neither listing nor the progress pre-pass has a CPU/time cap.
- Progress follows archive order, so renamed/flattened names and duplicate sizes
  cannot corrupt the percentage. Unknown/overflowing size totals use entry counts.
- JSON preserves raw strings with JSON escaping. Human output replaces control
  characters; do not reuse raw JSON names directly in a terminal.
- No crash-atomicity, durability, external-process isolation, or exact directory
  metadata restoration is promised. Exclusive-copy publication can be visible
  while incomplete. Recovery backups remain when restoration fails.
- Windows support remains experimental. Unix tests exercise symlinks; Windows
  builds are checked separately.

## Testing and dependencies

Tests use fake readers, synthetic RAR5 headers, real filesystem collision tests,
rollback failures, pinned-root path replacement, and duplicate-name benchmarks.
Optional fixture-based integration tests skip when fixtures are absent.

Runtime dependencies are `rardecode/v2`, `x/term`, and `x/sys`. Go 1.26.10 is the
minimum toolchain, including fixes for GO-2026-4970 and GO-2026-6604. See
`docs/project-changelog.md` for release changes. Version 0.5.0 (2026-11-04)
contains the security and performance hardening described here.
