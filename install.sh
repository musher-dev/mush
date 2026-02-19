#!/bin/sh
# Install script for Mush CLI
# Usage: curl -fsSL https://get.musher.dev | sh
#
# Options:
#   --version VERSION   Install a specific version (default: latest)
#   --prefix DIR        Install to DIR/bin (default: ~/.local)
#   --install-tmux      Install tmux if missing (opt-in)
#   --yes, -y           Skip confirmation prompts
#   --help, -h          Show this help message

set -eu

REPO="musher-dev/mush"
BINARY="mush"
DEFAULT_PREFIX="$HOME/.local"
BASE_URL="${MUSH_INSTALL_BASE_URL:-https://github.com/${REPO}}"

# ── Helpers ──────────────────────────────────────────────────────────────────

say() {
  printf '%s\n' "$@"
}

err() {
  say "Error: $*" >&2
  exit 1
}

bold() {
  if [ -t 1 ]; then
    printf '\033[1m%s\033[0m\n' "$*"
  else
    say "$*"
  fi
}

green() {
  if [ -t 1 ]; then
    printf '\033[32m%s\033[0m\n' "$*"
  else
    say "$*"
  fi
}

yellow() {
  if [ -t 1 ]; then
    printf '\033[33m%s\033[0m\n' "$*"
  else
    say "$*"
  fi
}

# ── Argument parsing ─────────────────────────────────────────────────────────

VERSION=""
PREFIX=""
YES=false
INSTALL_TMUX=false

usage() {
  cat <<EOF
Install Mush CLI

Usage:
  install.sh [options]

Options:
  --version VERSION   Install a specific version (default: latest)
  --prefix DIR        Install to DIR/bin (default: ~/.local)
  --install-tmux      Install tmux if missing (opt-in)
  --yes, -y           Skip confirmation prompts
  --help, -h          Show this help message

Examples:
  # Install latest version
  curl -fsSL https://get.musher.dev | sh

  # Install specific version
  curl -fsSL https://get.musher.dev | sh -s -- --version 2.0.0

  # Install to custom location
  curl -fsSL https://get.musher.dev | sh -s -- --prefix /usr/local

  # Install latest and also install tmux if missing
  curl -fsSL https://get.musher.dev | sh -s -- --install-tmux
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      [ $# -ge 2 ] || err "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --prefix)
      [ $# -ge 2 ] || err "--prefix requires a value"
      PREFIX="$2"
      shift 2
      ;;
    --yes | -y)
      YES=true
      shift
      ;;
    --install-tmux)
      INSTALL_TMUX=true
      shift
      ;;
    --help | -h)
      usage
      exit 0
      ;;
    *)
      err "Unknown option: $1"
      ;;
  esac
done

# ── Platform detection ───────────────────────────────────────────────────────

detect_os() {
  os="$(uname -s)"
  case "$os" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    *) err "Unsupported operating system: $os" ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64 | amd64) echo "amd64" ;;
    aarch64 | arm64) echo "arm64" ;;
    *) err "Unsupported architecture: $arch" ;;
  esac
}

# ── Download helpers ─────────────────────────────────────────────────────────

has_curl() {
  command -v curl >/dev/null 2>&1
}

has_wget() {
  command -v wget >/dev/null 2>&1
}

download() {
  url="$1"
  output="$2"

  if has_curl; then
    if [ "${MUSH_INSTALL_INSECURE:-}" = "1" ]; then
      curl -fsSL -o "$output" "$url"
    else
      curl --proto '=https' --tlsv1.2 -fsSL -o "$output" "$url"
    fi
  elif has_wget; then
    if [ "${MUSH_INSTALL_INSECURE:-}" = "1" ]; then
      wget -q -O "$output" "$url"
    else
      wget --https-only -q -O "$output" "$url"
    fi
  else
    err "Neither curl nor wget found. Please install one and try again."
  fi
}

# ── Version resolution ───────────────────────────────────────────────────────

