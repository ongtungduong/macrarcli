# Troubleshooting

## Exit Code 2: Wrong or Missing Password (RAR5 Only)

If you see:
```
macrarcli: archive.rar: wrong password or corrupted entry
exit 2
```

This indicates a password-protected RAR5 archive. Provide the correct password:

```bash
macrarcli --password 'your-password' archive.rar
```

If the password is interactive (TTY), macrarcli prompts for one:
```bash
macrarcli archive.rar
# Prompts: Enter password for archive.rar:
```

**Known limitation**: Legacy RAR3/4 archives with wrong passwords **cannot be reliably distinguished from corruption**. The `rardecode` library returns the same error for both. A wrong password on RAR3/4 may report exit code 3 (corruption) instead of 2 (password). This is a library limitation, not a bug.

## Exit Code 3: Corrupted Entry or Bomb Cap Exceeded

If you see:
```
macrarcli: archive.rar: corrupted entry or max size exceeded
exit 3
```

This indicates either:
1. A genuinely corrupted archive (CRC32 mismatch during extraction or validation)
2. A decompression-bomb cap was exceeded

**To distinguish**:
```bash
# Test without writing (validates checksums only)
macrarcli -t archive.rar

# If this also fails with exit 3, the archive is likely corrupted
# If this succeeds but extraction fails, likely a bomb-cap hit
```

**If a bomb cap was hit**:
Increase or disable the limit (if you trust the archive):
```bash
# Increase max size to 10 GB
macrarcli --max-size 10G archive.rar

# Disable size limit entirely (not recommended for untrusted archives)
macrarcli --max-size 0 archive.rar

# Increase max entry count
macrarcli --max-entries 1000000 archive.rar
```

**If the archive is corrupted**: Try with an external tool (`unrar`, `7z`) for a second opinion. macrarcli cannot fix corruption.

## Destination Collision

If you see:
```
macrarcli: destination exists (use --overwrite, --skip, or --rename)
exit 1
```

The destination file or directory already exists. Choose a policy:

```bash
# Replace existing destination
macrarcli --overwrite -o out/ archive.rar

# Skip this archive (useful for retrying failed batches)
macrarcli --skip -o out/ archive.rar

# Keep both: rename collider to " (n)"
macrarcli --rename -o out/ archive.rar

# Change destination directory
macrarcli -o out_new/ archive.rar
```

## Non-TTY Password Prompt Fails

If you see:
```
macrarcli: archive is encrypted but no --password given and not a TTY
exit 1
```

The archive is password-protected, and macrarcli cannot prompt on a non-interactive terminal (CI, pipe). Provide the password explicitly:

```bash
# Pass via flag
macrarcli --password 'secret' archive.rar

# Or via environment variable (not recommended)
RAR_PASSWORD=secret macrarcli archive.rar  # Not supported; use --password
```

**Security note**: Avoid passing passwords on the command line in shared environments (visible via `ps`). Prefer `--password` in scripts with proper access controls.

## Flat Extraction (`-e/--flat`) Gotchas

Flat extraction discards directory structure:

```bash
# Input archive structure:
# folder1/file1.txt
# folder2/file2.txt

# After flat extraction:
macrarcli -e archive.rar
# Output:
# file1.txt
# file2.txt
# folder/ (empty dir entries ignored in flat mode)
```

**Name collisions**: If different directories have files with the same name, the overwrite policy applies:
```bash
# Input:
# folder1/readme.md
# folder2/readme.md

# Flat extraction with --rename:
macrarcli -e --rename archive.rar
# Output:
# readme.md (from first entry)
# readme.md (n) (from second entry, if collision)
```

## Windows Quarantine (Gatekeeper)

If you download macrarcli on macOS and see:
```
"macrarcli" cannot be opened because the developer cannot be verified
```

The binary is quarantined by Gatekeeper. Verify the checksum first, then remove the quarantine attribute:

```bash
# Verify checksum (after downloading from release page)
sha256sum macrarcli

# Remove quarantine
xattr -d com.apple.quarantine ./macrarcli

# Now run
./macrarcli --version
```

**Avoid this**: Install via Homebrew or the install script instead — they don't trigger quarantine.

