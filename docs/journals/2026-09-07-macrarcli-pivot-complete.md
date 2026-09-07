# rar2zip → macrarcli Pivot: 6-Phase Rebranding & Engine Rewrite Closed

**Date**: 2026-09-07 14:30
**Severity**: Low (all work resolved; no blockers remain)
**Component**: Module rename, extraction engine rewrite, CLI surface rewrite, distribution & docs
**Status**: Resolved — all 6 phases complete, committed to `macrarcli-pivot` branch

## What Happened

Completed a full pivot from `rar2zip` (RAR-to-ZIP converter) to `macrarcli` (direct RAR extraction CLI with no ZIP output). Six phases: module/import rename (Phase 1), extraction engine rewrite around staging+atomic commit (Phase 2), feature completion with `Test()` + password handling + progress tracking (Phase 3), overwrite policies + fallback removal (Phase 4), CLI surface rewrite with new mode flags and exit codes (Phase 5), and distribution + documentation rewrite (Phase 6). All work test-first via `tester` subagent, code-reviewed via `code-reviewer` subagent, all phases sync'd to plan tracker. `go build ./...`, `go test ./...`, `gofmt`, `go vet` all pass at repo root.

## The Brutal Truth

This was procedurally clean but exposed two "plan said X, but X was never built" gaps discovered only during implementation, not during original plan authoring:
1. **Phase 5**: `internal/rarutil.Options` struct lacked the `Flat` field that the CLI architecture assumed existed. Added `Options.Flat + flattenName()` in-phase to close the gap.
2. **Phase 5**: The `-l/--list` dispatch branch was passing the unresolved `--password` flag value instead of the single resolved password, discarding an interactively-prompted password before reaching `List()`. Caught by code review, fixed same session.
3. **Phase 6**: Docs-manager subagent's generated documentation contained three factual inaccuracies (stale directory-naming convention, obsolete command example, references to Phase-4-deleted files) that only surfaced from manual line-by-line fact-check, not from the `validate-docs.cjs` validator (which doesn't scan `internal/rarutil/`). All three fixed.

The relief: none of these gaps were architectural; all were pure implementation details fixed in the same phase they surfaced. Process worked: subagent test+review caught a real password bug; manual doc fact-check caught LLM hallucinations.

## Technical Details

### Phases 1–2: Module Rename & Engine Rewrite

- **Phase 1**: `github.com/ongtungduong/rar2zip` → `github.com/ongtungduong/macrarcli`. Zero behavior change; import paths updated throughout
- **Phase 2**: Rewrote extraction engine from stream-to-ZIP (`internal/convert/`) to staging-directory-then-atomic-commit design (`internal/rarutil/`). Each archive: decode entries into private temp dir, commit into destination via per-entry `os.Rename` only after whole archive succeeds, roll back entirely on failure. Replaced ZIP output with real files. This phase broke `go build ./...` by design (intermediate accepted state); documented in plan

### Phase 3: Feature Completion

- Added `EntryInfo.PackedSize`, `EntryInfo.Encrypted`
- Implemented `Test()`: checksum-only validation (no writes), reuses decompression-bomb-cap from extraction
- Implemented `password.go`: `ResolvePassword()` function (masked TTY prompt via `golang.org/x/term`, only runtime dependency), designed to be called exactly once per invocation
- Implemented `progress.go`: `ProgressTracker` for live progress arithmetic
- Code review caught `Test()` not wiring `OnEntry` callback; fixed same session

### Phase 4: Overwrite Policies & Hardening

- Implemented four overwrite policies: fail (default), overwrite, skip, rename
- Removed `unrar`/`7z` shell-out fallback entirely (closing documented "not bomb-bounded" security weakness)

### Phase 5: CLI Surface Rewrite (This Triggered `go build ./...` Success)

- New mode flags: `-e/--flat`, `-l/--list`, `-t/--test` (mutually exclusive)
- New overwrite-policy flags; consolidated `-o/--dest`
- New 5-code exit scheme (0/1/2/3/4, with 2>3>4 batch precedence) replacing old 0/1/2
- **Implementation gap found & fixed**: `Options.Flat` field never added in Phase 2/4 despite Phase 5's architecture assuming it. Added `Options.Flat + flattenName()` to close
- **Password bug found & fixed by code review**: `-l/--list` branch was passing unresolved `--password` flag string instead of single resolved password, silently discarding interactive prompts. Fixed and reverified

### Phase 6: Distribution & Docs (Final Phase)

