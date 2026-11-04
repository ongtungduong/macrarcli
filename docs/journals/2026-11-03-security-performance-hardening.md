# Security and performance hardening

Date: 2026-11-03
Status: implemented and locally verified; not committed or released.

## Changes

- Overwrite originals survive failed commits. Recovery failures retain data and
  report a recovery directory instead of deleting the remaining backup.
- Keep staging, backup, and cleanup roots pinned. Verify object identity after
  moving it into a private area before deletion. Directory-only cleanup cannot
  delete a file substituted by another process.
- Publish via descriptor-relative hard links on Unix; use exclusive copying
  where unavailable, including experimental Windows support. Preserve file
  metadata through handles, including nanosecond times on Linux/macOS.
- Cache duplicate/rename suffixes; refuse filesystem aliases and unsafe Windows
  paths. Keep `a:b` rejected: it is a Windows drive-relative path.
- Default each decoder dictionary to 256 MiB, configurable with `--max-dict`.
  Count flat directory entries, reject size overflow, and remove unused buffers.
- Close header probes, inspect all batch inputs, sanitize human output, and
  match progress by entry order.
- Raise minimum Go to 1.26.10. The first scan found reachable standard-library
  root-escape issues in Go 1.26.2; the final scan reports no vulnerabilities.
- Add CI race and vulnerability checks; document recovery and resource limits.

## Evidence

- `go test -race ./...`: passed both packages on macOS arm64.
- `go vet ./...`: passed.
- `govulncheck@v1.1.4 ./...` with Go 1.26.10: no vulnerabilities found.
- Cross-builds: Windows amd64, Linux arm64, macOS amd64 passed.
- Duplicate benchmark: 10,000 repeated names approximately 14.8 ms in this run.
  Earlier audit measured approximately 10 seconds for 10,000 with rescanning.
  The measurements are indicative, not a controlled end-to-end throughput test.
- Regressions cover overwrite rollback, original symlinks, failed restoration,
  path substitution, directory/file replacement, copy failure, dictionary caps,
  flat entry counts, terminal escapes, ordered progress, and file metadata.

## Scope and limits

Listing remains header iteration to avoid a full extra decode pass for ordinary
archives. Solid archives can decompress inside `Next`; listing has no byte/CPU
budget. The dictionary cap is not an RSS limit. Extraction is not crash-atomic
or isolated from other processes; recovery data can remain after errors. Windows
was cross-compiled, not runtime-tested. Optional archive-fixture tests still
skip when fixtures are absent.

Parallel implementation agents produced the initial patches. The parent
completed work after some agents hit usage limits. An independent follow-up
review identified additional filesystem races; these received regression tests
and fixes before final verification.