resolve_latest_version() {
  # GitHub redirects /releases/latest to /releases/tag/vX.Y.Z
  if has_curl; then
    if [ "${MUSH_INSTALL_INSECURE:-}" = "1" ]; then
      url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
        "${BASE_URL}/releases/latest" 2>/dev/null) ||
        err "Failed to resolve latest version. Check ${BASE_URL}/releases"
    else
      url=$(curl --proto '=https' --tlsv1.2 -fsSLI -o /dev/null -w '%{url_effective}' \
        "${BASE_URL}/releases/latest" 2>/dev/null) ||
        err "Failed to resolve latest version. Check ${BASE_URL}/releases"
    fi
  elif has_wget; then
    if [ "${MUSH_INSTALL_INSECURE:-}" = "1" ]; then
      url=$(wget --max-redirect=0 -S \
        "${BASE_URL}/releases/latest" 2>&1 |
        sed -n 's/.*Location: *//p' | tr -d '\r') || true
    else
      # wget doesn't have a clean redirect-follow option; parse Location header
      url=$(wget --https-only --max-redirect=0 -S \
        "${BASE_URL}/releases/latest" 2>&1 |
        sed -n 's/.*Location: *//p' | tr -d '\r') || true
    fi
    [ -n "$url" ] || err "Failed to resolve latest version."
  else
    err "Neither curl nor wget found."
  fi

  # Extract tag from URL: https://github.com/owner/repo/releases/tag/v1.2.3
  tag="${url##*/}"
  [ -n "$tag" ] || err "Could not parse version from redirect URL: $url"
  echo "$tag"
}

# ── Checksum verification ───────────────────────────────────────────────────

verify_checksum() {
  file="$1"
  checksums_file="$2"
  archive_name="$3"

  expected=$(awk -v name="$archive_name" '$2 == name { print $1; exit }' "$checksums_file")
  [ -n "$expected" ] || err "Archive '$archive_name' not found in checksums file"

  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$file" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$file" | awk '{print $1}')
  else
    err "Neither sha256sum nor shasum found. Cannot verify checksum."
  fi

  if [ "$actual" != "$expected" ]; then
    err "Checksum mismatch!
  Expected: $expected
  Actual:   $actual
This could indicate a corrupted download or a tampered file."
  fi
}

# ── Sudo helpers ─────────────────────────────────────────────────────────────

maybe_sudo() {
  target="$1"

  # Find the nearest existing directory up the path hierarchy
  dir="$target"
  while [ ! -d "$dir" ]; do
    parent="$(dirname "$dir")"
    # Stop if we reached the root (dirname is unchanged)
    if [ "$parent" = "$dir" ]; then
      break
    fi
    dir="$parent"
  done

  if [ -w "$dir" ]; then
    # Nearest existing ancestor is writable — no sudo needed
    return
  fi

  if command -v sudo >/dev/null 2>&1; then
    echo "sudo"
  else
    err "Cannot write to $target and sudo is not available. Try: --prefix ~/.local"
  fi
}

# ── tmux helpers ─────────────────────────────────────────────────────────────

has_tmux() {
  command -v tmux >/dev/null 2>&1
}

detect_pkg_manager() {
  if command -v brew >/dev/null 2>&1; then
    echo "brew"
    return
  fi
  if command -v apt-get >/dev/null 2>&1; then
    echo "apt-get"
    return
  fi
  if command -v dnf >/dev/null 2>&1; then
    echo "dnf"
    return
  fi
  if command -v yum >/dev/null 2>&1; then
    echo "yum"
    return
  fi
  if command -v pacman >/dev/null 2>&1; then
    echo "pacman"
    return
  fi
  if command -v zypper >/dev/null 2>&1; then
    echo "zypper"
    return
  fi
  if command -v apk >/dev/null 2>&1; then
    echo "apk"
    return
  fi
}

package_install_sudo_prefix() {
  if [ "$(id -u)" -eq 0 ]; then
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    echo "sudo"
    return
  fi
  err "Installing tmux requires elevated privileges but sudo is not available"
}

