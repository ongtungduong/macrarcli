# macrarcli

[![CI](https://github.com/ongtungduong/macrarcli/actions/workflows/ci.yml/badge.svg)](https://github.com/ongtungduong/macrarcli/actions/workflows/ci.yml)

A small CLI for **macOS** and **Linux** that extracts `.rar` archives directly —
list, extract (preserving structure or flat), or validate integrity. **No ZIP
output.**

Built in Go with a **pure-Go RAR decoder** ([`nwaples/rardecode`](https://github.com/nwaples/rardecode)).
The result is a single self-contained binary — no `unrar` / `7z` required on
the user's machine, and no shell-out fallback path to waive that guarantee.

## Install

### Homebrew (macOS / Linux)

```sh
brew install ongtungduong/tap/macrarcli
```

### One-line install script

```sh
curl -fsSL https://raw.githubusercontent.com/ongtungduong/macrarcli/main/scripts/install.sh | sh
```

Installs to `~/.local/bin` (if on `$PATH`) or `/usr/local/bin`. Override with `INSTALL_DIR`:

```sh
INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/ongtungduong/macrarcli/main/scripts/install.sh | sh
```

The script verifies the download against the release `checksums.txt` (a missing
`sha256sum`/`shasum` is a hard error — no silent unverified install). If
[`cosign`](https://github.com/sigstore/cosign) is installed it also verifies the
keyless signature over `checksums.txt` (authenticity). `SKIP_CHECKSUM=1` disables
verification — never use it with a piped install.

### Windows (Scoop) — experimental

```powershell
scoop bucket add macrarcli https://github.com/ongtungduong/scoop-bucket
scoop install macrarcli
```

Windows binaries (`amd64`/`arm64`) are published as `.zip` assets on each
release. Windows support is **experimental** and exercised only on Unix CI.
You can also download a `.zip` from the
[Releases](https://github.com/ongtungduong/macrarcli/releases) page directly.

### From source

```sh
# Go 1.26.10 or newer (security patches required)
go build -o bin/macrarcli .
# or
make build
```

## Usage

```sh
macrarcli [flags] <input.rar> [more.rar ...]
```

By default, extracts every input into the destination directory (cwd, or
`-o/--dest`), preserving its internal directory structure. Examples:

```sh
macrarcli archive.rar                     # extract into cwd, preserving structure
macrarcli -o out/ archive.rar             # extract into out/
macrarcli -e archive.rar                  # extract flat (no subdirectories)
macrarcli -l archive.rar                  # preview contents, write nothing
macrarcli -l --json archive.rar           # structured listing on stdout
macrarcli -t archive.rar                  # validate integrity, write nothing
macrarcli --overwrite -o out/ archive.rar # replace existing destination files
macrarcli --skip -o out/ archive.rar      # skip individually colliding entries
macrarcli --rename -o out/ archive.rar    # keep both under a " (n)" suffix
macrarcli -q archive.rar                  # no progress output
macrarcli *.rar                           # batch: extract every match into cwd
macrarcli --jobs 4 -o out/ *.rar          # extract 4 archives concurrently
macrarcli --password secret locked.rar    # password-protected archive
macrarcli --json *.rar                    # machine-readable summary on stdout
macrarcli --verbose archive.rar           # show per-archive timing on stderr
```

Batch runs are **continue-on-error**: a failed archive is reported but doesn't
abort the rest. By default a batch runs **concurrently** (`--jobs` defaults to
`min(NumCPU, 4)`); per-archive result lines are still printed in input order,
so output stays deterministic. Multi-volume sets (`.part1.rar` / `.r00`) are
followed automatically.

If no `--password` is given, `macrarcli` checks the `MACRARCLI_PASSWORD`
environment variable. With neither set, it probes every input for encrypted
headers, closes each successfully opened probe immediately, and prompts once
(masked, no echo) if needed. In a non-interactive context (CI, a pipe), detected
header encryption fails fast instead of hanging. Content-only encryption may
not be detected by this header probe; supply a password explicitly in that case. The password
is resolved once per invocation and reused across every archive in a batch —
never re-prompted per archive. Prefer `MACRARCLI_PASSWORD` over `--password`
for scripted use: a flag value is visible to other local users via `ps`/
`/proc/<pid>/cmdline` for the life of the process. The environment fallback avoids putting the
password in command-line arguments.

### Flags

| Flag | Description |
|------|-------------|
| `-o`, `--dest <dir>` | Destination directory for extracted files. Default: cwd. Rejected with `-l`/`-t` (they write nothing). |
| `-e`, `--flat` | Extract without preserving directory structure — every file lands directly under the destination by its base name. Mutually exclusive with `-l`/`-t`. |
| `-l`, `--list` | Preview an archive's entries (size, compressed size, encrypted flag, modification time, name) without extracting. Read-only. Mutually exclusive with `-e`/`-t`. |
| `-t`, `--test` | Validate every entry's checksum without extracting anything. Read-only. Mutually exclusive with `-e`/`-l`. |
| `--overwrite` | Replace an existing destination file. Mutually exclusive with `--skip`/`--rename`. Never follows a symlink at the destination — see Security. |
| `--skip` | Skip just the colliding entry, keeping the rest of that archive's extraction. Mutually exclusive with `--overwrite`/`--rename`. |
| `--rename` | Write a colliding entry under a `" (n)"`-suffixed name instead of touching the existing file. Mutually exclusive with `--overwrite`/`--skip`. |
| `-q`, `--quiet` | Suppress progress output (printed to stderr). |
| `--verbose` | Print extra diagnostics to stderr: per-archive timing. Suppressed under `--json`/`--quiet`. |
| `--password <pw>` | Password for encrypted archives. If omitted, falls back to `$MACRARCLI_PASSWORD`, then prompts interactively (see above). |
| `--jobs <n>` | Process up to `n` archives concurrently — applies to extraction and to `-l`/`-t`. Default: `min(NumCPU, 4)`. Per-job results are still printed in input order. |
| `--json` | Emit a machine-readable JSON summary on stdout (suppresses the human progress output). Includes a `mode` field (`extract`/`list`/`test`) and per-job `skippedEntries`. |
| `--max-size <n>` | Cap total decoded bytes per archive in extract and test modes. Accepts bytes or a `K`/`M`/`G` suffix (powers of 1024). Default `20G`; `0` disables this cap. Exceeding it fails before extraction commit. |
| `--max-entries <n>` | Cap the number of entries an archive may contain. Default `200000`; pass `0` for unlimited. Also bounds `-l` and `-t`. |
| `--max-dict <n>` | Cap each decoder dictionary. Default `256M` (256 MiB); accepts bytes or `K`/`M`/`G`. Must be positive; `0` is rejected. Applies to all modes and header probes. This is not a total process memory cap. |
| `--version` | Print version and exit. |
| `-h`, `--help` | Print usage and exit. |

Regular-file **modification time** and safe **Unix permissions** are preserved;
directory metadata is not restored. Extraction first decodes into a private
staging directory under the destination, then publishes entries individually.
Decode failures publish no archive entries. Commit failures attempt rollback;
failed restoration retains backups and reports their location for recovery.
This is **not crash-atomic** and cannot promise an exactly unchanged destination
on every error. See Security below. Progress is written to stderr.

Listing reads headers; it does not enforce `--max-size`. Advancing through a
solid archive can require decompression. Single-archive human progress uses a
listing pre-pass; `-q` or `--json` avoids that extra pass. The dictionary and
entry-count limits apply to listing too, but do not bound CPU time.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success. |
| `1` | Usage error (bad flags, missing/invalid input) — no archive was touched. |
| `2` | Wrong or missing password. **Only reliably guaranteed for RAR5-encrypted archives** — `rardecode` has no equivalent sentinel for legacy RAR3/4, so a wrong password there decrypts to garbage that fails the same checksum check corruption would trigger, and may report exit `3` instead. This is a documented limitation, not a bug. |
| `3` | A corrupted entry (checksum mismatch), or — for a legacy RAR3/4 archive — possibly a wrong password (see above). |
| `4` | Any other runtime error (missing file, decompression-bomb cap tripped, unsafe path rejected, I/O error). |

For a batch, the process exit code aggregates every job's outcome with
`2 > 3 > 4` precedence when results differ across archives (a password
failure is reported over a checksum failure, which is reported over any
other error).

### Shell completion

Completion scripts for bash, zsh, and fish ship in [`completions/`](completions/)
(and inside each release archive):

```sh
# bash — source it, or drop it in your bash-completion.d
source completions/macrarcli.bash

# zsh — place on your $fpath as _macrarcli, then `autoload -U compinit && compinit`
cp completions/macrarcli.zsh ~/.zsh/completions/_macrarcli

# fish
cp completions/macrarcli.fish ~/.config/fish/completions/macrarcli.fish
```

### Security

**Paths and metadata:** extraction rejects paths that escape the root, absolute
and drive-relative names (including `a:b`), Windows device/stream names, and
other unsafe Windows components on every platform. Archive symlink/device
entries become inert regular-file content; group/other-write and special mode
bits are removed. After the destination is opened, staging, publication,
rollback, and cleanup use root-relative operations. File metadata is applied
through open handles so a path swap cannot redirect it outside the root.

**Resource limits:** `--max-size` and `--max-entries` default to `20G` and
`200000` respectively (`0` disables either cap). The entry cap applies to all modes; the byte cap applies to extraction and testing. `--max-dict` separately defaults
to 256 MiB per decoder. Concurrent jobs have separate decoders; dictionaries,
other decoder allocations, entry metadata, and runtime overhead all contribute
to memory use. These controls are not an RSS cap or a CPU/time budget.

**Collisions:** exact duplicate names use cached `" (n)"` suffixes. Different
names that collide after sanitization are rejected. Exclusive staging creation
also rejects filesystem aliases (such as case or Unicode normalization aliases)
rather than silently truncating an earlier entry.

**Publication and recovery:** on Unix, a descriptor-relative hard link publishes a completed staged file
without copying on supported filesystems. On other platforms or when hard links are unavailable, an exclusive-create copy
claims the output path without replacing an existing file; that copy can be
visible while incomplete. `--overwrite` first moves the existing file or symlink
itself into a private backup directory, then publishes the replacement. There
can be a visibility gap between those steps. A backup rename across a mount
boundary can fail safely before replacing that entry.

Commit failures attempt to remove this run's published files and restore
originals. Recovery errors are reported; original backups and unexpected replacement objects
are retained in `.macrarcli-backup-*` or `.macrarcli-cleanup-*`. Preserve reported
recovery directories for manual inspection. Successful
publication followed by backup-cleanup failure also returns an error even though
outputs are committed. Process crashes, concurrent external changes, and I/O
failures can leave partial results, backups, or staging data. The destination
itself may have been created even when decoding fails. No crash durability,
transaction isolation from other processes, or exact restoration of directory
metadata is promised.

**Human output:** archive-controlled names and diagnostic text have control
characters replaced with `?`. JSON retains original strings through the JSON
encoder; downstream consumers must render them appropriately.

## Scope & limitations

Lists, extracts (preserving structure or flat), and validates one or more
RAR archives, with multi-volume and password support, JSON output
(`--json`), and per-entry overwrite policies (`--overwrite`/`--skip`/
`--rename`). No ZIP output — this is a direct-extraction tool. Not yet
supported:

- stdin/stdout streaming.
- Crash-atomic or crash-durable extraction.

Integrity testing reads each file fully and relies on checksums supported by
the pinned `rardecode/v2` v2.2.5 decoder. Its RAR file checks use CRC32 when
present; RAR5 BLAKE2sp hash records are not implemented. A successful read is
not a guarantee that every entry had a supported content checksum.

### Filename encoding (legacy / CJK charsets)

There is **no filename-encoding override**. The pure-Go decoder
(`nwaples/rardecode`) returns each entry name already decoded to a Go string
and discards the archive's raw name bytes and stored codepage, so
`macrarcli` cannot re-interpret a legacy-charset name (Shift-JIS, GBK,
Big5, …) after the fact — the original bytes are gone before `macrarcli`
ever sees the entry. Names that the archive recorded in a non-Unicode
codepage may therefore appear garbled.

Re-decoding the already-decoded string would be both lossy and unsafe (a
re-interpreted byte could resurface a `/` or `..` *after* path sanitization
ran), so it is deliberately not attempted. If you need correct legacy-charset
names, extract with a tool that accepts a source-codepage flag (e.g.
`unrar x -cp936 …`).

RAR is a proprietary format; this tool only reads it, and only extracts real
files — it never writes another archive format.

## Development

```sh
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -w .
```

Tests include a fixture-agnostic fidelity check: any `.rar` placed in `testdata/`
is extracted and verified entry-by-entry (names + content hashes) against the source.
