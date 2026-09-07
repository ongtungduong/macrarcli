# Phase 02 — CLI Dispatch Unification (Perf #1 + Arch #1 + Arch #2)

## Context
Report: `plans/reports/parallel-codebase-audit-260907-1011-architecture-performance-security-macrarcli-report.md`.
These three findings are grouped because they all touch `main.go`/`list_output.go`'s dispatch layer and the fix for one enables the others.

## Findings addressed
- Perf #1: `-t`/`-l` ignore `--jobs`, run sequentially.
- Arch #1: `runList` can't receive `OnVerbose`/future `Options` fields; `main.go` doesn't even pass `verbose` through to it.
- Arch #2: `report`/`runTest`/list's report loop are 3 near-identical hand-rolled loops.

## Design

### Generic concurrency helper (`internal/rarutil/batch.go`)
```go
func runParallel[T, R any](items []T, maxParallel int, fn func(T) R) []R
```
Semaphore + WaitGroup, same shape `RunBatch` already had. `RunBatch` becomes a thin wrapper over `runParallel` (no behavior change, existing `batch_test.go` must still pass unmodified).

### New batch entry points
- `rarutil.TestBatch(srcs []string, opts Options, maxParallel int) []Result` (`batch.go`) — `Result{Job: Job{Src: src}, Err: Test(src, opts)}` via `runParallel`.
- `rarutil.ListResult{Src string; Entries []EntryInfo; Err error}` (`list.go`) + `rarutil.ListBatch(srcs []string, opts Options, maxParallel int) []ListResult` (`batch.go`) via `runParallel`.

### `main.go` dispatch
```go
case modeList:
    return runList(inputs, opts, jobs, jsonOut)
case modeTest:
    return runTest(inputs, opts, jobs, jsonOut, quiet)
```
`opts` (built once, already carries `Password`, `OnVerbose`, etc.) replaces the ad hoc `(password, maxEntries)` pair `runList` took before — this is what closes Arch #1: `runList` can now receive any current or future `Options` field with zero signature change.

### `list_output.go`
```go
func runList(inputs []string, opts rarutil.Options, maxParallel int, jsonOut bool) int {
    results := rarutil.ListBatch(inputs, opts, maxParallel)
    archives := make([]listedArchive, len(results))
    for i, r := range results {
        archives[i] = listedArchive{Src: r.Src, Entries: r.Entries, Err: r.Err}
    }
    ...
}
```
`listedArchive`, `printList`, `reportListJSON`, `listExitCodes` unchanged — list's output is a table, not a line, so it is NOT forced into the shared line-reporter below (would be a worse-fitting abstraction). This is the one loop the report itself expected to stay distinct; only the concurrency + input-plumbing gap is closed.

### Shared human reporter (`main.go`)
```go
func reportHuman(results []rarutil.Result, quiet, showPartial bool, successLine func(rarutil.Result) string) int {
    // body = current report()'s loop, parameterized by successLine and
    // whether the summary line shows a "partial" column (extract: yes, test: no)
}

func report(results []rarutil.Result, quiet bool) int {
    return reportHuman(results, quiet, true, func(r rarutil.Result) string {
        return fmt.Sprintf("extracted %s -> %s", r.Src, r.Dst)
    })
}

func runTest(inputs []string, opts rarutil.Options, maxParallel int, jsonOut, quiet bool) int {
    results := rarutil.TestBatch(inputs, opts, maxParallel)
    if jsonOut {
        return reportTestJSON(results, os.Stdout)
    }
    return reportHuman(results, quiet, false, func(r rarutil.Result) string {
        return fmt.Sprintf("%s: OK", r.Src)
    })
}
```
Byte-for-byte identical stdout/stderr output to today for both modes — this is a pure refactor, verified by existing `TestRun_TestFixture`/`TestRun_Batch`/`TestRun_Extract` (no new output-format tests needed, but re-run all of them).

## Concurrency safety notes
- `opts.OnEntry` (progress) is only ever attached when `len(inputs) == 1` (existing `attachProgress` gate in `main.go`, unchanged) — so concurrent `TestBatch`/`ListBatch` runs (`len(inputs) > 1`) never have `OnEntry` set. No new race.
- `opts.OnVerbose` is already documented safe for concurrent use (`extract.go:22`); `List`/`Test` don't call it today, so no new call sites are added by this phase.

## Todo
- [ ] `runParallel` generic helper + `RunBatch` rewritten on top of it
- [ ] `TestBatch` + `ListResult`/`ListBatch`
- [ ] `main.go` dispatch passes `opts`/`jobs` to both `runList` and `runTest`
- [ ] `list_output.go` `runList` signature + body updated
- [ ] `reportHuman` extracted; `report`/`runTest` rewritten on top of it
- [ ] Update `docs/system-architecture.md`'s `RunBatch` mention if it now materially differs (skim only, don't chase unrelated doc drift)

## Success Criteria
- `--test`/`--list` with `--jobs N>1` on a multi-archive input measurably runs concurrently (verified by code path, not a timing-flaky test).
- All existing tests pass unmodified in behavior (`batch_test.go`, `main_test.go` fixture tests, `list_output` — no `list_output_test.go` exists today, none added beyond what's needed).
- `go vet ./...` clean.
