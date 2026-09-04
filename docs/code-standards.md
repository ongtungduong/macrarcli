# Code Standards & Conventions

## Guiding Principles

- **YAGNI**: You Aren't Gonna Need It. Implement exactly what the requirements specify, nothing more.
- **File discipline**: Keep `.go` files in `internal/convert/` under ~200 lines. Use composition over inheritance.
- **Security first**: This tool reads untrusted archives. Preserve hardening invariants in any change to sanitize, emit, or fallback paths.
- **Testability**: Design interfaces (`headerReader`) to enable synthetic testing without real RAR files.
- **Dependency minimalism**: No new runtime dependencies on the native path — its value is being `unrar`-free.

## Code Organization

### File Naming

Go files follow standard Go naming conventions:
- `main.go` — package main entry point
- `cli_args.go` — CLI argument parsing and validation
- `json_output.go`, `list_output.go` — output formatting
- `convert.go` — engine entry point
- `emit.go` — ZIP write logic
- `sanitize.go` — path/name hardening
- `fallback.go` — system tool integration
- Etc. (semantic names describing the concern; no abbreviated names)

### File Size

Target ~100-150 lines per file; hard limit ~200 lines for `internal/convert/*.go`.

**Rationale**: Easier to review, test, and reason about. If a file approaches 200 lines, split it:
- By logical concern (e.g., separate `verify.go` from core `convert.go`)
- By domain (e.g., `freespace_unix.go` vs `freespace_other.go` for platform-specific code)

## Naming Conventions

### Functions & Methods

- **Exported** (public API): `PascalCase`
  - `Convert()`, `RunBatch()`, `List()`, `sanitize()` — wait, `sanitize` is lowercase. See below.
- **Unexported** (internal): `camelCase`
  - `convertNative()`, `convertViaFallback()`, `checkCount()`, `resolveName()`

Note: Some unexported functions use lowercase to indicate internal status (e.g., `sanitize()`, `safeMode()` are unexported helpers used internally by `Convert()`).

### Variables & Constants

- **Package-level constants**: `UPPER_SNAKE_CASE` or descriptive `camelCase` depending on visibility
  - `errLimitBytes`, `errLimitEntries` (internal; camelCase)
  - `ContentType = "application/zip"` (if exported; would use descriptive name)
- **Local variables**: `camelCase`
  - `tempFile`, `entriesProcessed`, `availableSpace`
- **Struct fields**: `PascalCase` (always exported in Go; prefix with `_` if internal struct)
  - `type Job struct { InputPath string; OutputPath string; ... }`

### Interfaces

- Named with `-er` suffix when describing a behavior
  - `headerReader` — something that reads headers
  - `io.Writer`, `io.Reader` (stdlib conventions)

## Comments & Documentation

### When to Comment

1. **Why, not what**: Comments explain the invariant, trade-off, or non-obvious design choice.
   - ✅ `// Zip-Slip defense: reject any name with .. or absolute path`
   - ❌ `// Set name to sanitized value`

2. **Security invariants**: Always document why a check is present.
   - ✅ `// Strip S_IFLNK/S_IFBLK/S_IFCHR to prevent symlink/device escapes`
   - ❌ `// Modify mode`

3. **Non-obvious performance choices**:
   - ✅ `// Pooled 512KB buffer: reduces syscall frequency and per-entry allocations (~7% throughput gain)`
   - ❌ `// Use buffer pool`

4. **Deviations from simplicity**:
   - ✅ `// ErrSkipped marks a skipped archive (not a failure); batch continues`
   - ❌ `// Custom error type`

### Comment Style

- **Exported functions**: Doc comments (start with function name)
  ```go
  // Convert transforms a RAR archive to ZIP with bomb defense and Zip-Slip hardening.
  func Convert(job Job) error { ... }
  ```
- **Internal logic**: Inline comments explaining tricky sections
  ```go
  // Dedup logic: same raw name (e.g. multi-volume repeat) gets renamed to (1), (2), ...
  // Different raw names colliding after sanitization is a hard error (data-loss prevention).
  name = resolveName(origName, knownNames)
  ```

### No Plan References

Never reference plan codes, issue numbers, or audit labels in code comments:
- ❌ `// Per F13, implement advisory lock` (F13 is a finding code from a plan)
- ❌ `// Phase 2 security hardening: add bomb cap` (phase number can change)
- ✅ `// Decompression-bomb defense: cap total uncompressed size`

The *reason* for code must be stable and self-contained. Plan references belong in `plans/` and PR descriptions, not in code.

## Error Handling

### Error Types

- Use built-in `error` interface for simplicity
- Create custom error types only when callers need to distinguish categories:
  - `ErrSkipped` — special case for batch continue-on-error
  - `ErrNotFound`, `ErrInvalid` — only if multiple callers need to pattern-match

