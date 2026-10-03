#!/bin/bash
# hc installer
# Usage: curl -sSf https://raw.githubusercontent.com/jmeiracorbal/hc/main/install.sh | bash
#
# Environment overrides:
#   HC_VERSION=v0.3.0 bash install.sh
#   HC_INSTALL_DIR=$HOME/bin bash install.sh
#   HC_DRY_RUN=true bash install.sh

set -e

REPO="jmeiracorbal/hc"
INSTALL_DIR="${HC_INSTALL_DIR:-$HOME/.local/bin}"
DRY_RUN="${HC_DRY_RUN:-false}"
HC_VERSION="${HC_VERSION:-}"

# ── helpers ────────────────────────────────────────────────────────────────────

info()  { printf "\033[1;34m[hc]\033[0m %s\n" "$*"; }
ok()    { printf "\033[1;32m[hc]\033[0m %s\n" "$*"; }
err()   { printf "\033[1;31m[hc]\033[0m %s\n" "$*" >&2; exit 1; }
warn()  { printf "\033[1;33m[hc]\033[0m %s\n" "$*"; }

dry() {
  if [ "$DRY_RUN" = "true" ]; then
    printf "\033[2m  (dry-run) %s\033[0m\n" "$*"
  else
    eval "$@"
  fi
}

# ── detect platform ────────────────────────────────────────────────────────────

detect_platform() {
  local os arch

  case "$(uname -s)" in
    Darwin) os="darwin" ;;
    Linux)  os="linux" ;;
    *)      err "Unsupported OS: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    arm64|aarch64) arch="arm64" ;;
    x86_64)        arch="amd64" ;;
    *)             err "Unsupported architecture: $(uname -m)" ;;
  esac

  echo "${os}-${arch}"
}

# ── fetch ──────────────────────────────────────────────────────────────────────

fetch() {
  local url="$1" dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -sSfL "$url" -o "$dest" 2>/dev/null
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url" 2>/dev/null
  else
    err "curl or wget required"
  fi
}

fetch_stdout() {
  local url="$1"
  if command -v curl >/dev/null 2>&1; then
    curl -sSfL "$url" 2>/dev/null
  else
    wget -qO- "$url" 2>/dev/null
  fi
}

probe_url() {
  local url="$1"
  if command -v curl >/dev/null 2>&1; then
    curl -sSfI "$url" >/dev/null 2>&1
  else
    wget -q --spider "$url" 2>/dev/null
  fi
}

check_version_compat() {
  local version="$1" platform="$2"
  local base_url="https://github.com/${REPO}/releases/download/${version}"

  info "Checking compatibility of pinned version ${version}..."

  if ! probe_url "${base_url}/hc-${platform}.sha256"; then
    err "Release ${version} does not ship a binary for ${platform}. Unset HC_VERSION to use the latest release."
  fi

  ok "Release ${version} is compatible."
}

fetch_latest_version() {
  local version
  version=$(fetch_stdout "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
  [ -z "$version" ] && err "Could not fetch latest release version"
  echo "$version"
}

download_binary() {
  local version="$1" platform="$2"
  local base_url="https://github.com/${REPO}/releases/download/${version}"
  local binary_url="${base_url}/hc-${platform}"
  local checksum_url="${base_url}/hc-${platform}.sha256"
  local dest="${INSTALL_DIR}/hc"

  info "Downloading hc ${version} for ${platform}..."

  if [ "$DRY_RUN" = "true" ]; then
    dry "curl -sSfL \"${binary_url}\" -o \"${dest}\""
    dry "curl -sSfL \"${checksum_url}\" | shasum -a 256 -c"
    dry "chmod +x \"${dest}\""
    return
  fi

  mkdir -p "$INSTALL_DIR"

  local tmp
  tmp=$(mktemp)
  trap 'rm -f "$tmp"' EXIT

  fetch "$binary_url" "$tmp" || err "Download failed: ${binary_url}"

  local checksum_file
  checksum_file=$(mktemp)
  trap 'rm -f "$tmp" "$checksum_file"' EXIT

  fetch_stdout "$checksum_url" > "$checksum_file" || err "Checksum download failed: ${checksum_url}"

  local expected_hash
  expected_hash=$(awk '{print $1}' "$checksum_file")
  local actual_hash
  actual_hash=$(shasum -a 256 "$tmp" | awk '{print $1}')

  if [ "$expected_hash" != "$actual_hash" ]; then
    err "Checksum mismatch — aborting. Expected: ${expected_hash}, got: ${actual_hash}"
  fi

  mv "$tmp" "$dest"
  chmod +x "$dest"
  ok "Binary installed: ${dest}"
}

check_path() {
  if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    warn "${INSTALL_DIR} is not in your PATH."
    warn "Add this to your shell profile (~/.zshrc or ~/.bashrc):"
    warn "  export PATH=\"\$HOME/.local/bin:\$PATH\""
  fi
}

main() {
  [ "$DRY_RUN" = "true" ] && info "Dry-run mode — no changes will be made"

  local platform version
  platform=$(detect_platform)
  version="${HC_VERSION:-$(fetch_latest_version)}"

  info "Latest release: ${version}"

  if [ -n "${HC_VERSION}" ]; then
    if [ "$DRY_RUN" = "true" ]; then
      info "Dry-run: would check compatibility of pinned version ${version}"
    else
      check_version_compat "$version" "$platform"
    fi
  fi

  download_binary "$version" "$platform"
  check_path

  if [ "$DRY_RUN" = "true" ]; then
    info "Dry-run: would run hc setup"
    ok "Done (dry-run)."
    return
  fi

  local hc_bin="${INSTALL_DIR}/hc"
  if ! [ -x "$hc_bin" ]; then
    hc_bin=$(command -v hc 2>/dev/null) || err "hc not found in ${INSTALL_DIR} or PATH"
    warn "Using hc from PATH: ${hc_bin} (expected: ${INSTALL_DIR}/hc)"
  fi

  info "Installing Claude Code hooks and awareness..."
  "$hc_bin" setup
  ok "Done. Shared DB created by setup. Run 'hc init' inside a project to enroll and index."
}

main "$@"
