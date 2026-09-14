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

build_local=0
case "${1:-}" in
	--build) build_local=1 ;;
	-h | --help)
		printf 'usage: %s [--build]\n\n  (no args)  install runtime deps + latest release binary\n  --build    build this checkout instead of downloading\n' "$0"
		exit 0
		;;
	"") ;;
	*) printf 'error: unknown argument: %s\n' "$1" >&2; exit 2 ;;
esac

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

say "Installing $BIN for ${os}/${arch} ..."
if [ "$build_local" = "1" ]; then
	command -v go >/dev/null 2>&1 || die "--build requires Go (install it with: apk add go, or use the release binary)"
	say "Building from this checkout..."
	(
		cd "$(dirname "$0")/.."
		GOMAXPROCS=${GOMAXPROCS:-1} GODEBUG=${GODEBUG:-asyncpreemptoff=1} CGO_ENABLED=0 go build -trimpath -o "$tmp" .
	) || die "build failed"
elif command -v curl >/dev/null 2>&1; then
	say "Downloading $asset ..."
	if [ -t 2 ]; then
		curl -fL --retry 3 -o "$tmp" "$url" || die "download failed: $url"
	else
		curl -fsSL --retry 3 -o "$tmp" "$url" || die "download failed: $url"
	fi
elif command -v wget >/dev/null 2>&1; then
	say "Downloading $asset ..."
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