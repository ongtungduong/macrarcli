# Changelog

Notable changes to macrarcli (formerly rar2zip). Behavior changes that affect
observable output or defaults are called out explicitly.

## Unreleased

## 0.4.0 — 2026-09-07

### Hardened
- **`--max-size`/`--max-entries` now default to `20G`/`200000` instead of
  unlimited.** Behavior change: a script relying on the previous unbounded
  default must now pass `--max-size 0 --max-entries 0` explicitly to keep
  running unlimited. Closes a decompression-bomb gap where a small crafted
  archive could exhaust disk or inodes with no flag required to trigger it.
- **Extracted file permission bits are capped.** An archive can no longer make
  an extracted file group- or other-writable (mode is masked with `&^0o022`
  after type-bit stripping), regardless of what mode it recorded — previously
  an archive entry with mode `0777` extracted as world-writable verbatim.
- **`$MACRARCLI_PASSWORD`** is now a fallback for `--password` (flag still
  wins if set), so scripted/automated use no longer has to put the secret in
  argv, which is visible to other local users via `ps`/`/proc/<pid>/cmdline`
  for the life of the process.
- The cross-filesystem rename fallback (`stage.go`, only reached when staging
  and the destination are on different filesystems) now opens its destination
  with `O_NOFOLLOW` on unix, closing a narrow TOCTOU window where a symlink
  planted at the destination path between the existence check and the open
  could otherwise be written through.

### Changed
- **`-l`/`--list` and `-t`/`--test` now honor `--jobs`.** Previously only
  extraction ran a multi-archive batch concurrently; list/test always ran
  sequentially regardless of `--jobs`. Per-archive result lines are still
  printed in input order. `-l`'s internal signature also now takes the same
  `Options` every other mode does, so a future option reaches list mode
  automatically instead of needing separate plumbing.

## 0.3.0 — 2026-09-07

### Breaking — pivot from ZIP conversion to direct RAR extraction

This release renames the project and changes what it does: `rar2zip`
converted RAR to ZIP; `macrarcli` extracts RAR archives directly. No ZIP
output.

- **Module and binary renamed.** `github.com/ongtungduong/rar2zip` ->
  `github.com/ongtungduong/macrarcli`; the built binary is now `macrarcli`.
  Clean cutover, no compatibility shim or deprecated formula.
- **New command surface.** `-e`/`--flat` (extract flat), `-l`/`--list`
  (preview, read-only), `-t`/`--test` (checksum-only validation, read-only);
  no mode flag extracts preserving directory structure (the new default —
  previously the default was ZIP conversion). `-o`/`--output`/`--out-dir`
  collapse into a single `-o`/`--dest <dir>` naming a destination directory.
  `--overwrite`/`--skip`/`--rename` replace `-f`/`--force` and
  `--skip-existing` as three mutually-exclusive per-entry collision policies.
- **`--store`, `--level`, `--verify`, `--allow-fallback` removed.** All were
  ZIP-specific or tied to the removed system-`unrar`/`7z` fallback path,
  which no longer exists — the pure-Go decoder (`nwaples/rardecode/v2`)
  already covers RAR5, and removing the fallback also removes its documented
  "not bomb-bounded" weakness.
- **New 5-code exit scheme.** `0` success / `1` usage error / `2` wrong-or-
  missing password / `3` corrupted entry / `4` other runtime error, replacing
  the previous `0`/`1`/`2` scheme. Exit `2` is only reliably guaranteed for
  RAR5-encrypted archives — `rardecode` has no equivalent sentinel for legacy
  RAR3/4, so a wrong password there may report exit `3` instead (documented
  limitation, not a bug — see `README.md`).
- **Extraction is now atomic per archive.** Entries are staged in a private
  temporary directory and committed into the destination only once the whole
  archive has decoded successfully; a failure rolls back the entire staging
  directory instead of leaving a partial extraction.
- **New:** `-t`/`--test` (checksum-only validation), password prompting
  (masked, via `golang.org/x/term` — the one new runtime dependency), and a
  live single-archive progress line (`[%] name (throughput/s)`). `-l`/`--list`
  now also reports each entry's compressed size and encrypted flag.

