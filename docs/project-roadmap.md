# Project Roadmap

## Current Status

**Latest Release**: 0.2.1 (2026-06-18)  
**Maturity**: Stable and feature-complete for primary use case (RAR→ZIP conversion with security hardening)  
**Active Development**: Paused; driven by user feedback and platform support requests

## Completed Phases (Hardening Upgrade Roadmap)

All 5 phases of the hardening upgrade roadmap have been completed. See [`plans/260614-2315-rar2zip-hardening-upgrade-roadmap/`](../plans/260614-2315-rar2zip-hardening-upgrade-roadmap/) for full details.

### Phase 1: Correctness & Safety Hardening ✅

**Focus**: Zip-Slip defense, symlink/device neutralization, decompression-bomb caps

**Delivered**:
- Entry-name sanitization (reject traversal, absolute, empty names)
- Symlink/device bit stripping (via `safeMode()`)
- Per-entry size + total size caps (`--max-size`, `--max-entries`)
- Post-sanitize name-collision guard (dedup repeats, error on cross-collisions)
- Comprehensive security review and threat model documentation

**Security Invariants Locked In**: All code paths (native & fallback) enforce identical hardening.

### Phase 2: Real-World Robustness ✅

**Focus**: Handle multi-volume sets, exotic archive variants, edge cases

**Delivered**:
- Multi-volume RAR support (`.part1.rar`, `.r00` chains)
- Password-protected archives (`--password` flag)
- `--allow-fallback` path for exotic/corrupted RAR variants (shells out to `unrar`/`7z`)
- Free-space pre-check for fallback extraction (early-exit on tight disks)
- Fixture-agnostic test coverage via interface seams

**Known Gap**: `--allow-fallback` is not decompression-bomb-bounded (documented in README).

### Phase 3: Performance & Benchmarks ✅

**Focus**: Optimize throughput and memory usage

**Delivered**:
- Pooled 512 KB copy buffer for large-entry streaming (~7% throughput gain, 60-85% fewer allocations)
- Concurrent batch processing by default (`--jobs` defaults to `min(NumCPU, 4)`)
- Deterministic ordered output (results printed in input order despite concurrent execution)
- ZIP64 (>4 GiB) support and round-trip verification
- Benchmarks for hot paths (streaming, `--verify`, native convert)

**Performance Characteristics**:
- Single 10 GB archive: ~few seconds on modern hardware
- Batch of 100 archives: concurrent processing via worker pool
- Memory: bounded to ~512 KB per concurrent job

### Phase 4: Distribution & Supply-Chain Hardening ✅

**Focus**: Secure releases, packaging, code signing

**Delivered**:
- Keyless code signing via Sigstore/cosign (Actions OIDC, no stored keys)
- SHA256 checksum verification in `install.sh`
- Cosign signature verification in `install.sh` (optional, if cosign installed)
- Goreleaser automation (multi-platform: linux/darwin/windows, amd64/arm64)
- Homebrew tap integration (`ongtungduong/homebrew-tap`)
- Scoop bucket integration (`ongtungduong/scoop-bucket`, Windows experimental)
- Shell completion scripts (bash, zsh, fish)

**Release Checklist**: Version tag → GitHub Actions → build → sign → publish to Homebrew/Scoop.

### Phase 5: UX & Docs Polish ✅

**Focus**: Documentation, help text, user-facing error messages

**Delivered**:
- Comprehensive README with examples, flags table, security section, troubleshooting
- CONTRIBUTING.md for developers
- TROUBLESHOOTING.md for users
- Shell completion scripts
- `--help` and `--version` output
- Clear error messages (e.g., "output exists (use -f to overwrite)")
- Project documentation suite (this roadmap, codebase summary, system architecture)

## Unreleased Work

Currently: **None**

The `docs/project-changelog.md` "Unreleased" section is empty. Backlog is driven by:
- User bug reports or feature requests (filed via GitHub issues)
- Platform-specific support requests (e.g., Windows hardening, ARM testing)
- Dependency updates (e.g., `rardecode` or Go version bumps)

## Explicit Out of Scope (YAGNI Decisions)

These features were evaluated and intentionally excluded to keep the tool focused and maintainable:

### Functionality

| Feature | Rationale | Impact |
|---------|-----------|--------|
| stdin/stdout streaming | Adds complexity; one-file-per-invocation is simpler and covers 99% of use cases | Users pipe results via shell redirection (`<` / `>`) instead |
| RAR5 format support | RAR5 is newer; `rardecode` supports RAR4 well; would need dependency upgrade | RAR5 users need to extract with external tool first |
| Per-file include/exclude globs | Adds arg parsing complexity; `--list` + shell `grep` covers use case | Users filter via shell (`rar2zip *.rar \| grep pattern`) |
| Path flattening (`--flatten`) | Contradicts "preserve structure" goal; archive structures usually intentional | Users can restructure via shell or post-processing |
| Filename encoding override (`--encoding`) | Pure-Go decoder discards raw bytes; re-decoding lossy + path-traversal risk | Users run `unrar -cp936 …` then re-zip for CJK archives |
| Comment/metadata preservation | RAR comments rare in practice; ZIP comment insertion adds complexity | Comments are dropped (noted in changelog) |
| Parallel compression within archive | Requires splitting archive pre-write; simpler to use `--jobs` across archives | Batch parallelism covers most throughput needs |

