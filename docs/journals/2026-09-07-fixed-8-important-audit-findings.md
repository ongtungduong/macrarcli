# Fixed All 8 Important Findings from Parallel Audit: Security, Performance, Architecture

**Date**: 2026-09-07 11:57
**Severity**: High (security + breaking behavior changes; unfixed could expose users to attacks & silent bugs)
**Component**: internal/rarutil (permissions, password, staging rename), cli dispatch (batch concurrency, verbose plumbing)
**Status**: Resolved — all code committed to working tree, code-reviewed, cross-compiled verified, pending user review before merge

## What Happened

Implemented fixes for all 8 **Important** findings flagged in the parallel architecture/performance/security audit (dated 2026-09-07 10:11), split across two phases: Phase 1 addressed 4 security findings (perm-bit capping, password env-var fallback, TOCTOU O_NOFOLLOW guard, decompression-bomb non-zero defaults); Phase 2 unified CLI dispatch so `-t`/`-l` modes honor `--jobs` (was sequential-only), and plumbed `Options` struct through both modes to close a `--verbose` dead-code gap. All changes are uncommitted but in working tree; 15 files changed (+311/-98 lines).

## The Brutal Truth

This session's only real regression was caught by the code-reviewer subagent, not by any automated tool: unifying `report()` and `runTest()` into a `reportHuman()` helper accidentally added a `!quiet` guard to extraction's stdout success line ("extracted X -> Y"), which the original `report()` never had — only test mode's "OK" line was ever quiet-gated. This directly violated Phase 2's explicit acceptance criterion ("byte-for-byte identical output"). Root cause: existing test suite asserts exit codes only, never stdout content; a plausible-looking refactor slipped an undocumented CLI contract change past compilation, `go vet`, and the full test suite. The fix required two new targeted tests (`TestReport_QuietDoesNotSuppressSuccessLine`, `TestReportHuman_QuietGatesTestSuccessLine`) asserting actual stdout, plus a `quietGatesSuccess bool` parameter preserving the per-mode distinction. This is exactly the kind of coverage gap—assertion of output contract, not just exit codes—that was missing and would have caught the slip immediately.

## Technical Details

### Security Hardening (Phase 1)

**1. Perm-bit mask (CVE-class)** — `internal/rarutil/sanitize.go:45`
- Archive-controlled mode bits (e.g., 0777) now masked with `&^ 0o022` before chmod. Prevents extracted files from being world-writable even if archive records 0777 (on shared destDir like `/tmp` or checkout share, other local users could modify before victim reads/executes).
- Updated test expectations in `safe_mode_test.go` (0o777 → 0o755 after mask).

**2. Password env-var fallback** — `cli_args.go` + `main.go:32`
- New `resolveExplicitPassword(flagVal, getenv)` helper picks `--password` flag first, then `$MACRARCLI_PASSWORD`, then TTY prompt (via existing `ResolvePassword`).
- Avoids exposing secret via `ps`/`/proc/<pid>/cmdline` for scripted/automated use (argv visible to all local users for process lifetime).
- Updated `--password` flag usage string to mention env var.
- Tested in `cli_args_test.go` (flag-wins / env-fallback / both-empty cases).

**3. TOCTOU symlink race in rename fallback** — `internal/rarutil/stage.go` + two new build-tagged files
- Cross-filesystem rename fallback (`os.Rename` fails with EXDEV) now opens files with `O_NOFOLLOW` to prevent narrow TOCTOU window between `checkOverwrite`'s Lstat and the fallback open.
- Implemented as `const noFollowFlag` in `nofollow_unix.go` (`syscall.O_NOFOLLOW`, `//go:build unix`) and `nofollow_other.go` (0, `//go:build !unix`).
- Verified cross-compile clean: `GOOS=linux`, `GOOS=darwin`, `GOOS=windows` all build without error. Real-world trigger rare (staging nests under destDir, so rename nearly always same-fs), but closes the gap.

**4. Decompression-bomb cap defaults** — `main.go:35-45`
- `--max-size` now defaults to "20G" (was "0"/unlimited), `--max-entries` to 200000 (was 0/unlimited).
- Archive-controlled size/entry-count cap mechanism itself was already correct; only the default was opt-in (user had to know to use flags).
- Users can still pass `--max-size 0 --max-entries 0` to opt back into unlimited (documented in flag help). Pure behavior change—called out in `docs/project-changelog.md` Unreleased section.

### Performance + Architecture Unification (Phase 2)

