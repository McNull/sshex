#!/bin/sh
set -eu

REPO="McNull/sshex"
INSTALL_DIR="${SSHEX_INSTALL_DIR:-/usr/local/bin}"

usage() {
	cat <<EOF
Usage: install.sh [VERSION]

Downloads and installs the sshex binary from GitHub releases.

Arguments:
  VERSION              Release tag to install (e.g. v0.3.1). Defaults to latest.

Environment:
  SSHEX_INSTALL_DIR    Directory to install into. Defaults to /usr/local/bin.
  SSHEX_VERSION        Same as the VERSION argument.

Examples:
  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh
  SSHEX_INSTALL_DIR="\$HOME/.local/bin" ./install.sh v0.3.1
EOF
}

case "${1:-}" in
-h | --help)
	usage
	exit 0
	;;
esac

version="${1:-${SSHEX_VERSION:-}}"

if [ -n "$version" ]; then
	base_url="https://github.com/${REPO}/releases/download/${version}"
else
	base_url="https://github.com/${REPO}/releases/latest/download"
fi

# --- platform detection ---------------------------------------------------

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
x86_64 | amd64) arch="amd64" ;;
aarch64 | arm64) arch="arm64" ;;
*)
	echo "sshex: unsupported architecture: $(uname -m)" >&2
	exit 1
	;;
esac

asset="sshex-${os}-${arch}.zip"
url="${base_url}/${asset}"

# --- dependencies ---------------------------------------------------------

if command -v curl >/dev/null 2>&1; then
	download() { curl -fL --progress-bar -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -q --show-progress -O "$2" "$1"; }
else
	echo "sshex: need either curl or wget to download releases" >&2
	exit 1
fi

if ! command -v unzip >/dev/null 2>&1; then
	echo "sshex: need 'unzip' to extract the release archive" >&2
	exit 1
fi

# --- download -------------------------------------------------------------

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading ${asset} ..."
if ! download "$url" "$tmp/$asset"; then
	echo "sshex: no release available for ${os}/${arch} yet" >&2
	echo "sshex: tried ${url}" >&2
	exit 1
fi

if download "${base_url}/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
	expected="$(awk -v a="$asset" '$2 == a { print $1 }' "$tmp/checksums.txt")"
	if [ -z "$expected" ]; then
		echo "sshex: checksum for ${asset} missing from checksums.txt" >&2
		exit 1
	fi
	actual="$(sha256sum "$tmp/$asset" | awk '{ print $1 }')"
	if [ "$actual" != "$expected" ]; then
		echo "sshex: checksum mismatch for ${asset}" >&2
		echo "  expected: ${expected}" >&2
		echo "  actual:   ${actual}" >&2
		exit 1
	fi
	echo "Checksum verified."
else
	echo "sshex: warning: could not download checksums.txt, skipping verification" >&2
fi

# --- extract and install --------------------------------------------------

unzip -q "$tmp/$asset" -d "$tmp"

if [ ! -f "$tmp/sshex" ]; then
	echo "sshex: ${asset} did not contain an sshex binary" >&2
	exit 1
fi

echo "Installing to ${INSTALL_DIR}/sshex ..."
if [ ! -d "$INSTALL_DIR" ]; then
	if [ -w "$(dirname "$INSTALL_DIR")" ] || [ "$(id -u)" -eq 0 ]; then
		mkdir -p "$INSTALL_DIR"
	else
		sudo mkdir -p "$INSTALL_DIR"
	fi
fi

if [ -w "$INSTALL_DIR" ] || [ "$(id -u)" -eq 0 ]; then
	install -m 0755 "$tmp/sshex" "$INSTALL_DIR/sshex"
elif command -v sudo >/dev/null 2>&1; then
	sudo install -m 0755 "$tmp/sshex" "$INSTALL_DIR/sshex"
else
	echo "sshex: ${INSTALL_DIR} is not writable and sudo is unavailable" >&2
	exit 1
fi

echo "Installed $("$INSTALL_DIR/sshex" --version 2>/dev/null || echo sshex)."

case ":${PATH}:" in
*":${INSTALL_DIR}:"*) ;;
*) echo "Note: ${INSTALL_DIR} is not on your PATH." ;;
esac
