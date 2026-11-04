# System Architecture

## Dispatch and concurrency

The CLI parses flags and validates mode/overwrite exclusions before opening
archives. Password precedence is `--password`, `MACRARCLI_PASSWORD`, then one
masked terminal prompt if any input has encrypted headers. Each successful
probe closes immediately. Content-only encryption may require an explicit
password because the header probe does not detect it.

`ListBatch`, `TestBatch`, and `RunBatch` share a bounded semaphore dispatcher.
They continue after per-job failures and return results in input order.
`commitMu` serializes extraction commits within the process; decoding remains
parallel. It is not a cross-process lock.

## Archive readers

Every backend open uses shared password/dictionary options. CLI header probes
use the same dictionary cap. The default dictionary budget is 256 MiB per reader
and may be raised with `--max-dict`. It is not a bound on total process memory.

| Operation | Data handling | Limits |
|-----------|---------------|--------|
| List | Header iteration, no output files | Entry count and dictionary |
| Test | Read each file to discard, surface supported checksum errors | Bytes, entries, dictionary |
| Extract | Read into private staging, then commit | Bytes, entries, dictionary |

Advancing solid archive headers can decompress internally. `List` deliberately
keeps header iteration rather than decoding every non-solid file; `--max-size`
does not apply there. Listing and the progress pre-pass have no CPU/time cap.
The decoder validates supported checksums when present, not authenticity.

## Filesystem boundary

`Extract` opens the destination root before creating staging. The decoder
borrows an opened staging root; publication, rollback, and cleanup use the
pinned destination root. Renaming the original directory path or swapping an
intermediate path to an outside symlink cannot redirect those operations outside
the root on supported patched Go platforms.

`sanitize` rejects escaping, absolute, drive-relative, device, stream, and
ambiguous Windows paths. `a:b` is intentionally rejected because it is a
Windows drive-relative path. Validation happens before flattening. List/test
names are not used as filesystem paths.

`safeMode` strips symlink/device and special permission bits, plus group/other
write bits. Archive link bodies are stored as inert regular-file content.
Files are created exclusively in staging; tracked directory spellings prevent
case/normalization aliases from merging implicitly on insensitive filesystems.
Permissions and modification time are applied to an open file handle, avoiding
path-based metadata races. Linux/macOS timestamps preserve nanoseconds; Windows preserves its native 100 ns precision.

## Publication and rollback

1. Walk the staged tree; check existing destination directories and per-file
   overwrite policy. Refuse pre-existing symlink directories.
2. For `--overwrite`, move the existing file or symlink itself into a private
   `.macrarcli-backup-*` directory under the destination.
3. Publish a completed staged file with a hard link (no byte copy). If unavailable,
   open the destination exclusively and copy bytes plus safe metadata. Never
   truncate a path created concurrently after collision checks.
4. Record each publication's identity and backup before proceeding.
5. On failure, reverse publications, restore backups without replacing concurrent
   files, and remove only directories created by this commit. Report recovery
   errors and retain any remaining originals for manual recovery.
6. On success, remove the backups; root-relative cleanup removes staging.

Unexpected replacement objects are moved to a private recovery area and verified
before deletion; mismatches are retained. Backup and cleanup roots remain pinned.
A cross-mount backup rename can fail before modifying that entry. Exclusive-copy
fallback may be visible while incomplete. A replacement has a visibility gap
between backup and publication. A cleanup failure can return an error after
outputs have committed. The destination directory may be created even if decoding
fails. This is not a crash-durable transaction and does not restore directory
metadata exactly or isolate other processes.

## Collision handling and performance

Both archive duplicate resolution and destination `--rename` cache the next
suffix per base name. Previously repeated duplicate searches restarted at one
and grew quadratically. Unexpected stat errors now terminate probing.

Exact repeated archive names are retained under `(n)` variants. Distinct raw
names that sanitize to the same source path fail. Flat extraction may merge
basenames from different source directories, preserving both with suffixes.
Filesystem aliases fail safely instead of overwriting staged data.

Streaming uses `io.Copy`; rardecode supplies `WriterTo`. The previous 512 KiB pool
was bypassed by that interface and has been removed. Integer overflow cannot
bypass the byte cap or CLI size parsing.

## Output and exit codes

Single-archive human progress uses a listing pre-pass and ordered entry sizes,
not output-name lookup. Flat directory entries are omitted from progress;
unknown or overflowing totals use completed-entry counts. Quiet/JSON modes avoid
the pre-pass. Human strings are sanitized before terminal display; JSON retains
original values through `encoding/json` escaping.

Exit codes remain 0 success, 1 usage, 2 password, 3 checksum failure, and 4 other
runtime error. Batch precedence is 2 > 3 > 4. Legacy RAR3/4 wrong passwords may
produce checksum errors and exit 3; only RAR5 provides reliable password errors.
