# Project Overview & Product Development Requirements

## Vision

macrarcli is a lightweight, self-contained CLI tool for extracting RAR archives directly — no ZIP conversion, no external tool dependencies. It prioritizes simplicity, speed, and security hardening by using a pure-Go RAR decoder.

## Purpose

Provide macOS and Linux users with a fast, dependency-free way to extract RAR archives, list contents, or validate integrity — all from a single compiled binary.

## Target Users

- macOS/Linux users with RAR archives who prefer self-contained tools
- Developers and system administrators automating archive extraction
- Users extracting large or untrusted archives (security hardening)
- Batch processing workflows needing concurrent extraction

## Problem Statement

RAR archives are common but extraction requires external tools (`unrar` / `7z`) or GUI applications. Existing solutions either:
- Require runtime dependencies (tool bloat, maintenance burden)
- Are closed-source or platform-specific
- Lack security hardening for untrusted archives

macrarcli solves this with a pure-Go CLI, zero runtime dependencies, and explicit security design.

## Scope & Goals

### In Scope

**Functional**
- Extract single or batch RAR archives (preserving structure or flat)
- Support password-protected archives
- List archive contents without extracting
- Validate integrity without extracting
- Preserve directory structure, timestamps, and Unix permissions
- Multi-volume RAR sets (`.part1.rar`, `.r00`, etc.)
- Controllable concurrency (default `min(NumCPU, 4)`)
- Decompression-bomb defense (`--max-size`, `--max-entries`)
- Overwrite policies (fail-closed, overwrite, skip, rename)
- JSON output mode for automation
- Shell completion scripts

**Non-Functional**
- No external runtime dependencies on native path
- Atomic writes (temp + rename, never partial)
- Bounded memory usage (pooled buffers per job)
- Hardened against Zip-Slip, path traversal, symlink/device attacks
- Keyless code signing (Sigstore/cosign) for release authenticity
- Cross-platform availability (Homebrew, install script, Scoop)

### Out of Scope (Explicit YAGNI)

- Stdin/stdout streaming
- ZIP output or format conversion
- RAR5-only archives (RAR3/4 focus via `rardecode/v2`)
- Per-file include/exclude filters
- Filename encoding overrides
- Path restructuring/flattening beyond directory structure preservation
- Comment metadata preservation
- Parallel compression within archive
- Docker image, deb/rpm packaging, winget/asdf

## Success Criteria

1. **Correctness**: Preserve file content, names, timestamps, and permissions exactly
2. **Security**: Defend against Zip-Slip, decompression bombs, symlink/device attacks
3. **Performance**: Handle multi-GiB archives efficiently with bounded memory
4. **Usability**: Single-command CLI with clear error messages and deterministic output
5. **Reliability**: Atomic writes; failed extractions never touch destination
6. **Distribution**: Available via Homebrew, install script, and Scoop (Windows experimental)

## Key Features

| Feature | Status | Notes |
|---------|--------|-------|
| Pure-Go RAR extraction | ✅ | Via `nwaples/rardecode/v2` |
| Preserve directory structure | ✅ | Default behavior |
| Flat extraction (-e/--flat) | ✅ | Extract without subdirectories |
| Archive preview (-l/--list) | ✅ | Read-only, shows sizes + encryption status |
| Integrity validation (-t/--test) | ✅ | Checksum validation without writes |
| Password support | ✅ | TTY prompt or `--password` flag |
| Multi-volume handling | ✅ | Auto-follows `.part1.rar` chains |
| Batch processing | ✅ | Concurrent via `--jobs` (default 4) |
| Overwrite policies | ✅ | fail (default), overwrite, skip, rename |
| Decompression-bomb caps | ✅ | `--max-size`, `--max-entries` |
| JSON output | ✅ | For automation and tooling |
| Release signing | ✅ | Keyless cosign for checksums |
| Windows support | ⚠️ | Experimental (build + vet only in CI) |

## Version & Timeline

**Current Version**: 0.3.0 (2026-09-07) — macrarcli pivot release  
**Previous**: 0.2.1 (2026-06-18) — rar2zip (RAR-to-ZIP converter)  
**Maturity**: Stable for production use on native path

## Roadmap Status

The project recently pivoted from a RAR-to-ZIP converter (`rar2zip`) to direct RAR extraction (`macrarcli`). All core features are implemented and tested. Future work is driven by:
- User bug reports or feature requests
- Platform-specific support (Windows hardening)
- Dependency updates

## Exit Codes

Exit code reflects operation outcome:
- **0**: Success (all archives processed)
- **1**: Usage error (bad flags, no inputs, missing files)
- **2**: Wrong or missing password (RAR5-encrypted archives only; legacy RAR3/4 may report 3 instead)
- **3**: Corrupted entry (failed CRC32 or decompression)
- **4**: Other runtime error (I/O, no space, permissions)

For batches: highest-priority code wins (2 > 3 > 4).

## Known Limitations

1. **Password ambiguity (RAR3/4)**: Exit code 2 (wrong password) is only guaranteed for RAR5-encrypted archives. Legacy RAR3/4 archives with wrong passwords cannot be reliably distinguished from corruption and may report exit 3 instead. This is a limitation of the underlying `rardecode` library.

2. **Flat extraction caveats**: `-e/--flat` discards all directory structure. Empty directories are not created. Name collisions within the archive are resolved by the overwrite policy.

3. **Windows support**: Experimental. Full test suite is Unix-specific (symlinks, external fallback tools). Contributions welcome.

4. **Filename encoding**: The pure-Go decoder has no encoding-override flag. Filenames stored in non-Unicode codepages (Shift-JIS, GBK, etc.) may appear garbled. No post-extraction re-decoding is supported.