## 0.2.1 — 2026-06-18

### Added
- `--skip-existing` — in a batch, skip inputs whose output already exists
  (reported as skipped, not failed) instead of erroring. The `--json` summary
  gains a `skipped` count. Advisory pre-check, not a collision-safety mechanism.
- `--verbose` — print extra diagnostics to stderr (which decode path ran:
  native vs fallback, and per-archive timing). Suppressed under `--json`/`--quiet`.
- Shell completion scripts for bash, zsh, and fish under `completions/` (also
  bundled in each release archive).
- Contributor docs: `CONTRIBUTING.md` and `docs/TROUBLESHOOTING.md`.
- **Windows support (experimental).** Releases now publish Windows `amd64`/`arm64`
  binaries as `.zip` assets, with a Scoop bucket manifest. A Windows CI row builds
  and vets the code (the test suite is Unix-only — the `--allow-fallback` path
  shells out to `unrar`/`7z` and creates symlinks). See README "Windows (Scoop)".
- **Release signing.** `checksums.txt` is signed with keyless `cosign` (Sigstore,
  via Actions OIDC — no stored keys). `install.sh` verifies the signature when
  `cosign` is installed (authenticity), in addition to the mandatory checksum.
- `--list` — preview an archive's entries (size, modification time, name) without
  converting (read-only; writes no ZIP). Honors `--json`, `--password`, and the
  `--max-entries` cap; rejects `-o`/`--out-dir`. Human output strips control
  characters from entry names to prevent terminal-escape injection from hostile
  archives (`--json` output is already escaped by the encoder).
- Benchmarks for the hot paths (`go test -bench .` in `internal/convert`):
  large-entry streaming write, `--verify` over many entries, and a fixture-gated
  native convert.
- ZIP64 round-trip coverage: entries past the 4 GiB boundary are verified to
  re-pack and `--verify` with correct 64-bit size accounting (exercised on the
  fallback path; gated behind `-short`).

### Changed
- **`--verify` now validates content CRC, not just structure.** It reads every
  entry to force the ZIP reader's CRC32 check, so a same-size-but-corrupted entry
  is caught — previously `--verify` checked only entry count and declared sizes.
- **`--jobs` now defaults to `min(NumCPU, 4)` instead of `1`.** This is a
  behavior change for any invocation with two or more inputs (e.g. a `*.rar`
  glob): the batch now runs **concurrently by default**. Per-archive result
  lines are still emitted in input order, so stdout/stderr stay deterministic;
  the previous live per-job "converting X" start line (which would interleave
  nondeterministically across workers) was removed in favor of the ordered final
  summary. Pass `--jobs 1` to restore fully serial execution.
- Large-entry streaming now uses a pooled 512 KB copy buffer (`io.CopyBuffer`)
  instead of `io.Copy`'s default 32 KB, reducing syscall frequency and
  per-entry allocations on big files (measured ~7% throughput gain and ~60–85%
  fewer bytes allocated on the large-entry write path). Memory stays bounded —
  buffers are pooled and shared across concurrent jobs.

### Hardened
- The `--allow-fallback` path now performs a lightweight free-space pre-check
  before extracting: if the temp filesystem has less free space than the archive
  itself, it fails early with a clear message (pointing at `TMPDIR`) rather than
  extracting partway and hitting an opaque out-of-space error. Extraction stays
  in the system temp dir (it is not relocated onto the output volume). The check
  is a conservative floor, not a full reservation — the external extractor's own
  out-of-space error remains the hard bound. Skipped on platforms without
  `statfs`.

### Cut
- `--encoding` (filename charset override) was evaluated and cut: the pure-Go
  decoder exposes only already-decoded entry names and discards the raw bytes /
  stored codepage, so byte-accurate transcoding is infeasible, and re-decoding
  the decoded string would be both lossy and a path-traversal risk. See the
  "Filename encoding" section in `README.md`.