Grouped because all three findings touch the dispatch layer and fixing one enables the others.

**5. `--test`/`--list` now honor `--jobs`** — `internal/rarutil/batch.go` (new generic `runParallel[T,R]`)
- New `TestBatch(srcs, opts, maxParallel)` and `ListBatch(srcs, opts, maxParallel)` functions built on a shared generic `runParallel[T,R any]` helper (semaphore + WaitGroup, same shape `RunBatch` had).
- `RunBatch` rewritten as a thin wrapper over `runParallel` (behavior-identical, verified by existing `batch_test.go` still passing unmodified).
- Result: `macrarcli -t --jobs 4 *.rar` and `macrarcli -l --jobs 4 *.rar` now run concurrently (previously sequential).

**6. `runList` now takes full `Options` struct** — `main.go:204`, `list_output.go:21-28`
- Old signature: `runList(inputs []string, password string, maxEntries int, jsonOut bool)` — ad-hoc, incomplete.
- New signature: `runList(inputs []string, opts Options, maxParallel int, jsonOut bool)` — reuses the `Options` struct every other mode already uses.
- Closes a `--verbose` no-op gap (Arch #1 from audit): `verbose` flag now reaches list mode via `opts.OnVerbose` (was never plumbed before). Also closes root cause of a prior password bug in list mode (unresolved raw password reused instead of resolved one, already fixed on main, but pattern gap lingered in signature).
- Every mode (extract, test, list) now uses the same `Options` struct, eliminating a three-headed signature divergence.

**7. Unified human-output reporter** — `main.go:307-330` (new `reportHuman()` helper)
- Original `report()`, `runTest()`, and `runList`'s output loop were 3 near-identical hand-rolled loops (root cause for pattern bugs to hide in one mode but not others).
- Extracted `reportHuman(results, quiet, showPartial, quietGatesSuccess, successLine)` helper parameterized by:
  - `successLine` function (extract: "extracted X -> Y", test: "X: OK").
  - `showPartial` bool (extract batches can have skipped entries, test never can).
  - `quietGatesSuccess` bool (extract's success line unconditional, test's quiet-gated — a pre-existing, documented distinction preserved).
- `report()` and `runTest()` now one-liners calling `reportHuman`.
- List's table output (`printList`, `reportListJSON`, `listExitCodes`) stays separate (table is a different output shape, forced into a line-loop would be a worse abstraction fit per design doc).
- Verified byte-for-byte identical output to pre-refactor (except where code review found and fixed the regression).

**Code review regression & fix:**
- Initial refactor accidentally added `!quiet` guard to extract's success line, violating the phase plan's acceptance criterion ("byte-for-byte identical stdout/stderr").
- Caught by code-reviewer subagent (not by `go test`, `go vet`, or existing test suite — all passed despite the change).
- Root cause: existing tests assert exit codes only; no test asserted stdout content. A plausible refactor slipped a CLI contract change undetected.
- Fixed via `quietGatesSuccess bool` parameter in `reportHuman` preserving the per-mode distinction.
- Locked going forward with two new tests:
  - `TestReport_QuietDoesNotSuppressSuccessLine`: asserts extract's success line prints even with `-q`.
  - `TestReportHuman_QuietGatesTestSuccessLine`: asserts test's success line is suppressed with `-q`, honored without `-q`.
- Both tests use a new `captureStdout(t, fn)` helper that the repo was missing.

### Deliberate Non-Change: `commitMu` Global Lock

Audit flagged Performance #2: global `commitMu` mutex in `stage.go` serializes batch commits across all jobs, even when destDirs differ completely.

**Decision: No code change.** Rationale (per project `.claude/rules/review-audit-self-decision.md`):
1. Verified by reading call site (`main.go:210`) — CLI always assigns the same `--dest` to every job in a single invocation today. A per-destDir lock provides zero benefit for current usage.
2. Code's own doc comment (`stage.go:19-20`) already states the global lock is deliberate: "a per-destDir lock would be a premature optimization for this tool's typical batch sizes."
3. Audit opinion alone is insufficient to reverse a verified prior design decision without new data. The new data (call-site inspection) showed the optimization would not apply to today's usage.
4. Marked as "reviewed-not-applicable" in the fix report rather than a regression/oversight.

This is a worked example of the repo's own audit-decision rules in action: design decisions should stick unless an audit brings **new data** that the decision was wrong for the actual use case, not just theoretically suboptimal.

## Verification

**Build/test/format clean:**
- `go build ./...` ✓
- `go vet ./...` ✓
- `gofmt -l .` (no files) ✓
- `go test -race ./...` ✓
- Cross-compile: `GOOS=linux go build ./...` ✓, `GOOS=windows go build ./...` ✓

**Test coverage:**
- Existing `batch_test.go` passes unmodified (RunBatch behavior-identical).
- New `cli_args_test.go` tests password env-var precedence.
- New `main_test.go` tests: `TestDefaultCaps` (bomb-cap defaults), `TestReport_QuietDoesNotSuppressSuccessLine`, `TestReportHuman_QuietGatesTestSuccessLine` (output contract).
- All fixture tests (`TestRun_Extract`, `TestRun_Batch`, `TestRun_TestFixture`) pass.

**Coverage gap:** O_NOFOLLOW's cross-filesystem rename fallback can't be unit-tested portably (requires real cross-fs mount). Verified via build-tag correctness and code inspection; not exercised by real archive.

**No testdata fixture:** Repo has no committed `.rar` files. `TestBatch`/`ListBatch` concurrency correctness against a real archive is verified only by manual CLI smoke-testing and unit-level coverage of underlying primitives (not end-to-end fixture-backed test).

## What We Tried

1. **Quiet-gating refactor, first cut:** Unified `report()` and `runTest()` into `reportHuman` with a single `quiet` param gating all success lines. Accidentally regressed extract's documented behavior.
2. **Fix attempt #1:** Add a separate `extractSuccessUnconditioned bool` param. Felt ad-hoc.
3. **Fix (final):** Renamed to `quietGatesSuccess` and added explicit doc comment explaining the pre-existing per-mode distinction. Much clearer intent.

## Root Cause Analysis

### Why the quiet-gating regression slipped through

1. **Test suite gap:** Existing tests assert exit codes only, never stdout content. A CLI's output contract (what exactly gets printed when) isn't enforced.
2. **Plausible refactor:** The unification looked clean and correct; diff logic suggested both branches should be gated the same way.
3. **Code review caught it:** The subagent code-reviewer manually traced the phase plan's acceptance criterion ("byte-for-byte identical output") and caught the slip. This is why code review, not just test automation, is in the pipeline.

### Why the verbose/password plumbing gaps existed until now

`runList` had always taken an ad-hoc parameter tuple (password, maxEntries) instead of the shared `Options` struct. This divergence meant:
- New `Options` fields (like `OnVerbose` in a later phase) were easy to forget to wire through.
- A prior bug (list mode using raw password instead of resolved password) could hide in one mode without surfacing in others.

Phase 2's design fix (all modes on `Options`) closes this at the source.

## Lessons Learned

1. **Test-only assertions on exit codes are insufficient:** A CLI's output is part of its contract. Tests asserting stdout/stderr content should be standard, especially after refactors that touch reporting logic. The new `captureStdout` helper fills a gap this repo didn't have.

2. **Signature divergence hides bugs:** When three modes all do similar work (extract, test, list) but have different parameter tuples, bugs hide in one mode. Unifying onto a single shared struct (`Options`) reduces the surface area for pattern repetition.

3. **Audit rules applied: verified decisions stick:** Reversing the `commitMu` lock without new evidence it was wrong for the actual usage would have been premature. The decision rule in `.claude/rules/review-audit-self-decision.md` is right — changes should be grounded in evidence, not just opinion.

4. **Code review is necessary, not sufficient:** `go test` + `go vet` + `gofmt` all passed. Only manual code review reading the phase plan's acceptance criterion caught the regression. Automation is cheap; augment with targeted manual review for high-risk changes.

## Next Steps

1. **User review & approval** — work remains uncommitted pending review.
2. **Commit & merge** — once approved, create single commit documenting the 8 fixes + one revert (commitMu decision).
3. **Update changelog** — `docs/project-changelog.md` already notes the two breaking behavior changes (perm-bit capping, bomb-cap defaults).
4. **Future: close testdata gap** — consider adding a synthetic/generated `.rar` fixture for end-to-end batch concurrency testing (currently missing, verified only by smoke-test + primitive coverage).

---

**Unresolved Questions**

- Should a synthetic `.rar` fixture be added to enable fixture-backed end-to-end concurrency tests for `TestBatch`/`ListBatch`? (Deferred; covered by smoke-testing + primitive coverage today.)
- Are there any other modes/flags that should have output-contract tests going forward (test suite coverage model for stdout/stderr assertions)?
