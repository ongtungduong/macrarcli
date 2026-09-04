# Project Overview & Product Development Requirements

## Vision

rar2zip is a small, single-purpose CLI tool for converting RAR archives to ZIP format. It prioritizes simplicity, speed, and independence from external tools by using a pure-Go RAR decoder.

## Purpose

Provide users with a lightweight, dependency-free way to convert RAR archives to ZIP on macOS and Linux, with optional fallback to system tools (`unrar`/`7z`) for exotic archive variants.

## Target Users

- macOS/Linux users with RAR archives who prefer ZIP (smaller, more universal)
- Developers and system administrators managing archive format conversions
- Users needing batch conversion with compression control and integrity verification
- Advanced users requiring decompression-bomb protection and Zip-Slip defense

## Problem Statement

RAR archives are common in file sharing but less universally supported than ZIP. Existing converters either:
- Require external tools (`unrar`/`7z`) as runtime dependencies
- Are GUI-only or closed-source
- Lack security hardening for untrusted archives

rar2zip solves this with a self-contained pure-Go CLI and explicit security design.

## Scope & Goals

### In Scope

**Functional**
- Convert single or batch RAR archives to ZIP
- Support password-protected archives
- Preserve file/directory structure, timestamps, and Unix permissions
- Support multi-volume RAR sets (`.part1.rar`, `.r00`, etc.)
- Controllable compression (store, Deflate levels 1-9)
- Post-conversion integrity verification (`--verify`)
- Read-only archive preview (`--list`)
- Decompression-bomb defense (`--max-size`, `--max-entries`)
- Human and JSON output modes
- Shell completion scripts

**Non-Functional**
- No external runtime dependencies on the native path
- Atomic output (temp + rename, never partial/truncated)
- Bounded memory usage (pooled buffers for concurrent batch work)
- Hardened against Zip-Slip, path traversal, symlink/device attacks
- Keyless code signing (Sigstore/cosign) for release authenticity
- Cross-platform availability (Homebrew, install script, Scoop)

### Out of Scope (Explicit YAGNI)

- Stdin/stdout streaming
- RAR5 format (only RAR4)
- Per-file include/exclude globs
- Filename encoding overrides
- Path flattening or directory restructuring
- Comment/metadata preservation beyond POSIX basics
- Parallel compression within a single archive
- Public library promotion of `internal/convert`
- Docker image, winget, asdf, mise, deb, rpm packaging

## Success Criteria

1. **Correctness**: All conversions preserve file content, names, timestamps, and permissions identically
2. **Security**: Resists Zip-Slip, decompression bombs, symlink/device attacks on untrusted archives
3. **Performance**: Handles multi-GiB archives efficiently with bounded memory
4. **Usability**: Single-command CLI with clear error messages and deterministic batch output
5. **Reliability**: Atomic writes; failed conversions never touch output; all tests pass on CI
6. **Distribution**: Available via Homebrew, install script, and Scoop (Windows experimental)

## Key Features

| Feature | Status | Notes |
|---------|--------|-------|
| Pure-Go RAR decode | ✅ | Native path, via `nwaples/rardecode/v2` |
| ZIP output with compression control | ✅ | `--store` / `--level 1-9` |
| Batch processing with concurrency | ✅ | Default `min(NumCPU, 4)` jobs |
| Password support | ✅ | Native and fallback paths |
| Multi-volume handling | ✅ | Follows `.part1.rar` / `.r00` chains |
| Post-conversion verification | ✅ | `--verify` checks CRC32 per entry |
| Decompression-bomb caps | ✅ | `--max-size` / `--max-entries` (native only) |
| Archive preview without conversion | ✅ | `--list` with `--json` support |
| System tool fallback | ✅ | `--allow-fallback` for exotic RAR variants (not bomb-bounded) |
| Release signing | ✅ | Keyless cosign for checksums.txt |
| Windows support | ⚠️ | Experimental; build + vet only in CI |

## Version & Timeline

**Current Version**: 0.2.1 (2026-06-18)  
**Last Major Phase**: Hardening Upgrade (5 phases completed)  
**Maturity**: Stable for production use on native path; experimental on Windows

## Roadmap Status

All planned phases complete per `plans/260614-2315-rar2zip-hardening-upgrade-roadmap/`:
1. ✅ Correctness & Safety Hardening
2. ✅ Real-World Robustness
3. ✅ Performance & Benchmarks
4. ✅ Distribution & Supply-Chain Hardening
5. ✅ UX & Docs Polish

No backlog items; future work is driven by user feedback and platform support requests.
