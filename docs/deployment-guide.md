# Deployment & Release Guide

## Overview

rar2zip releases are built, signed, and distributed via multiple channels:
- **GitHub Releases** — Direct binary downloads with checksums and signatures
- **Homebrew** — macOS and Linux package manager via `ongtungduong/homebrew-tap`
- **Scoop** — Windows package manager via `ongtungduong/scoop-bucket` (experimental)
- **Install Script** — Universal one-liner: `curl | sh`

All releases use **keyless code signing** (Sigstore/cosign) and require **GitHub Actions OIDC** identity.

## Release Process

### 1. Create a Release Tag

```bash
# Bump version in code (if not already done)
# Update docs/project-changelog.md "Unreleased" section

git tag -a v0.2.1 -m "Release 0.2.1"  # or v0.3.0, etc.
git push origin v0.2.1
```

**Tag Format**: `v<major>.<minor>.<patch>` (semver)

**Changelog**: Move "Unreleased" section to versioned section (see `docs/project-changelog.md` for format).

### 2. GitHub Actions Release Workflow

Triggered automatically when a `v*` tag is pushed.

**Workflow**: `.github/workflows/release.yml`

```yaml
name: Release
on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      id-token: write  # For OIDC cosign signing
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - uses: sigstore/cosign-installer@v4.1.2  # Pinned
        with:
          cosign-release: 'v2.6.3'  # PINNED to v2.x (v3 breaks legacy sign-blob)
      - uses: goreleaser/goreleaser-action@v7.2.2
        with:
          version: 'v2.16.0'  # Pinned
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}  # Optional
          SCOOP_TAP_TOKEN: ${{ secrets.SCOOP_TAP_TOKEN }}  # Optional
```

**What it does**:
1. Checks out code
2. Sets up Go
3. Installs cosign v2.6.3 (NOT v3.x)
4. Runs goreleaser with `.goreleaser.yaml` config
5. Goreleaser builds, signs, and publishes to GitHub + Homebrew + Scoop

### 3. Goreleaser Configuration

**File**: `.goreleaser.yaml`

#### Build Matrix

```yaml
builds:
  - id: rar2zip
    main: .
    binary: rar2zip
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    env:
      - CGO_ENABLED=0
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
```

**Outputs**:
- **Linux/macOS**: `.tar.gz` archives (e.g., `rar2zip_v0.2.1_linux_amd64.tar.gz`)
- **Windows**: `.zip` archives (required by Scoop, e.g., `rar2zip_v0.2.1_windows_amd64.zip`)

#### Checksums & Signing

```yaml
checksum:
  name_template: 'checksums.txt'
  algorithm: sha256

signs:
  - cmd: cosign
    args: ['sign-blob', '--key=cosignature', '${artifact}']
    artifacts: checksum
    output: true
```

**Effect**:
1. Generate `checksums.txt` (SHA256 of all artifacts)
2. Sign `checksums.txt` with cosign (keyless, via OIDC)
3. Produce `checksums.txt.sig` and `checksums.txt.pem` (detached signature + public key)

**Security**: Users verify integrity (sha256) and authenticity (cosign signature).

#### Homebrew Tap

```yaml
brews:
  - tap:
      owner: ongtungduong
      name: homebrew-tap
    commit_author:
      name: rar2zip Release Bot
      email: releases@example.com
    url_template: "https://github.com/ongtungduong/rar2zip/releases/download/{{ .Tag }}/{{ .ArtifactName }}"
    homepage: "https://github.com/ongtungduong/rar2zip"
    description: "Convert RAR archives to ZIP"
    license: MIT
    skip_upload: false  # Only upload if HOMEBREW_TAP_TOKEN set
```

**Creates/Updates**: `ongtungduong/homebrew-tap` with a `rar2zip.rb` formula.

**User Installation**: `brew install ongtungduong/tap/rar2zip`

#### Scoop Bucket