### Distribution

| Package Format | Rationale | Workaround |
|---|---|---|
| Docker image | Adds build/publish complexity; CLI is small enough to install directly | Users build locally: `docker run --rm -v $(pwd):/work golang make build` |
| `winget` / `asdf` / `mise` / `deb` / `rpm` | Fragmentation; Homebrew + install.sh + Scoop covers 95% of use cases | File an issue if your package manager is critical |

### Architecture

| Design | Rationale | Impact |
|---|---|---|
| Public library (`rar2zip` package) | Tool is not a library; `internal/convert` is private by design | Embedders must shell out to the CLI or fork the code |

## Dependency Updates

### Pinned Versions

| Dependency | Version | Reason |
|---|---|---|
| `nwaples/rardecode` | v2.x | Pure-Go RAR decode; actively maintained |
| Go toolchain | 1.26.2 (see `go.mod`) | Stable; backported security patches |
| cosign | v2.6.3 (release.yml) | v3.x dropped legacy detached sign-blob flow |
| goreleaser | v2.16.0 (release.yml) | Stable; pinned to avoid surprise breaking changes |

### Update Process

1. **Minor dependency update** (e.g., `rardecode` v2.1 → v2.2): Bump in `go.mod`, test, no release needed
2. **Major version update** (e.g., Go 1.26 → 1.27): Coordinate with CI matrix, test Windows/ARM, release if breaking changes
3. **Security patch**: Test, release immediately with `fix:` prefix in commit message

## Platform Support Matrix

| Platform | CPU | Build | Test | Status |
|---|---|---|---|---|
| macOS | amd64 | ✅ | ✅ | Fully supported |
| macOS | arm64 | ✅ | ✅ | Fully supported |
| Linux | amd64 | ✅ | ✅ | Fully supported |
| Linux | arm64 | ✅ | ✅ | Fully supported |
| Windows | amd64 | ✅ | ⚠️ | Experimental (vet only) |
| Windows | arm64 | ✅ | ⚠️ | Experimental (vet only) |

**Windows Gap**: Test suite is Unix-only (fallback tools and symlinks are Unix-specific). Contributions welcome to add Windows test support (e.g., mock fallback tools).

## Success Metrics

### Reliability
- ✅ CI pass rate: 100% (all platforms, all PRs)
- ✅ Test coverage: 80%+ (fixture-gated tests excluded)
- ✅ No known security bugs (0 reported)
- ✅ Atomic output: 100% of conversions either fully succeed or fully fail

### Performance
- ✅ Single 10 GiB archive: <10 seconds on modern hardware
- ✅ Batch concurrency: 4 concurrent archives by default
- ✅ Memory bound: ~512 KB per concurrent job (pooled buffer)

### Adoption
- ✅ Homebrew: published and installable
- ✅ Scoop: experimental Windows support
- ✅ GitHub releases: signed and checksummed

### Documentation
- ✅ README: comprehensive with examples and security caveats
- ✅ CONTRIBUTING.md: developer onboarding + security review checklist
- ✅ TROUBLESHOOTING.md: user-facing Q&A
- ✅ System architecture: component diagram + data flow + security invariants
- ✅ Code standards: explicit conventions for future maintainers

## Feedback & Bug Reports

**Where to File Issues**: https://github.com/ongtungduong/rar2zip/issues

**Common requests monitored**:
- Windows support hardening
- Platform-specific packaging (deb, rpm, winget, asdf)
- Performance profiles on very large archives (50+ GiB)
- Exotic RAR variants (rare, usually fallback-handled)

## Technical Debt & Known Gaps

| Item | Severity | Status |
|---|---|---|
| `--allow-fallback` not bomb-bounded | Medium | Documented; users must trust archives. Pre-extraction bound planned for future phase. |
| Windows test suite missing | Low | Experimental status acceptable; contributors welcome. |
| RAR5 support | Low | No demand; `rardecode` focus is RAR4. File issue if needed. |
| Per-file globs | Low | Rare use case; shell filtering covers it. |

## How to Contribute

See [`CONTRIBUTING.md`](../CONTRIBUTING.md) for development setup, test conventions, and security review checklist.

**High-Impact Contributions**:
1. Windows test suite (e.g., mock tools, symlink emulation)
2. Performance optimizations with benchmarks
3. Bug reports on exotic archives (always include `--verbose` output)
4. Translations of `README.md` and `TROUBLESHOOTING.md`

## Future Vision (>2026)

Possible directions if demand materializes:

- **Windows maturity**: Full test coverage, official support drop of "experimental" tag
- **RAR5 support**: If `rardecode` adds it or significant user demand
- **Batch progress UI**: Real-time progress bars for large batches (TUI)
- **Cloud storage**: Native S3/GCS support for sources/destinations

However, **YAGNI applies**: no work starts until there is concrete user demand and clear requirements.
