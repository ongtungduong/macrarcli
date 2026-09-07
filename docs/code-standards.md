# Code Standards & Conventions

## Guiding Principles

- **YAGNI**: Implement exactly what the spec requires, nothing more.
- **File discipline**: Keep `.go` files under ~200 lines. Use composition.
- **Security first**: This tool reads untrusted archives. Preserve hardening invariants in any change to `sanitize.go`, `writer.go`, or `stage.go`.
- **Testability**: Design interfaces to enable synthetic testing without real RAR files.
- **Dependency minimalism**: No new runtime dependencies on the native path — the value is being `unrar`-free.

## Code Organization

### File Naming

Go files follow standard conventions:
- `main.go` — package main entry point
- `cli_args.go` — argument parsing and validation
- `json_output.go`, `list_output.go` — output formatting
- Files in `internal/rarutil/` use semantic names describing the concern (e.g., `sanitize.go`, `stage.go`, `extract.go`)
- Platform-specific code: `overwrite_test.go`, etc.

### File Size

Target ~80–150 lines per file; hard limit ~200 for `internal/rarutil/*.go`.

**Rationale**: Easier to review, test, and reason about. If a file approaches 200 lines, split by concern:
- Separate concerns into different files (e.g., `writer.go` for stream enforcement, `sanitize.go` for path hardening)
- Platform-specific code into separate `_unix.go`/`_windows.go`-suffixed files if the need arises (no file in `internal/rarutil/` currently needs this split)

## Naming Conventions

### Functions & Methods

- **Exported** (public API): `PascalCase`
  - `Extract()`, `List()`, `Test()`, `RunBatch()`, `TestBatch()`, `ListBatch()` — operations users/tests call
- **Unexported** (internal): `camelCase`
  - `sanitize()`, `safeMode()`, `commitStaged()`, `extractToStaging()`

### Variables & Constants

- **Package-level constants**: `UPPER_SNAKE_CASE` or `camelCase` depending on visibility
  - `errLimitBytes`, `errLimitEntries` (internal sentinels)
  - `maxDefaultJobs` (threshold constant)
- **Local variables**: `camelCase`
  - `stagingDir`, `entriesProcessed`, `resolvedPassword`
- **Struct fields**: `PascalCase` (always exported in Go)
  - `type Job struct { Src string; Dst string; Password string }`
  - `type Result struct { Job Job; Err error; SkippedEntries []string }`

### Interfaces

Named with `-er` suffix when describing behavior:
- `headerReader` — something that reads headers
- `io.Writer`, `io.Reader` (stdlib patterns)

## Comments & Documentation

### When to Comment

1. **Why, not what**: Explain the invariant or non-obvious design choice
   - ✅ `// Zip-Slip defense: reject any path with .. or absolute prefix`
   - ❌ `// Check if name is valid`

2. **Security invariants**: Always document why a check exists
   - ✅ `// Strip S_IFLNK/S_IFBLK/S_IFCHR to prevent symlink/device escapes`
   - ❌ `// Modify file mode`

3. **Non-obvious performance choices**
   - ✅ `// Pooled 64KB buffer: reduces per-entry allocations (~5% throughput gain)`
   - ❌ `// Use buffer pool`

4. **Deviations from simplicity**
   - ✅ `// ErrLimitEntries signals bomb cap exceeded; batch continues instead of aborting`
   - ❌ `// Custom error type`

### Comment Style

- **Exported functions**: Doc comments (start with function name)
  ```go
  // Extract reads the RAR archive at srcRar and writes its contents
  // to destDir, staging to a temp directory and committing atomically.
  func Extract(srcRar, destDir string, opts Options) ([]string, error) { ... }
  ```

- **Internal logic**: Inline comments explaining tricky sections
  ```go
  // Sanitize before write: strip traversal patterns, enforce relative paths.
  name, err := sanitize(hdr.Name)
  ```

### No Plan References

Never reference plan phases, finding codes, or audit labels in code comments:
- ❌ `// Per Phase 2, implement symlink stripping`
- ❌ `// Finding F13: add decompression cap`
- ✅ `// Zip-Slip defense: cap total uncompressed size`

The *reason* for code must be stable and self-contained. Plan references belong in `plans/` and PR descriptions, not in code.

## Error Handling

### Error Types

