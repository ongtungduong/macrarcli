# Phase 01 — Security Hardening

## Context
Report: `plans/reports/parallel-codebase-audit-260907-1011-architecture-performance-security-macrarcli-report.md`, Security Important #1/#2/#3/#4.

## Findings addressed
1. World-writable extracted files (`writer.go:172` chmod, `sanitize.go:45` `safeMode`).
2. `--password` argv exposure — add `$MACRARCLI_PASSWORD` fallback.
3. TOCTOU symlink race in `stage.go` cross-filesystem rename fallback.
4. Decompression-bomb caps default to unlimited — ship non-zero defaults.

## Changes

### 1. Perm-bit cap (`internal/rarutil/sanitize.go`)
`safeMode` masks out group/other-write bits after stripping type bits:
```go
perm := m.Perm() &^ 0o022
```
Update `safe_mode_test.go` expectations: `0o777` inputs now want `0o755` (both the "symlink bit stripped" file case and "symlink-as-dir" case). `extract_test.go` computes its expectation by calling `safeMode()` itself — no change needed there.

### 2. Password env var (`cli_args.go` + `main.go`)
Add pure helper in `cli_args.go`:
```go
func resolveExplicitPassword(flagVal string, getenv func(string) string) string
```
`flagVal` wins if non-empty, else `getenv("MACRARCLI_PASSWORD")`. Wire into `run()` right after flag parsing, before the `headerEncrypted` probe. Update `--password` flag usage string to mention the env var. Test in `cli_args_test.go` (flag-wins / env-fallback / both-empty).

### 3. TOCTOU O_NOFOLLOW (`internal/rarutil/stage.go`)
Add two build-tagged files:
- `internal/rarutil/nofollow_unix.go` (`//go:build unix`): `const noFollowFlag = syscall.O_NOFOLLOW`
- `internal/rarutil/nofollow_other.go` (`//go:build !unix`): `const noFollowFlag = 0`

`renameOrCopy`'s fallback `os.OpenFile` gains `|noFollowFlag` in its flag bits. Verify cross-compile: `GOOS=linux`, `GOOS=darwin`, `GOOS=windows` all `go build ./...` clean.
No new unit test: forcing the `os.Rename` EXDEV fallback portably needs a real cross-filesystem mount, not reproducible in this test environment. Note this as a verification gap in the fix report rather than fabricating a test.

### 4. Non-zero bomb-cap defaults (`main.go`)
```go
const (
	defaultMaxSize    = "20G"
	defaultMaxEntries = 200000
)
```
Use as `fs.StringVar(&maxSize, "max-size", defaultMaxSize, ...)` / `fs.IntVar(&maxEntries, "max-entries", defaultMaxEntries, ...)`. Update usage strings ("default 20G", "default 200000"; `0` still means unlimited, explicit opt-out documented). Add `TestDefaultCaps` in `main_test.go` (mirrors existing `TestDefaultJobs` pattern) asserting `parseSize(defaultMaxSize) == 20<<30` and `defaultMaxEntries == 200000`.

## Docs
- README.md: `--password` row (env var), `--max-size`/`--max-entries` rows (new defaults), Security section (mention perm-bit capping).
- `docs/project-changelog.md`: note the two behavior changes (bomb-cap defaults, perm-bit capping) as they affect existing users.

## Todo
- [ ] `safeMode` perm mask + test update
- [ ] Password env var + `cli_args_test.go` coverage
- [ ] O_NOFOLLOW build-tag files + stage.go wiring + cross-GOOS build check
- [ ] Non-zero cap defaults + `main_test.go` coverage
- [ ] README + changelog updates

## Success Criteria
`go build ./...` and `go vet ./...` clean on darwin; `go build ./...` clean under `GOOS=linux` and `GOOS=windows` (cross-compile only); `go test ./...` green.
