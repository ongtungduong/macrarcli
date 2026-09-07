# Deployment & Release Guide

## Overview

macrarcli releases are built, signed, and distributed via:
- **GitHub Releases** — Direct binary downloads with checksums and keyless signatures
- **Homebrew** — macOS and Linux via `ongtungduong/homebrew-tap`
- **Scoop** — Windows via `ongtungduong/scoop-bucket` (experimental)
- **Install Script** — Universal one-liner: `curl | sh` with verification

All releases use **keyless code signing** (Sigstore/cosign via GitHub Actions OIDC).

## Building Locally

### From Source

```bash
# Clone the repo
git clone https://github.com/ongtungduong/macrarcli.git
cd macrarcli

# Build the binary
go build -o bin/macrarcli .

# Or use the Makefile
make build

# Verify
./bin/macrarcli --version
```

**Build output**: Single executable `macrarcli` (or `macrarcli.exe` on Windows)

**No dependencies**: Pure-Go binary with no runtime dependencies. Works on any machine with the target OS/arch.

### Cross-Compilation

```bash
# macOS arm64
GOOS=darwin GOARCH=arm64 go build -o bin/macrarcli-darwin-arm64 .

# Linux amd64
GOOS=linux GOARCH=amd64 go build -o bin/macrarcli-linux-amd64 .

# Windows amd64
GOOS=windows GOARCH=amd64 go build -o bin/macrarcli.exe .
```

## Release Process

### 1. Prepare Release

Update version and changelog:

```bash
# Edit docs/project-changelog.md: move "Unreleased" section to versioned section
# Commit the changelog update
git add docs/project-changelog.md
git commit -m "docs: prepare 0.3.1 release"

# Create annotated tag (pushes release)
git tag -a v0.3.1 -m "Release 0.3.1"
git push origin v0.3.1
```

**Tag format**: `v<major>.<minor>.<patch>` (semver)

**Changelog format**: See `docs/project-changelog.md` for section template.

### 2. GitHub Actions Release Workflow

Triggered automatically when a `v*` tag is pushed to `main`.

**Workflow file**: `.github/workflows/release.yml`

**Steps**:
1. Checkout code
2. Set up Go environment
3. Install cosign v2.6.3 (pinned — v3.x breaks legacy detached sign-blob)
4. Run goreleaser (see `.goreleaser.yaml`)
5. goreleaser builds, signs, publishes to GitHub + Homebrew + Scoop

**Key permissions**:
```yaml
permissions:
  contents: write        # Create GitHub Release
  id-token: write        # OIDC for cosign keyless signing
```

### 3. Goreleaser Configuration

**File**: `.goreleaser.yaml`

#### Build Matrix

```yaml
builds:
  - id: macrarcli
    main: .
    binary: macrarcli
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    env:
      - CGO_ENABLED=0
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.ShortCommit}}
```

**Output archives**:
- **Linux/macOS**: `.tar.gz` (e.g., `macrarcli_linux_amd64.tar.gz`)
- **Windows**: `.zip` (required by Scoop)

#### Checksums & Signing

```yaml
checksum:
  name_template: checksums.txt
  algorithm: sha256

signs:
  - cmd: cosign
    signature: "${artifact}.sig"
    certificate: "${artifact}.pem"
    args:
      - sign-blob
      - "--output-signature=${signature}"
      - "--output-certificate=${certificate}"
      - "${artifact}"
      - "--yes"
    artifacts: checksum
    output: true
```

**Process**:
1. Generate `checksums.txt` (SHA256 of all artifacts)
2. Sign `checksums.txt` with cosign (keyless, via GitHub OIDC)
3. Produce `checksums.txt.sig` (signature) + `checksums.txt.pem` (certificate chain)

**Security**: Users verify integrity (sha256) and authenticity (cosign signature).

#### Homebrew Tap

```yaml
brews:
  - name: macrarcli
    skip_upload: "{{ if eq .Env.HOMEBREW_TAP_TOKEN \"\" }}true{{ end }}"
    repository:
      owner: ongtungduong
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    homepage: "https://github.com/ongtungduong/macrarcli"
    description: "Extract RAR archives directly — pure-Go, no unrar required"
    license: "MIT"
    install: |
      bin.install "macrarcli"
    test: |
      system "#{bin}/macrarcli", "--version"
```

**Creates**: Formula in `ongtungduong/homebrew-tap` repo.

**User installation**:
```bash
brew install ongtungduong/tap/macrarcli
```

**Setup** (maintainer only): Store `HOMEBREW_TAP_TOKEN` in GitHub Actions secrets.

#### Scoop Bucket

```yaml
scoops:
  - name: macrarcli
    skip_upload: "{{ if eq .Env.SCOOP_TAP_TOKEN \"\" }}true{{ end }}"
    repository:
      owner: ongtungduong
      name: scoop-bucket
      token: "{{ .Env.SCOOP_TAP_TOKEN }}"
    homepage: "https://github.com/ongtungduong/macrarcli"
    description: "Extract RAR archives directly — pure-Go, no unrar required"
    license: MIT
```

**Creates**: Manifest in `ongtungduong/scoop-bucket` repo.