Use built-in `error` interface for simplicity. Create custom error types only when callers need to distinguish categories:
- `errLimitBytes`, `errLimitEntries` — decompression-bomb sentinels
- Use `errors.Is()` to match against rardecode sentinels (`ErrBadPassword`, `ErrBadFileChecksum`)

### Error Messages

Start with context (what operation, where), then reason:
- ✅ `"destination exists (use --overwrite to replace)"`
- ✅ `"max size 1GB exceeded (decompression bomb?)"`
- ❌ `"error"`, `"failed"`

## Testing Conventions

### Test Location & Naming

- Place `*_test.go` in the same package
- Test name format: `TestFunctionName_Scenario`
  - ✅ `TestSanitize_TraversalReject`, `TestExtract_DestinationCollision`
  - ❌ `TestSanitize_1`, `Test_Issue42`

### Test Structure

```go
func TestSanitize_TraversalReject(t *testing.T) {
    // Arrange
    badName := "../../etc/passwd"
    
    // Act
    result, err := sanitize(badName)
    
    // Assert
    if err == nil {
        t.Errorf("expected error for traversal path, got none")
    }
}
```

### Fixture-Gated Tests

Tests depending on real RAR files skip gracefully:

```go
func TestExtract_RealArchive(t *testing.T) {
    if _, err := os.Stat("testdata/sample.rar"); os.IsNotExist(err) {
        t.Skip("testdata/sample.rar not found (proprietary format)")
    }
    // Test implementation
}
```

### Synthetic Testing

Use interface seams to test bomb caps and sanitization without real RAR files:

```go
type mockHeaderReader struct {
    headers []*rardecode.FileHeader
}
func (m *mockHeaderReader) Next() (*rardecode.FileHeader, error) { ... }
```

## Security Review Checklist

Before committing changes to `sanitize.go`, `writer.go`, or `stage.go`:

- [ ] Zip-Slip invariant preserved: all entry names sanitized before write
- [ ] Symlink/device bits stripped: `safeMode()` called on all mode values
- [ ] Bomb caps enforced: `cappedWriter` applied for stream writes, entry count checked
- [ ] Atomic writes: temp file + rename pattern intact, no partial outputs on error
- [ ] Post-sanitize collision guard: collision handling prevents data loss
- [ ] Test added: synthetic or fixture-gated test verifying the invariant

## Commit Message Format

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**: `feat`, `fix`, `perf`, `docs`, `build`, `test`, `chore`, `refactor`

**Examples**:
- `fix(sanitize): enforce Zip-Slip defense on all paths`
- `feat: add -t/--test for integrity validation`
- `perf(batch): use pooled buffer for concurrent jobs`

**Subject**: Imperative mood ("add" not "adds"), ~50 chars, no period.

**Body**: Explain *why* not what. Wrap at ~72 chars.

## Go Idioms & Best Practices

### Error Handling

```go
// ✅ Check and return early
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
result := doSomething()

// ❌ Nested ifs
if err == nil {
    result := doSomething()
}
```

### Interface Design

```go
// ✅ Accept interfaces, return concrete types
func Process(r io.Reader) ([]string, error) { ... }

// ❌ Accept and return interfaces
func Process(r Reader) Reader { ... }
```

### Context & Cleanup

```go
// ✅ Use defer for cleanup
tempDir, err := os.MkdirTemp(destDir, "macrarcli-*")
defer os.RemoveAll(tempDir)  // safe even if already removed

// ❌ Manual cleanup after each branch
if err != nil {
    os.RemoveAll(tempDir)
    return err
}
```

## Development Workflow

### Before Commit

```sh
make fmt    # gofmt -w .
make vet    # go vet ./...
make test   # go test ./...
```

All three must pass.

### Before PR

- [ ] All tests pass (`make test`)
- [ ] Code formatted (`make fmt`)
- [ ] Vet clean (`make vet`)
- [ ] Commit messages follow Conventional Commits
- [ ] No new runtime dependencies on native path
- [ ] Comments explain the *why* (invariants, trade-offs)
- [ ] Security review checklist passed (if touching security code)

## Module Visibility

- **`main` package**: Public API (CLI entry point, flags)
- **`internal/rarutil`**: Private implementation (never import outside this tool)
  - Exception: interface design (`headerReader`) for synthetic testing

This keeps the API surface small and preserves freedom to refactor internals.