install_tmux() {
  if has_tmux; then
    say "tmux is already installed."
    return
  fi

  pm="$(detect_pkg_manager)"
  [ -n "${pm:-}" ] || err "tmux requested but no supported package manager found (supported: brew, apt-get, dnf, yum, pacman, zypper, apk)"

  sudo_cmd="$(package_install_sudo_prefix)"
  say "Installing tmux using ${pm}..."

  case "$pm" in
    brew)
      # Homebrew installs for the current user and should not be run via sudo.
      brew install tmux
      ;;
    apt-get)
      ${sudo_cmd} apt-get update
      ${sudo_cmd} apt-get install -y tmux
      ;;
    dnf)
      ${sudo_cmd} dnf install -y tmux
      ;;
    yum)
      ${sudo_cmd} yum install -y tmux
      ;;
    pacman)
      ${sudo_cmd} pacman -Sy --noconfirm tmux
      ;;
    zypper)
      ${sudo_cmd} zypper --non-interactive install tmux
      ;;
    apk)
      ${sudo_cmd} apk add tmux
      ;;
    *)
      err "Unsupported package manager: ${pm}"
      ;;
  esac

  has_tmux || err "tmux installation reported success but tmux is still not found in PATH"
}

# ── PATH check ───────────────────────────────────────────────────────────────

check_path() {
  bin_dir="$1"

  case ":${PATH}:" in
    *":${bin_dir}:"*) return 0 ;;
  esac

  yellow "Warning: $bin_dir is not in your PATH."
  say ""
  say "Add it to your shell profile:"

  shell_name="$(basename "${SHELL:-/bin/sh}")"
  case "$shell_name" in
    bash)
      say "  echo 'export PATH=\"$bin_dir:\$PATH\"' >> ~/.bashrc"
      say "  source ~/.bashrc"
      ;;
    zsh)
      say "  echo 'export PATH=\"$bin_dir:\$PATH\"' >> ~/.zshrc"
      say "  source ~/.zshrc"
      ;;
    fish)
      say "  fish_add_path $bin_dir"
      ;;
    *)
      say "  export PATH=\"$bin_dir:\$PATH\""
      ;;
  esac
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
  OS="$(detect_os)"
  ARCH="$(detect_arch)"
  PREFIX="${PREFIX:-$DEFAULT_PREFIX}"
  BIN_DIR="${PREFIX}/bin"

  bold "Mush CLI Installer"
  say ""

  # Resolve version
  if [ -n "$VERSION" ]; then
    # Strip leading 'v' if provided, then re-add for tag
    VERSION="${VERSION#v}"
    TAG="v${VERSION}"
  else
    say "Resolving latest version..."
    TAG="$(resolve_latest_version)"
    VERSION="${TAG#v}"
  fi

  ARCHIVE_NAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
  ARCHIVE_URL="${BASE_URL}/releases/download/${TAG}/${ARCHIVE_NAME}"
  CHECKSUMS_URL="${BASE_URL}/releases/download/${TAG}/checksums.txt"

  say "  Version:  ${TAG}"
  say "  Platform: ${OS}/${ARCH}"
  say "  Target:   ${BIN_DIR}/${BINARY}"
  say ""

  # Confirm unless --yes
  if [ "$YES" = false ] && [ -t 0 ]; then
    printf "Proceed with installation? [Y/n] "
    read -r reply
    case "$reply" in
      [nN]*)
        say "Aborted."
        exit 0
        ;;
    esac
  fi

  # Create temp directory with cleanup trap
  TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t mush)"
  trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

  # Download archive and checksums
  say "Downloading ${ARCHIVE_NAME}..."
  download "$ARCHIVE_URL" "${TMP_DIR}/${ARCHIVE_NAME}"

  say "Downloading checksums..."
  download "$CHECKSUMS_URL" "${TMP_DIR}/checksums.txt"

  # Verify checksum
  say "Verifying checksum..."
  verify_checksum "${TMP_DIR}/${ARCHIVE_NAME}" "${TMP_DIR}/checksums.txt" "$ARCHIVE_NAME"

  # Extract
  say "Extracting..."
  tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "$TMP_DIR"

  # Install
  SUDO="$(maybe_sudo "$BIN_DIR")"
  mkdir -p "$BIN_DIR" 2>/dev/null || ${SUDO} mkdir -p "$BIN_DIR"
  ${SUDO} install -m 755 "${TMP_DIR}/${BINARY}" "${BIN_DIR}/${BINARY}"

  if [ "$INSTALL_TMUX" = true ]; then
    install_tmux
  fi

  say ""
  green "Successfully installed mush ${TAG} to ${BIN_DIR}/${BINARY}"
  say ""

  # Check PATH
  check_path "$BIN_DIR"

  say "Get started:"
  say "  mush --help"
  say "  mush init"
}

if [ "${INSTALL_SH_TESTING:-}" != "1" ]; then
  main
fi