```yaml
scoop:
  - bucket:
      owner: ongtungduong
      name: scoop-bucket
    commit_author:
      name: rar2zip Release Bot
      email: releases@example.com
    url_template: "https://github.com/ongtungduong/rar2zip/releases/download/{{ .Tag }}/{{ .ArtifactName }}"
    homepage: "https://github.com/ongtungduong/rar2zip"
    description: "Convert RAR archives to ZIP"
    license: MIT
    skip_upload: false  # Only upload if SCOOP_TAP_TOKEN set
```

**Creates/Updates**: `ongtungduong/scoop-bucket` with a `rar2zip.json` manifest.

**User Installation**: `scoop bucket add rar2zip https://github.com/ongtungduong/scoop-bucket && scoop install rar2zip`

**Status**: Experimental (Windows CI is vet-only, not full test suite).

#### GitHub Release

```yaml
release:
  draft: false
  prerelease: false
  name_template: "Release {{.Version}}"
  body: |
    Automatic release.
    See docs/project-changelog.md for details.
```

**Creates**: GitHub Release page with all artifacts, checksums, signatures.

## Code Signing Details

### Keyless Signing (Sigstore/cosign)

rar2zip uses **keyless code signing**, which means:
- No stored private keys in GitHub (eliminates key theft risk)
- Identity via GitHub Actions OIDC token (tied to the repo and workflow)
- Signature verifiable by anyone with public cosign/Sigstore setup

### Signing Flow

```
1. GitHub Actions starts release workflow
2. Actions OIDC provider issues identity token (OIDC proof that this is the real repo)
3. cosign: "I am github.com/ongtungduong/rar2zip at commit XYZ"
4. Sigstore (public CA): Verifies OIDC token, issues short-lived signing certificate
5. cosign: Signs checksums.txt with certificate, produces .sig + .pem
6. .sig (signature) + .pem (cert chain) uploaded to GitHub Release
7. Users can verify with: cosign verify-blob --cert checksums.txt.pem --signature checksums.txt.sig checksums.txt
```

### Verification (User Side)

**With cosign installed**:
```bash
# Verify integrity (checksums)
sha256sum -c checksums.txt

# Verify authenticity (cosign signature) — optional, requires cosign
cosign verify-blob \
  --cert checksums.txt.pem \
  --signature checksums.txt.sig \
  checksums.txt
```

**Without cosign** (just checksum):
```bash
sha256sum -c checksums.txt  # Still required
```

## Installation Methods

### 1. Homebrew (macOS / Linux)

```bash
brew tap ongtungduong/tap
brew install rar2zip
```

**Prerequisites**: Homebrew installed.

**Automatic Updates**: `brew upgrade rar2zip`

### 2. One-Line Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/ongtungduong/rar2zip/main/scripts/install.sh | sh
```

**Script Location**: `scripts/install.sh`

**What it does**:
1. Detects OS (macOS, Linux, Windows via MSYS2/WSL)
2. Detects CPU (amd64, arm64, etc.)
3. Downloads latest release binary
4. Verifies SHA256 checksum (hard error if missing hash tool)
5. Optionally verifies cosign signature (if cosign installed)
6. Installs to `~/.local/bin` or `/usr/local/bin`

**Installation directory** (in order):
1. `$INSTALL_DIR` (if set)
2. `~/.local/bin` (if on `$PATH`)
3. `/usr/local/bin` (fallback)

**Script behavior**:

```bash
# Basic usage
curl -fsSL https://raw.githubusercontent.com/ongtungduong/rar2zip/main/scripts/install.sh | sh

# Custom install directory
INSTALL_DIR=/opt/bin curl -fsSL ... | sh

# Skip checksum verification (UNSAFE — only if you know what you're doing)
SKIP_CHECKSUM=1 curl -fsSL ... | sh  # NOT recommended