## Filenames Look Garbled (CJK / Non-UTF8 Archives)

If extracted filenames are mojibake (garbled text), the archive was created with a non-UTF8 encoding (e.g., Shift-JIS, GBK, Big5).

**macrarcli limitation**: The pure-Go decoder has no encoding-override flag. Filenames stored in legacy codepages cannot be re-decoded after extraction.

**Workaround**: Use an external tool that supports charset overrides:

```bash
# Extract with unrar, specifying source codepage (e.g., GBK for Chinese)
unrar x -cp936 archive.rar

# Then re-zip if needed
zip -r archive.zip ./extracted_dir
```

macrarcli will then work with the re-zipped archive.

## Installation Fails to Verify

If the `install.sh` script fails:
```
error: no checksum entry for macrarcli_linux_amd64.tar.gz in checksums.txt
```

**Check**:
1. Is `sha256sum` or `shasum` installed? (Required; no fallback)
2. Was the download incomplete? (Re-run the script)
3. Did you specify a custom `VERSION` that doesn't exist?

**Workaround** (only if you understand the risk):
```bash
# Skip checksum verification (NOT RECOMMENDED for piped installs)
SKIP_CHECKSUM=1 curl -fsSL https://raw.githubusercontent.com/ongtungduong/macrarcli/main/scripts/install.sh | sh
```

Never use `SKIP_CHECKSUM=1` with piped installs from the network unless you are absolutely certain of the source.

## Progress Line Looks Broken in CI

In non-TTY environments (CI, log files), the live progress line `[%] name (rate/s)` may appear as garbage:

```
[100%] file.txt (1.2M/s)^[[K
```

This is normal — the progress line uses ANSI escape codes (`\r\033[K`) to redraw in place. The output is correct; it just doesn't render in plain text. Suppress with `-q/--quiet`:

```bash
# In CI: suppress progress output
macrarcli -q archive.rar
```

## Very Large Archives Slow or Hang

If extraction of a 50+ GiB archive appears hung:

1. **Check disk space**:
   ```bash
   df -h
   ```
   Extraction requires free space for the entire extracted tree.

2. **Check progress** (with `--verbose`):
   ```bash
   macrarcli --verbose archive.rar
   # Shows per-archive timing and decode path
   ```

3. **Increase concurrent jobs** (if you have spare cores and disk I/O):
   ```bash
   macrarcli --jobs 8 archive.rar
   # Default is min(NumCPU, 4)
   ```

4. **Or reduce to single-threaded**:
   ```bash
   macrarcli --jobs 1 archive.rar
   ```

macrarcli reads RAR sequentially (streaming decoder) — parallel extraction across multiple archives helps, but a single archive is not parallelized.

## Performance or Memory Issues

**Symptom**: Extraction is slow or uses a lot of memory.

**Check**:
```bash
# Run with verbose to see timing
macrarcli --verbose archive.rar

# Check available memory
free -h  # Linux
vm_stat  # macOS
```

**Tuning**:
- Reduce `--jobs` if disk I/O is bottlenecked
- Increase `--jobs` if CPU/network is bottlenecked (for batch processing)
- Default is `min(NumCPU, 4)` — good balance for most systems

Memory usage is bounded by the size of the largest entry (streamed via 64KB internal buffer). A single 10 GiB entry uses ~64KB in memory during extraction.

## Batch Processing Stops After First Failure

Batch processing is **continue-on-error by default**. A failed archive does not stop the batch:

```bash
macrarcli archive1.rar archive2.rar archive3.rar
# If archive2.rar fails, archive3.rar still processes
# Exit code: highest-priority error (2 > 3 > 4)
```

If you need to stop on first failure, extract one at a time:
```bash
for rar in *.rar; do
  macrarcli "$rar" || exit $?
done
```

## Still Stuck?

Run with `--verbose` to get extra diagnostics:

```bash
macrarcli --verbose archive.rar
# Prints per-archive timing and any internal decode messages
```

Report issues with:
- `macrarcli --version` output
- `macrarcli --verbose` output from your failed extraction
- The command you ran
- The archive (if it's safe to share)

File at: https://github.com/ongtungduong/macrarcli/issues
