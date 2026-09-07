# Contributing to macrarcli

Thanks for your interest in improving macrarcli. It's a small, pure-Go CLI
that extracts RAR archives directly (list/extract/extract-flat/test) with no
`unrar` dependency.

## Development

Requires the Go version in [`go.mod`](go.mod).

```sh
make build   # build ./bin/macrarcli
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -w .
```

Before opening a PR, make sure all three pass:

```sh
go test ./...
go vet ./...
test -z "$(gofmt -l .)"
```

Some tests are **fixture-gated** and skip when no `testdata/*.rar` is present —
RAR is a proprietary creation format and cannot be generated programmatically,
so binary fixtures are not committed. The core logic is still covered through
interface seams and synthetic inputs.

## Project layout

- `main.go`, `cli_args.go`, `json_output.go`, `list_output.go` — the CLI (package `main`).
- `internal/rarutil/` — the extraction engine: native RAR decode, staging-directory
  write with atomic commit-or-rollback, the shared emitter (bomb caps, dedup),
  overwrite policies, `List`/`Test`, password resolution, and progress tracking.
  Keep files focused and under ~200 lines.

## Conventions

- **Commits:** Conventional Commits (`feat:`, `fix:`, `perf:`, `docs:`, `build:`,
  `test:`, `chore:`, `refactor:`). Keep each commit focused.
- **Comments** explain the *why* (invariants, security trade-offs), not the
  origin (no plan/issue codes in code or test names).
- **TDD** for behavior changes: add a failing test first, then the fix.
- **No new runtime dependencies** beyond `golang.org/x/term` (used only for
  masked password prompting) — the project's value is being `unrar`-free and
  dependency-light; there is no shell-out fallback path.

## Security

This tool reads untrusted archives. Preserve the hardening already in place:
Zip-Slip / absolute-path sanitization, symlink and non-regular-mode
neutralization, decompression-bomb caps (`--max-size` / `--max-entries`), the
staging-directory-then-atomic-commit write path (a failure rolls back
everything, never leaves a partial extraction in the destination), the
post-sanitize name-collision guard, and `--overwrite` never following a
symlink at the destination. If you touch the emitter, sanitizer, or
staging/commit code, add a test proving the invariant still holds. Report
sensitive issues privately rather than in a public issue.
