# Project Roadmap

## Current Status

**Latest Release**: 0.4.0 (2026-09-07) — security hardening + CLI concurrency unification  
**Maturity**: Stable and feature-complete for direct RAR extraction  
**Active Development**: Paused; driven by user feedback and platform support requests

## Pivot Summary (v0.2.1 → v0.3.0)

The project pivoted from `rar2zip` (RAR-to-ZIP converter) to `macrarcli` (direct RAR extractor):

**What changed**:
- Binary and module renamed for clarity: `github.com/ongtungduong/macrarcli`
- No ZIP output; extracts RAR archives directly
- Four modes: extract (preserve structure), flat (-e), list (-l), test (-t)
- Overwrite policies instead of compression flags

**Preserved**:
- Core security invariants (Zip-Slip, bomb caps, atomic writes)
- Batch processing and concurrent extraction
- Pure-Go, dependency-minimal architecture
- Test suite and CI/CD infrastructure

**Rationale**: Direct extraction simplifies the tool, removes format-conversion complexity, and aligns with user workflows (most users want to extract, not convert).

## Completed Phases

All core extraction features are implemented:

### Phase 1: Extraction Engine ✅
- Native RAR decoding via `rardecode/v2`
- Archive listing with size metadata
- Integrity validation (checksum-only)
- Flat and structured extraction modes

### Phase 2: Security Hardening ✅
- Zip-Slip defense (path sanitization)
- Symlink/device neutralization
- Decompression-bomb caps (`--max-size`, `--max-entries`)
- Post-sanitize collision guard (dedup + error on cross-collisions)
- Atomic staging-directory extraction pattern

### Phase 3: Usability & Features ✅
- Password support (TTY prompt or `--password` flag)
- Overwrite policies (fail, overwrite, skip, rename)
- Batch processing with concurrency (`--jobs`)
- JSON output for automation
- Clear error messages and exit codes
- Progress reporting for single archives

### Phase 4: Distribution & Release ✅
- Keyless code signing (Sigstore/cosign)
- SHA256 checksum verification in install script
- Homebrew tap integration
- Scoop bucket (Windows experimental)
- One-line install script with cosign verification

### Phase 5: Documentation ✅
- Comprehensive README with examples
- System architecture and data flow
- Code standards and conventions
- TROUBLESHOOTING guide
- Security threat model documentation

### Phase 6: Security Hardening & Concurrency Unification ✅
- Decompression-bomb caps default to bounded values (`20G`/`200000`) instead of unlimited
- Extracted file permission bits capped to prevent group/other-writable output
- `$MACRARCLI_PASSWORD` env var fallback for scripted use (avoids argv exposure)
- Cross-filesystem rename fallback hardened against symlink TOCTOU
- List/test modes honor `--jobs` for concurrent batch processing

## Unreleased Work

**Currently**: None

The `docs/project-changelog.md` "Unreleased" section is empty. Backlog is driven by:
- User bug reports or feature requests
- Platform-specific support (Windows hardening, ARM testing)
- Dependency updates

## Explicit Out of Scope (YAGNI Decisions)

### Functionality

| Feature | Rationale |
|---------|-----------|
| ZIP output | Pivot decision: direct extraction is simpler and more user-friendly |
| Stdin/stdout streaming | Adds complexity; one-file-per-invocation covers 99% of use cases |
| RAR5-only archives | `rardecode` focuses on RAR3/4; RAR5 users can use external tools |
| Per-file include/exclude | Rare use case; shell filtering (`macrarcli -l *.rar \| grep pattern`) covers it |
| Path restructuring | Archive structures usually intentional; users post-process if needed |
| Filename encoding override | Pure-Go decoder discards raw bytes; lossy re-decoding not supported |
| Comment metadata preservation | Rarely used; adds complexity |
| Parallel compression within archive | Batch concurrency (`--jobs`) covers throughput needs |

### Distribution

| Package Format | Rationale |
|---|---|
| Docker image | CLI is small enough to install directly |
| deb/rpm/winget/asdf/mise | Homebrew + install.sh + Scoop covers 95% of use cases |

### Architecture

| Design | Rationale |
|---|---|
| Public library | Tool is not designed as a library; `internal/rarutil` is private |

## Platform Support Matrix

