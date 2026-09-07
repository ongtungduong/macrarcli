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

If an archive's contents are encrypted and no `--password` is given,
`macrarcli` prompts for one (masked, no echo) when run interactively; in a
non-interactive context (CI, a pipe) it fails fast instead of hanging. The
password is resolved once per invocation and reused across every archive in a
batch — never re-prompted per archive.

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
| `--password <pw>` | Password for encrypted archives. If omitted and the archive needs one, prompts interactively (see above). |
| `--jobs <n>` | Process up to `n` archives concurrently. Default: `min(NumCPU, 4)`. Per-job results are still printed in input order. |
| `--json` | Emit a machine-readable JSON summary on stdout (suppresses the human progress output). Includes a `mode` field (`extract`/`list`/`test`) and per-job `skippedEntries`. |
| `--max-size <n>` | Cap the total uncompressed size an archive may expand to (decompression-bomb defense). Accepts a plain byte count or a `K`/`M`/`G` suffix. `0` (default) = unlimited. Tripping the cap leaves no output and exits non-zero. Also enforced by `-t`. |
| `--max-entries <n>` | Cap the number of entries an archive may contain. `0` (default) = unlimited. Also bounds `-l` and `-t`. |
| `--version` | Print version and exit. |
| `-h`, `--help` | Print usage and exit. |

Per-entry **modification time** and **Unix permissions** are preserved.
Extraction is atomic per archive: entries are staged in a private temporary
directory first and committed into the destination only once the whole
archive has decoded successfully — a failure removes the staging directory
and leaves the destination exactly as it was before the run. Progress is
written to stderr so stdout stays clean for piping.

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

Untrusted-archive hardening is in place: entry names are sanitized against
**path traversal (Zip Slip)** and absolute paths, and symlink/device entries
are neutralized (stored as plain content) so they cannot escape the
extraction root.

**Decompression bombs:** extraction and `-t` both stream entries and can be
bounded with `--max-size`/`--max-entries`; exceeding either cap aborts with no
output. Duplicate or colliding entry names cannot silently overwrite one
another — a name that collides only after path sanitization is rejected, and
legitimate repeats are kept under a renamed entry.

**Overwrite safety:** `--overwrite` never writes through a symlink planted at
the destination path — the collision check uses `lstat`, never `stat`, so a
symlink is treated as occupied and removed (not followed) before the real
entry is written. A failure partway through an archive rolls back the entire
staging directory rather than leaving a partial extraction.

## Scope & limitations

Lists, extracts (preserving structure or flat), and validates one or more
RAR archives, with multi-volume and password support, JSON output
(`--json`), and per-entry overwrite policies (`--overwrite`/`--skip`/
`--rename`). No ZIP output — this is a direct-extraction tool. Not yet
supported (see `plans/` for the roadmap):

- stdin/stdout streaming, large-file throughput tuning.

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
