#!/bin/sh
# Pandaflix installer.
#
# Downloads the latest release binary for this platform and, on Alpine/musl
# systems, installs the runtime dependencies with apk. POSIX sh (BusyBox ash)
# compatible so it works in iSH and minimal containers.
#
# One-liner:
#   wget -qO- https://raw.githubusercontent.com/MADPANDA3D/pandaflix/master/scripts/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/MADPANDA3D/pandaflix/master/scripts/install.sh | sh
set -eu

REPO="MADPANDA3D/pandaflix"
BIN="pandaflix"

say() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# --- platform -----------------------------------------------------------------
os=$(uname -s)
case "$os" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) die "unsupported OS: $os (builds available for Linux and macOS)" ;;
esac

arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	i386 | i486 | i586 | i686) arch=386 ;;
	*) die "unsupported architecture: $arch" ;;
esac

asset="${BIN}_${os}_${arch}"

# --- runtime dependencies (Alpine/apk only) -------------------------------------
if command -v apk >/dev/null 2>&1; then
	say "Installing runtime dependencies via apk (mpv, ffmpeg, fzf, chafa)..."
	if [ "$(id -u)" = "0" ]; then
		apk add --no-cache mpv ffmpeg fzf || warn "some packages failed to install; continuing"
		apk add --no-cache chafa 2>/dev/null || warn "chafa unavailable; poster previews stay disabled"
	elif command -v sudo >/dev/null 2>&1; then
		sudo apk add --no-cache mpv ffmpeg fzf || warn "some packages failed to install; continuing"
		sudo apk add --no-cache chafa 2>/dev/null || warn "chafa unavailable; poster previews stay disabled"
	else
		warn "not root and no sudo; install mpv, ffmpeg, fzf manually"
	fi
fi

# --- install location -----------------------------------------------------------
dir=${PANDAFLIX_INSTALL_DIR:-}
if [ -z "$dir" ]; then
	if [ "$(id -u)" = "0" ]; then
		dir=/usr/local/bin
	else
		dir="$HOME/.local/bin"
	fi
fi
mkdir -p "$dir" || die "cannot create $dir"

# --- download -------------------------------------------------------------------
url="https://github.com/${REPO}/releases/latest/download/${asset}"
tmp="${TMPDIR:-/tmp}/.${BIN}.download.$$"
trap 'rm -f "$tmp"' EXIT INT TERM

say "Downloading $asset ..."
if command -v curl >/dev/null 2>&1; then
	if [ -t 2 ]; then
		curl -fL --retry 3 -o "$tmp" "$url" || die "download failed: $url"
	else
		curl -fsSL --retry 3 -o "$tmp" "$url" || die "download failed: $url"
	fi
elif command -v wget >/dev/null 2>&1; then
	wget -O "$tmp" "$url" || die "download failed: $url"
else
	die "need curl or wget"
fi

chmod +x "$tmp"
mv "$tmp" "$dir/$BIN"

say "Installed $dir/$BIN"
"$dir/$BIN" --version 2>/dev/null || true

case ":$PATH:" in
	*":$dir:"*) ;;
	*) say "Add to PATH: export PATH=\"$dir:\$PATH\"" ;;
esac

say ""
say "Run it:  $BIN \"movie name\""
say "Note: playback needs mpv; on iSH (32-bit emulation) Go binaries may be unstable."