# Verify it worked
~/.local/bin/rar2zip --version
```

**Security notes**:
- Always verify checksums (they're mandatory by default)
- Only skip checksums if you're installing from a trusted local file (not piped from network)
- If cosign is installed, the script also verifies the signature

### 3. Scoop (Windows) — Experimental

```powershell
scoop bucket add rar2zip https://github.com/ongtungduong/scoop-bucket
scoop install rar2zip
```

**Prerequisites**: Scoop installed.

**Status**: Experimental (no full test suite on Windows; vet-only CI).

**Known Limitations**:
- Symlink/device tests skipped (Unix-only)
- Fallback tool tests skipped (unrar/7z may not be installed)
- Fallback functionality itself works (pure Go + shells out if available)

### 4. Manual Download

Download directly from [GitHub Releases](https://github.com/ongtungduong/rar2zip/releases):

1. Find the latest release
2. Download the `.tar.gz` (macOS/Linux) or `.zip` (Windows)
3. Extract: `tar xzf rar2zip_*.tar.gz` or `unzip rar2zip_*.zip`
4. Verify checksums: `sha256sum -c checksums.txt`
5. Optionally verify signature: `cosign verify-blob ...` (see above)
6. Place `rar2zip` on `$PATH` or use `./rar2zip` directly

### 5. Build from Source

```bash
git clone https://github.com/ongtungduong/rar2zip.git
cd rar2zip
make build  # or: go build -o bin/rar2zip .
./bin/rar2zip --version
```

**Prerequisites**: Go 1.26.2+ installed.

## CI/CD Matrix

### Build CI (`.github/workflows/ci.yml`)

Runs on every push and PR. Matrix:

| OS | Arch | Go | Test | Fallback Tests |
|---|---|---|---|---|
| Linux | amd64 | 1.26.2 | ✅ | ✅ |
| Linux | arm64 | 1.26.2 | ✅ | ✅ |
| macOS | amd64 | 1.26.2 | ✅ | ✅ |
| macOS | arm64 | 1.26.2 | ✅ | ✅ |
| Windows | amd64 | 1.26.2 | ⚠️ (vet only) | ❌ (Unix-only) |
| Windows | arm64 | 1.26.2 | ⚠️ (vet only) | ❌ (Unix-only) |

**Windows caveat**: Test suite is Unix-only (symlinks, tool fallback). Windows CI verifies code compiles and passes `go vet`; full test coverage requires Unix.

### Release CI (`.github/workflows/release.yml`)

Triggered on `v*` tags. Steps:

1. Build multi-platform binaries
2. Generate checksums (SHA256)
3. Sign with cosign (keyless, OIDC)
4. Upload to GitHub Releases
5. Publish to Homebrew tap (if `HOMEBREW_TAP_TOKEN` set)
6. Publish to Scoop bucket (if `SCOOP_TAP_TOKEN` set)

## Secrets & Tokens

### Required for Full Release

| Secret | Used For | Where to Set |
|---|---|---|
| `GITHUB_TOKEN` | Publish to GitHub Releases | Auto-provided by GitHub Actions |
| `id-token: write` | OIDC identity for cosign | `.github/workflows/release.yml` permissions |

### Optional for Homebrew/Scoop

| Secret | Used For | Setup |
|---|---|---|
| `HOMEBREW_TAP_TOKEN` | Publish to Homebrew tap | GitHub Personal Access Token with `repo` scope |
| `SCOOP_TAP_TOKEN` | Publish to Scoop bucket | GitHub Personal Access Token with `repo` scope |

**If tokens not set**: Homebrew/Scoop uploads are skipped (goreleaser `skip_upload: false` becomes a no-op).

**Creating tokens**:
1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Create token with `repo` scope (read/write access to public repos)
3. Add to repository Secrets (Settings → Secrets and variables → Actions → New repository secret)

## Rollback & Cleanup

### If a Release Fails

1. Delete the tag: `git tag -d v0.2.1 && git push origin :v0.2.1`
2. Fix the issue
3. Re-tag and push: `git tag -a v0.2.1 -m "..." && git push origin v0.2.1`

### If a Release is Bad (e.g., security issue)

1. Delete the GitHub Release (via UI or `gh release delete v0.2.1`)
2. Tag is still pushed (delete it if you want to re-use the version)
3. Update `docs/project-changelog.md` to note the issue

**Example**: A security fix in v0.2.1 → v0.2.2 released immediately after.

### Homebrew/Scoop Updates

- **Homebrew**: Automatically picks up new releases from the tap
- **Scoop**: Updates when users run `scoop update`

Manual cleanup (rarely needed):
- Homebrew: Maintainer of tap can remove old formulas
- Scoop: Maintainer of bucket can remove old manifests

## Version Management

### Semantic Versioning

- **MAJOR** (0): Major breaking changes (CLI flags, output format)
- **MINOR** (2): New features, backward-compatible
- **PATCH** (1): Bug fixes, performance improvements, security patches

Examples:
- `0.1.0` → `0.2.0` (added `--verify`, `--allow-fallback`, new flags)
- `0.2.0` → `0.2.1` (added `--skip-existing`, shell completions)

### Version String in Binaries

Set via ldflags at build time:

```go
// main.go
var version = "dev"
var commit = "unknown"

