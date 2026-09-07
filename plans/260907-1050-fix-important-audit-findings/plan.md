# Fix Important findings from parallel architecture/performance/security audit

Source: `plans/reports/parallel-codebase-audit-260907-1011-architecture-performance-security-macrarcli-report.md`
Scope: all 8 **Important** findings only (Minor/doc-drift out of scope per user request).

## Decisions locked with user
- Decompression-bomb defaults: `--max-size` 20G, `--max-entries` 200000 (was unlimited). Behavior change — call out in README/CHANGELOG.
- Password env fallback: `MACRARCLI_PASSWORD`, precedence `--password` flag > env > TTY prompt.
- `commitMu` global lock (Performance #2): **no code change**. Verified the CLI always assigns the same `--dest` to every job in a batch today, so a per-destDir lock changes nothing for current usage; the code's own doc comment already documents the global lock as deliberate. Reversing a verified prior decision without new data is against project audit rules. Marked reviewed-not-applicable in the fix report.

## Phases
1. `phase-01-security-hardening.md` — perm-bit cap, password env var, TOCTOU O_NOFOLLOW guard, decompression-bomb non-zero defaults.
2. `phase-02-cli-dispatch-unification.md` — generic batch-concurrency helper so `-l`/`-t` honor `--jobs` (Perf #1); unify `runList` onto `rarutil.Options` closing the `--verbose` plumbing gap (Arch #1); dedupe the `report`/`runTest` human-output loop via a shared reporter (Arch #2, partial — list's table output stays separate by design, see phase file).

## Out of scope (Minor, not requested)
`onStart` dead code, `writer.go` per-file `MkdirAll`, fd leak on encryption probe, doc drift in `codebase-summary.md`/`system-architecture.md`, `list_output.go`/`writer.go` file-size split.

## Status
- [x] Phase 01 (perm-bit mask, password env fallback, O_NOFOLLOW, non-zero bomb-cap defaults)
- [x] Phase 02 (runParallel/TestBatch/ListBatch, runList on Options, reportHuman unification)
- [x] Test (tester subagent: build/vet/fmt/race clean on darwin+linux+windows cross-compile)
- [x] Code review (code-reviewer subagent found 1 High regression — `-q` accidentally suppressing extract's stdout success line in the new `reportHuman`; fixed via `quietGatesSuccess` param, locked with `TestReport_QuietDoesNotSuppressSuccessLine` + `TestReportHuman_QuietGatesTestSuccessLine`)
- [x] Finalize (docs sync via docs-manager: system-architecture.md/codebase-summary.md/code-standards.md updated for runParallel/TestBatch/ListBatch naming, including a pre-existing onProgress->onStart doc-drift fix picked up as a byproduct; journal + commit pending user go-ahead)
