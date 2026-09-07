#!/usr/bin/env sh
# install.sh — download the correct macrarcli release asset and install it.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/ongtungduong/macrarcli/main/scripts/install.sh | sh
#   VERSION=v0.2.0 sh install.sh          # pin a version
#   INSTALL_DIR=/usr/local/bin sh install.sh
#
# Verification:
#   The download is checksum-verified against the release's checksums.txt. A
#   missing sha256sum/shasum is FATAL (no silent unverified install). If cosign
#   is installed, the cosign signature over checksums.txt is also verified
#   (authenticity); without cosign that step is skipped (integrity still holds).
#   SKIP_CHECKSUM=1 disables integrity verification entirely — NEVER use it with
#   a piped (curl | sh) install.

set -eu

REPO="ongtungduong/macrarcli"
INSTALL_DIR="${INSTALL_DIR:-}"
VERSION="${VERSION:-}"
SKIP_CHECKSUM="${SKIP_CHECKSUM:-}"

# ── helpers ──────────────────────────────────────────────────────────────────

info()  { printf '\033[1;32m==> %s\033[0m\n' "$*"; }
warn()  { printf '\033[1;33mwarn: %s\033[0m\n' "$*" >&2; }
fatal() { printf '\033[1;31merror: %s\033[0m\n' "$*" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || fatal "required tool not found: $1"
}

# ── detect OS / arch ─────────────────────────────────────────────────────────

detect_os() {
  case "$(uname -s)" in
    Linux)  echo linux ;;
    Darwin) echo darwin ;;
    *)      fatal "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    arm64|aarch64) echo arm64 ;;
    *) fatal "unsupported arch: $(uname -m)" ;;
  esac
}

# ── resolve install directory ─────────────────────────────────────────────────

resolve_install_dir() {
  if [ -n "$INSTALL_DIR" ]; then
    echo "$INSTALL_DIR"
    return
  fi
  # Prefer ~/.local/bin (no sudo); fall back to /usr/local/bin.
  local_bin="$HOME/.local/bin"
  if [ -d "$local_bin" ] && echo "$PATH" | grep -q "$local_bin"; then
    echo "$local_bin"
  elif [ -w /usr/local/bin ]; then
    echo /usr/local/bin
  else
    # Create ~/.local/bin and remind user to add it to PATH.
    mkdir -p "$local_bin"
    warn "installed to $local_bin — make sure it is on your PATH"
    echo "$local_bin"
  fi
}

# ── fetch latest version tag ──────────────────────────────────────────────────

latest_version() {
  need curl
  url="https://api.github.com/repos/${REPO}/releases/latest"
  curl -fsSL "$url" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
}

# ── verify download (integrity + best-effort authenticity) ────────────────────

# verify_download enforces the release checksum over the downloaded tarball and,
# when cosign is present, the keyless signature over checksums.txt. A missing
# hash tool is fatal unless SKIP_CHECKSUM=1 (explicit, unsafe opt-out).
verify_download() {
  if [ "$SKIP_CHECKSUM" = "1" ]; then
    warn "SKIP_CHECKSUM=1 — integrity verification DISABLED (never use this with a piped install)"
    return
  fi

  if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
    fatal "no sha256sum/shasum found to verify the download; install one, or re-run with SKIP_CHECKSUM=1 to bypass (unsafe)"
  fi

  info "Verifying checksum..."
  curl -fsSL "${RELEASE_URL}/checksums.txt" -o "${WORK_DIR}/checksums.txt"

  # Extract OUR tarball's line by exact second-field (filename) match. Using awk
  # (not `grep | sha256sum -c -`) is deliberate: a pipe takes sha256sum's exit
  # status, and `sha256sum -c` on EMPTY stdin exits 0 — so a missing/renamed
  # entry or a checksums.txt that is actually an HTML error page would silently
  # "pass". An exact field match that errors on no-match closes that hole and the
  # substring/regex fragility of grep at once.
  if ! awk -v f="$TARBALL" '$2 == f { print; found = 1 } END { exit !found }' \
      "${WORK_DIR}/checksums.txt" > "${WORK_DIR}/expected.sha256"; then
    fatal "no checksum entry for ${TARBALL} in checksums.txt (asset name drift or a bad download)"
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$WORK_DIR" && sha256sum -c expected.sha256)
  else
    (cd "$WORK_DIR" && shasum -a 256 -c expected.sha256)
  fi

  # Authenticity is best-effort: only when cosign is available. checksums.txt is
  # signed keylessly in CI, so the certificate identity is the release workflow.
  if command -v cosign >/dev/null 2>&1; then
    info "Verifying signature (cosign)..."
    curl -fsSL "${RELEASE_URL}/checksums.txt.sig" -o "${WORK_DIR}/checksums.txt.sig"
    curl -fsSL "${RELEASE_URL}/checksums.txt.pem" -o "${WORK_DIR}/checksums.txt.pem"
    cosign verify-blob \
      --certificate "${WORK_DIR}/checksums.txt.pem" \
      --signature "${WORK_DIR}/checksums.txt.sig" \
      --certificate-identity-regexp "^https://github.com/${REPO}/.github/workflows/.+" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      "${WORK_DIR}/checksums.txt" \
      || fatal "cosign signature verification failed — refusing to install"
  else
    info "cosign not found — skipping signature check (checksum integrity already verified)"
  fi
}

# ── download + verify + install ───────────────────────────────────────────────

main() {
  need curl
  need tar

  OS="$(detect_os)"
  ARCH="$(detect_arch)"

  if [ -z "$VERSION" ]; then
    info "Fetching latest release..."
    VERSION="$(latest_version)"
    # latest_version pipes curl into grep|sed; a failed API call yields an empty
    # string (the pipe's exit status is sed's), so guard it explicitly.
    [ -n "$VERSION" ] || fatal "could not resolve the latest release version (GitHub API unreachable or rate-limited); set VERSION=vX.Y.Z"
  fi

  info "Installing macrarcli ${VERSION} (${OS}/${ARCH})"

  BASE="macrarcli_${OS}_${ARCH}"
  TARBALL="${BASE}.tar.gz"
  RELEASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

  WORK_DIR="$(mktemp -d)"
  trap 'rm -rf "$WORK_DIR"' EXIT

  info "Downloading ${TARBALL}..."
  curl -fsSL "${RELEASE_URL}/${TARBALL}" -o "${WORK_DIR}/${TARBALL}"

  verify_download

  info "Extracting..."
  tar -xzf "${WORK_DIR}/${TARBALL}" -C "$WORK_DIR"

  DEST_DIR="$(resolve_install_dir)"
  mkdir -p "$DEST_DIR"

  info "Installing to ${DEST_DIR}/macrarcli"
  cp "${WORK_DIR}/macrarcli" "${DEST_DIR}/macrarcli"
  chmod +x "${DEST_DIR}/macrarcli"

  info "Done! Run: macrarcli --version"
}

main