### Error Messages

- Start with context (where/what), then reason:
  - ✅ `"output file exists (use -f to overwrite)"`
  - ✅ `"max size 1GB exceeded (decompression bomb?)"`
  - ❌ `"error"`, `"failed"`
- No internal stack traces in user-facing messages; log with context for debugging

## Testing Conventions

### Test Location & Naming

- Place `*_test.go` in the same package (not `_test` package suffix)
- Test name format: `TestFunctionName_Scenario` or `TestFunctionName_EdgeCase`
  - ✅ `TestSanitize_TraversalReject`, `TestZipEmitter_BombCapEnforced`
  - ❌ `TestSanitize_1`, `Test_F5_Finding`

### Test Structure

1. **Arrange**: Set up inputs
2. **Act**: Call the function
3. **Assert**: Check outputs and error state

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

Tests that depend on real RAR files should skip gracefully if fixtures are absent:

```go
func TestConvertFixture_RealRAR(t *testing.T) {
    if _, err := os.Stat("testdata/sample.rar"); os.IsNotExist(err) {
        t.Skip("testdata/sample.rar not found (proprietary format, not committed)")
    }
    // Test implementation
}
```

### Synthetic Testing

Use interface seams to test bomb caps and sanitization without real RAR files:

```go
type mockHeaderReader struct {
    headers []*tar.Header
}
func (m *mockHeaderReader) Next() (*tar.Header, error) { ... }
```

## Security Review Checklist

Before committing changes to `sanitize.go`, `emit.go`, or `fallback.go`:

- [ ] Zip-Slip invariant preserved: all entry names sanitized before ZIP write
- [ ] Symlink/device bits stripped: `safeMode()` called on all mode values
- [ ] Bomb caps enforced: `cappedWriter` used for stream writes, `checkCount()` for entries
- [ ] Atomic writes: temp file + rename pattern intact, no partial outputs
- [ ] Post-sanitize collision guard: `dedupVariant()` prevents silent overwrites
- [ ] Argv hardening (fallback): `--` end-of-options + `safeArgPath()` for tool args
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
- `fix(emit): enforce decompression-bomb cap on all paths`
- `feat: add --list for archive preview without conversion`
- `perf(batch): use pooled 512KB buffer for large-entry streaming`
- `docs: clarify fallback bomb-cap gap in README`

**Subject**:
- Imperative mood ("add" not "adds" or "added")
- ~50 characters max
- No period at end

**Body**: Explain *why* not what. Wrap at ~72 characters. Reference related issues if any (e.g., "Closes #42").

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
    // ...
}
```

### Interface Design

```go
// ✅ Accept interfaces, return concrete types
func Process(r io.Reader) ([]byte, error) { ... }

// ❌ Accept and return interfaces (unless polymorphism is needed)
func Process(r Reader) Reader { ... }
```

### Context & Cleanup

```go
// ✅ Use defer for cleanup
tempFile, err := ioutil.TempFile(dir, "rar2zip-*")
defer os.Remove(tempFile.Name())  // safe even if file doesn't exist

// ❌ Manual cleanup after each branch
if err != nil {
    os.Remove(tempFile.Name())
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

All three must pass. Use `go test -short` to skip heavy tests locally.

### Before PR

- [ ] All tests pass (`make test`)
- [ ] Code formatted (`make fmt`)
- [ ] Vet clean (`make vet`)
- [ ] Commit messages follow Conventional Commits
- [ ] No new runtime dependencies on native path
- [ ] Comments explain the *why* (invariants, trade-offs)
- [ ] Security review checklist passed (if touching security-sensitive code)

## External Tool Integration (Fallback)

If adding new external tool support (e.g., `xz` fallback):

1. **Argv hardening**: Use `--` end-of-options + `safeArgPath()` to defuse tool-option injection
2. **Error clarity**: Wrap tool errors with context (tool name, command)
3. **Testability**: Mock the tool invocation; don't require it for core tests
4. **Documentation**: Update README "Security" section with any new threat model changes

## Performance Considerations

### Acceptable Optimizations

- Pooled buffers (e.g., 512 KB copy buffer) — reduces syscall frequency
- Concurrent batch processing (bounded by `--jobs`) — leverages multi-core
- Lazy decompression reading (stream via `io.Copy`) — keeps memory flat

### Avoid Over-Optimization

- Don't sacrifice readability for 1-2% throughput gains
- Measure before optimizing (use `go test -bench`)
- Document performance trade-offs in comments

## Module Visibility

- **`main` package**: Public API (CLI flags, entry point)
- **`internal/convert`**: Private implementation (never import outside this tool)
  - Exception: interface design (e.g., `headerReader`) allows synthetic testing

This keeps the API surface small and gives future maintainers freedom to refactor internals.