| Platform | CPU | Build | Test | Status |
|---|---|---|---|---|
| macOS | amd64 | ✅ | ✅ | Fully supported |
| macOS | arm64 | ✅ | ✅ | Fully supported |
| Linux | amd64 | ✅ | ✅ | Fully supported |
| Linux | arm64 | ✅ | ✅ | Fully supported |
| Windows | amd64 | ✅ | ⚠️ | Experimental (build + vet only) |
| Windows | arm64 | ✅ | ⚠️ | Experimental (build + vet only) |

**Windows gap**: Test suite is Unix-specific (symlinks, TTY password prompts). Full support requires Windows-native test infrastructure.

## Success Metrics

### Reliability
- ✅ CI pass rate: 100% (all platforms, all PRs)
- ✅ Test coverage: 80%+ (fixture-gated tests counted separately)
- ✅ No known security bugs
- ✅ Atomic output: 100% of extractions either fully succeed or fully fail

### Performance
- ✅ Single 10 GiB archive: <10 seconds on modern hardware
- ✅ Batch concurrency: 4 concurrent archives by default
- ✅ Memory bound: ~64KB per concurrent job (via `cappedWriter`)

### Adoption
- ✅ Homebrew: published and installable
- ✅ Scoop: published (experimental Windows support)
- ✅ GitHub releases: keyless-signed and checksummed

### Documentation
- ✅ README: comprehensive with examples and security caveats
- ✅ CONTRIBUTING.md: developer onboarding
- ✅ TROUBLESHOOTING.md: user Q&A
- ✅ System architecture: components, data flow, security invariants
- ✅ Code standards: explicit conventions for maintainers

## Dependency Management

### Pinned Versions

| Dependency | Version | Reason |
|---|---|---|
| `nwaples/rardecode` | v2.x | Pure-Go RAR decoder; actively maintained |
| `golang.org/x/term` | v0.45.0 | Masked TTY password prompts |
| Go toolchain | 1.26.2 | Stable; backported security patches |
| cosign | v2.6.3 | v3.x dropped legacy detached sign-blob flow |
| goreleaser | v2.16.0 | Pinned to avoid surprise breaking changes |

### Update Process

1. **Minor dependency update** (e.g., `rardecode` v2.1 → v2.2): Bump `go.mod`, test, no release needed
2. **Major version update** (e.g., Go 1.26 → 1.27): Coordinate with CI, test all platforms, release if breaking changes
3. **Security patch**: Test, release immediately with `fix:` prefix

## Known Limitations & Technical Debt

| Item | Severity | Status |
|---|---|---|
| Windows test suite missing | Low | Experimental status acceptable; contributors welcome |
| Password ambiguity (RAR3/4) | Medium | Legacy archives with wrong password report exit 3 (indistinguishable from corruption in `rardecode`) |
| RAR5-only archives | Low | No demand; `rardecode` focus is RAR4 |

## High-Impact Contribution Areas

1. **Windows test coverage** — Enable full CI testing on Windows (mock TTY, symlinks)
2. **Performance optimizations** — Benchmark-driven improvements for very large archives (50+ GiB)
3. **Bug reports on exotic archives** — Always include `--verbose` output
4. **Documentation translations** — README and TROUBLESHOOTING for other languages

## Future Vision (>2026)

Possible directions if demand materializes:

- **Windows maturity**: Full test coverage, drop "experimental" tag
- **Batch progress UI**: Real-time progress bars for large batches (TUI with library like `bubbletea`)
- **Archive introspection**: Detailed metadata without extraction (compression ratio, age distribution, etc.)
- **Cloud storage**: Native S3/GCS support for sources/destinations (probably out of scope — shell integration handles most)

However, **YAGNI applies**: no work starts until there is concrete user demand and clear requirements.

## How to Contribute

See [`CONTRIBUTING.md`](../CONTRIBUTING.md) for development setup, testing, and security review.

**Process**:
1. Fork the repo
2. Create a feature branch
3. Make changes (follow code standards)
4. Write/update tests
5. Run `make fmt vet test` — all must pass
6. Submit PR with clear description

**Security-sensitive code**: Include a security section in PR description explaining any changes to `sanitize.go`, `writer.go`, or `stage.go`.

## Issue Tracking

**File issues at**: https://github.com/ongtungduong/macrarcli/issues

**When reporting bugs**:
- Include `macrarcli --version` output
- Include `macrarcli --verbose` output (extra diagnostics)
- For security issues: email maintainer privately, don't file public issue