func main() {
    if *flagVersion {
        fmt.Printf("rar2zip %s (%s)\n", version, commit)
        os.Exit(0)
    }
}
```

Goreleaser fills in:
```bash
-X main.version={{.Version}}      # e.g., v0.2.1
-X main.commit={{.Commit}}        # e.g., a1b2c3d
```

**Result**: `rar2zip --version` outputs `rar2zip v0.2.1 (a1b2c3d)`

## Troubleshooting Deployments

### Release CI Stuck

Check `.github/workflows/release.yml` logs:
1. Actions tab → Latest run → Logs
2. Common issues:
   - `HOMEBREW_TAP_TOKEN` expired → renew token in Secrets
   - `goreleaser` syntax error in `.goreleaser.yaml` → lint locally with `goreleaser check`
   - cosign v3.x used (wrong; must be v2.6.3) → check workflow pinned version

### Homebrew Formula Stale

After a release, sometimes Homebrew caches old binaries. Users can force update:

```bash
brew tap ongtungduong/tap --force
brew install --force rar2zip
```

Or file an issue in `ongtungduong/homebrew-tap`.

### Install Script Fails

Common issues:

| Error | Fix |
|---|---|
| `sha256sum: command not found` | Install `coreutils` or use `shasum`. Or `SKIP_CHECKSUM=1` if from trusted source. |
| Permission denied writing to `/usr/local/bin` | Use `INSTALL_DIR=~/.local/bin` or use Homebrew/Scoop instead. |
| `curl: command not found` | Install curl or download manually from releases page. |
| Checksums don't match | Binary corrupted during download; retry or use different mirror. |

### Scoop Installation Fails

Windows-specific issues:

| Error | Fix |
|---|---|
| `bucket not found` | Run `scoop bucket add rar2zip https://github.com/ongtungduong/scoop-bucket` first. |
| `Hash doesn't match` | Scoop bucket is stale; wait for goreleaser to push, then `scoop update rar2zip`. |
| Cannot run binary | Binary quarantined; right-click → Properties → Unblock. Or use Homebrew (via WSL). |

## Post-Release Checklist

After publishing a release:

- [ ] GitHub Release created with correct tag and assets
- [ ] Checksums generated and signed
- [ ] Homebrew tap updated (if `HOMEBREW_TAP_TOKEN` set)
- [ ] Scoop bucket updated (if `SCOOP_TAP_TOKEN` set)
- [ ] `docs/project-changelog.md` version section finalized
- [ ] "Unreleased" section started for next version
- [ ] Announcement (issue, discussion, or social media) if major feature

## Related Documentation

- **Build**: `make build`, `make test`, `make vet` (see Makefile)
- **Contributing**: `CONTRIBUTING.md` (developer guide, security review)
- **Troubleshooting**: `docs/TROUBLESHOOTING.md` (user FAQ)
- **Changelog**: `docs/project-changelog.md` (version history)
- **Roadmap**: `docs/project-roadmap.md` (planned work)