**User installation**:
```bash
scoop bucket add macrarcli https://github.com/ongtungduong/scoop-bucket
scoop install macrarcli
```

**Status**: Experimental (Windows CI is vet-only, not full test suite).

## Code Signing Details

### Keyless Signing (Sigstore/cosign)

macrarcli uses **keyless signing**:
- No stored private keys (eliminates key theft risk)
- Identity via GitHub Actions OIDC token (proves workflow ran in the real repo)
- Signature verifiable by anyone with public cosign/Sigstore tools

### Signing Flow

```
1. GitHub Actions starts release workflow
2. Actions OIDC provider issues identity token
3. cosign: "I am github.com/ongtungduong/macrarcli at commit XYZ"
4. Sigstore (public CA): Verifies OIDC, issues short-lived signing cert
5. cosign: Signs checksums.txt with certificate
6. Produces checksums.txt.sig + checksums.txt.pem
7. Both uploaded to GitHub Release
8. Users verify with: cosign verify-blob --cert checksums.txt.pem --signature checksums.txt.sig checksums.txt
```

### Verification (User Side)

**With cosign installed**:
```bash
# Verify integrity (checksums)
sha256sum -c checksums.txt

# Verify authenticity (signature)
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp "^https://github.com/ongtungduong/macrarcli/.github/workflows/.+" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  checksums.txt
```

**Without cosign**:
```bash
sha256sum -c checksums.txt  # Integrity still verified
# Authenticity skipped (signature unverified)
```

## One-Line Install Script

**File**: `scripts/install.sh`

**Usage**:
```bash
# Latest release
curl -fsSL https://raw.githubusercontent.com/ongtungduong/macrarcli/main/scripts/install.sh | sh

# Pin version
VERSION=v0.3.0 sh install.sh

# Custom install directory
INSTALL_DIR=/usr/local/bin sh install.sh
```

**What it does**:
1. Detects OS and architecture
2. Fetches latest release tag from GitHub API
3. Downloads `macrarcli_OS_ARCH.tar.gz`
4. Verifies SHA256 checksum (required; FATAL if missing)
5. Verifies cosign signature if cosign is installed (optional)
6. Extracts and installs to `~/.local/bin` or `/usr/local/bin`

**Security**:
- Checksum verification is REQUIRED (no silent unverified install)
- Signature verification is OPTIONAL (best-effort when cosign available)
- `SKIP_CHECKSUM=1` disables verification — only use if you understand the risk

## Release Checklist

Before pushing a release tag:

- [ ] All tests pass (`make test`)
- [ ] Code formatted (`make fmt`)
- [ ] Vet clean (`make vet`)
- [ ] Changelog updated in `docs/project-changelog.md`
- [ ] Version bumped in code (if not using `--ldflags` injection)
- [ ] Commit messages follow Conventional Commits
- [ ] Tag is annotated (`git tag -a vX.Y.Z`)
- [ ] Tag message is descriptive (not empty)

After push:

- [ ] GitHub Actions workflow runs and completes
- [ ] GitHub Release page populated with artifacts
- [ ] Homebrew formula updated and tested (`brew install ongtungduong/tap/macrarcli`)
- [ ] Scoop manifest published (experimental)
- [ ] Checksum verification works: `sha256sum -c checksums.txt`

## CI/CD Configuration

**Lint & Test** (`.github/workflows/ci.yml`):
- Runs on every PR and push to main
- Tests on macOS, Linux, Windows
- Platforms: amd64, arm64
- Windows: vet-only (experimental)

**Release** (`.github/workflows/release.yml`):
- Triggered on `v*` tag push
- Builds for all platforms
- Signs with cosign v2.6.3
- Publishes to Homebrew and Scoop

## Troubleshooting Releases

### Issue: goreleaser fails to publish to Homebrew

**Check**:
1. `HOMEBREW_TAP_TOKEN` is set in GitHub Actions secrets
2. Token has write access to `ongtungduong/homebrew-tap` repo
3. Repo exists and is accessible

**Workaround**: `skip_upload: true` in `.goreleaser.yaml` to skip Homebrew (publish manually if needed).

### Issue: cosign signature fails to verify

**Check**:
1. Use cosign v2.x (not v3.x) — v3 breaks legacy sign-blob
2. Certificate identity includes the release workflow: `^https://github.com/ongtungduong/macrarcli/.github/workflows/.+`
3. OIDC issuer is `https://token.actions.githubusercontent.com`

**Workaround**: Re-sign manually with cosign after goreleaser completes.

### Issue: Windows binary quarantined by Gatekeeper

**Solution**: Users remove quarantine after verifying signature/checksum:
```bash
xattr -d com.apple.quarantine ./macrarcli
```

Homebrew and the install script avoid this by not flagging downloads as web-downloaded.

## Maintenance Notes

- **cosign pinning**: v2.6.3 is locked because v3.x dropped legacy detached sign-blob. Keep pinned until we migrate to `.sig` + `.pem` separately.
- **goreleaser pinning**: v2.16.0 is stable; upgrade carefully (test all platforms before release).
- **Go version**: Update `go.mod` and CI matrix together; test all supported platforms.