- Updated `.goreleaser.yaml`: added explicit `project_name: macrarcli` override (GoReleaser would infer old name from unrenamed git remote)
- Updated `scripts/install.sh`, regenerated shell completions
- Full rewrite of `README.md`, `CONTRIBUTING.md`, all 7 `docs/*.md` files via `docs-manager` subagent
- Manually fixed 3 docs inaccuracies post-generation: stale staging-dir-name claim, obsolete usage example, code-standards references to deleted files
- Discovered: CLI's own `"rar2zip:"` error-prefix/usage-banner/`--version` strings were never assigned to any phase's file list, yet Phase 6's success criterion (`grep -ri rar2zip` returns nothing except historical refs) required them gone. Fixed as pure string-literal rename, verified via full build/vet/test/gofmt + manual CLI smoke test

## What We Tried

1. **Staging vs. direct write**: Phase 2 initially explored direct streaming to destination. Abandoned for staging model (atomic per-entry commit on success, complete rollback on failure)
2. **Password handling scope**: Phase 3 experimented with per-job password resolution. Settled on single-invocation design (`ResolvePassword` called once, result passed through)
3. **Exit codes during batch**: Phase 5 first used simple 0/1/2. Realized batch operations need precedence (file-not-found < some-failed < all-failed). New 5-code scheme with 2>3>4 precedence
4. **Docs generation**: Phase 6 initially used `docs-manager` as authoritative. Discovered LLM hallucinates subtle details (directory names, deleted file references). Added mandatory manual fact-check pass

## Root Cause Analysis

### Why Phase 5 gaps existed

**Options.Flat field**: Phase 2 rewrote the engine but didn't enumerate all `Options` struct fields that Phase 5's new `-e/--flat` flag would need. Plan authoring step missed this dependency link. Caught during Phase 5 implementation (not fatal, fixed same phase).

**Password bug in list mode**: CLI dispatch code was written against the assumption that `--password` raw value would be resolved before reaching `List()`. The resolution function (`ResolvePassword`) was new in Phase 3, but Phase 5's dispatch code didn't wire it for the `-l` branch. Code review traced the data flow and caught it.

### Why docs had LLM inaccuracies

`validate-docs.cjs` validator has a known blind spot: it doesn't traverse `internal/` package code. LLM-generated docs cited conventions (directory naming, file paths, deleted modules) that the validator couldn't cross-check. Caught only by manual read-through.

## Lessons Learned

1. **Plan gaps surface during implementation, not authoring**: Two structural gaps (Options field, password resolution in list mode) were invisible until the phase actually wired them. This is normal; caught early via subagent test+review. Document it (not a failure mode)

2. **Subagent test+review pipeline is scalable**: Every phase this session: implement → spawn `tester` → spawn `code-reviewer` → fix findings → re-verify. Caught two real bugs before commit (Test() callback wiring in Phase 3; password bug in Phase 5). Pattern is worth repeating

3. **Generated docs need independent fact-check**: LLM docs generation is fast and functional but introduces subtle hallucinations (stale conventions, obsolete examples, references to deleted files). Automated validators have blind spots. Manual read-through remains necessary

4. **CLI strings are easy to miss during rename**: The `"rar2zip:"` prefix in error messages, usage banners, and `--version` output isn't in a single enum or constant. Discovered it wasn't assigned to any phase's file list until Phase 6's success criterion forced a search. Consider centralizing CLI brand strings in future rewrites

## Next Steps

### Manual follow-ups (outside this codebase)

1. **GitHub repository rename or remote URL update**: The git remote still points to `ongtungduong/rar2zip`. Go module, binary, README, and `.goreleaser.yaml` all say `macrarcli` now. Need manual GitHub repo rename (or `git remote set-url`) before links resolve
2. **Homebrew & Scoop manifests**: Separate repos (`ongtungduong/homebrew-tap`, `ongtungduong/scoop-bucket`) need `macrarcli` formula/manifest created. This codebase's `.goreleaser.yaml` only points at expected names; can't create them
3. **Local release snapshot test**: `make release-snapshot` never ran (goreleaser/cosign not installed locally). Config reviewed by eye for consistency but not executed. Run before cutting v1.0.0 tag

### Integration

- Merge `macrarcli-pivot` to `main`
- Coordinate with GitHub repo rename
- Tag v1.0.0 to trigger release automation

---

**Commit(s)**: All 6 phases committed to `macrarcli-pivot` branch per plan tracker. Ready for merge review.

**Owner**: rar2zip → macrarcli pivot  
**Status**: Code-complete. External steps (GitHub rename, Homebrew/Scoop manifests, local release snapshot test) pending
