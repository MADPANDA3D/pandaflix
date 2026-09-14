<h1 align='center'>
    PANDAFLIX
</h1>

<br>

<h3 align='center'>
    Spiritual successor of flix-cli and mov-cli.
</h3>


<div align='center'>
<br>


![Language](https://img.shields.io/badge/-go-00ADD8.svg?style=for-the-badge&logo=go&logoColor=white)

<a href="http://makeapullrequest.com"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen.svg" alt="PRs Welcome"></a>

<img src="https://img.shields.io/badge/os-linux-brightgreen" alt="OS linux">
<img src="https://img.shields.io/badge/os-mac-brightgreen"alt="OS Mac">
<img src="https://img.shields.io/badge/os-windows-brightgreen" alt="OS Windows">
<img src="https://img.shields.io/badge/os-android-brightgreen" alt="OS Android">

<br>


<a href="https://discord.gg/X6FzCz5vR"><img src="https://invidget.switchblade.xyz/X6FzCz5vR"></a>

</div>
<br>

---

![](./.assets/showcase.gif)

---

## Overview

- [Installation](#installation)
  - [Arch Linux](#1-arch-linux)
  - [NixOS / Nix / MacOS](#2-nixos--nix--macos)
  - [Debian-based or Ubuntu-based](#2-debian-based-or-ubuntu-based)
  - [Fedora](#3-fedora)
  - [MacOS](#5-macos)
  - [Windows](#6-windows)
  - [Go Install](#7-go-install)
  - [Build from Source](#8-build-from-source)
  - [Android Installation](#9-android-installation)
- [Dependencies](#dependencies)
- [Usage](#usage)
  - [Flags](#flags)
  - [Playback Controls](#playback-controls)
  - [Examples](#examples)
- [Configuration](#configuration)
- [Hooks & MpvArgs](#hooks--mpvargs)
  - [MpvArgs](#mpvargs)
  - [Hooks](#hooks)
- [Providers](#providers)

 > [!NOTE]
 > Before creating an issue, make sure to update pandaflix

## Installation

### 1. Prebuilt binaries (recommended)

Download the binary for your platform from the [latest release](https://github.com/MADPANDA3D/pandaflix/releases/latest):

| Platform | File |
|----------|------|
| Linux x86_64 | `pandaflix_linux_amd64` |
| Linux arm64 | `pandaflix_linux_arm64` |
| macOS Intel | `pandaflix_darwin_amd64` |
| macOS Apple Silicon | `pandaflix_darwin_arm64` |
| Windows x86_64 | `pandaflix_windows_amd64.exe` |

Each file ships with a `.sha256` checksum. Make it executable and put it on your `PATH`:

```sh
chmod +x pandaflix_linux_amd64
mv pandaflix_linux_amd64 ~/.local/bin/pandaflix
```

### 2. Go install

```bash
go install github.com/MADPANDA3D/pandaflix@latest
```

### 3. Build from source

```bash
git clone https://github.com/MADPANDA3D/pandaflix.git
cd pandaflix
CGO_ENABLED=0 go build -trimpath -o pandaflix .
```

### Dependencies

- `mpv` (default player) or `vlc`
- `ffmpeg` (download remuxing)
- `fzf` (menus)
- `chafa` (optional — poster previews; install with `sudo pacman -S chafa`, `apt install chafa`, or `brew install chafa`)

> [!IMPORTANT]
> On Windows, poster previews need a graphics-capable terminal such as WezTerm.

### 9. Android Installation

Install termux [(Guide)](https://termux.com/)

```sh
pkg up -y
pkg in fzf python-yt-dlp
curl -sL "https://github.com/DemonKingSwarn/pandaflix/releases/download/v1.2.1/pandaflix-android-arm64" -o $PREFIX/bin/pandaflix
chmod +x $PREFIX/bin/pandaflix
```


# Dependencies

- [`mpv`](https://mpv.io) - Video Player for Linux and Windows
- [`vlc`](https://www.videolan.org/vlc/) - Alternate video player for Linux and Windows
- [`iina`](https://iina.io) - Video Player for MacOS
- [`mpv-android`](https://play.google.com/store/apps/details?id=is.xyz.mpv&hl=en-US&pli=1) - Video Player for Android
- [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) - Download manager
- [`fzf`](https://github.com/junegunn/fzf) - For selection menus
- [`chafa`](https://github.com/hpjansson/chafa) & [`libsixel`](https://github.com/saitoha/libsixel) - For poster previews (`--show-image`)

## Usage

```bash
pandaflix [query] [flags]
```

`[query]` is the title you want to search for (e.g., "breaking bad", "dune", "one piece"). The query is optional when using `--history` or `--recommend`.

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--action` | `-a` | Action to perform: `play` (default) or `download`. |
| `--season` | `-s` | (Series only) Specify the season number. |
| `--episodes` | `-e` | (Series only) Specify a single episode (`5`) or a range (`1-5`). |
| `--best` | `-b` | Auto-select the best available quality (skips fzf quality prompt). |
| `--provider` | `-p` | Select provider (e.g. `sflix`, `braflix`). Overrides config. |
| `--history` | `-H` | Browse watch history and resume a previous title. |
| `--recommend` | `-r` | Get personalised recommendations based on watch history. |
| `--show-image` | `true` | Show poster previews in fzf (requires chafa; skipped automatically when missing). Disable with `--show-image=false`. |
| `--debug` | `-d` | Print debug information (URLs, decryption steps, etc.). |
| `--help` | `-h` | Show help message and exit. |

### Playback Controls

When watching a TV series, an fzf menu appears alongside the player with four options:

| Action | Description |
|--------|-------------|
| **Next** | Kill the player and move to the next episode. |
| **Previous** | Kill the player and move to the previous episode. |
| **Replay** | Restart the current episode from the beginning. |
| **Quit** | Kill the player and exit. |

### Examples

**Search & Play a Movie**
```bash
pandaflix "dune"
```

**Download a Movie**
```bash
pandaflix "dune" --action download
```

**Play a TV Episode**
```bash
pandaflix "breaking bad" -s 1 -e 1
```

**Download a Range of Episodes**
```bash
pandaflix "stranger things" -s 2 -e 1-5 -a download
```

**Auto-select Best Quality**
```bash
pandaflix "dune" --best
```

**Use a Different Provider**
```bash
pandaflix "breaking bad" --provider sflix
```

**Resume from Watch History**
```bash
pandaflix --history
```

**Get Personalised Recommendations**
```bash
pandaflix --recommend
```

**Recommendations with Poster Previews**
```bash
pandaflix --recommend --show-image
```

## Configuration

The config file lives at `~/.config/pandaflix/config.yaml`. All fields are optional; defaults are shown below.

```yaml
# Path to the fzf binary. Set to an absolute path if fzf is not on PATH.
fzf_path: fzf

# Video player: "mpv" (default) or "vlc". IINA is used automatically on macOS.
player: mpv

# chafa rendering backend for poster previews.
# Options: sixel (default), kitty, iterm, symbols
image_backend: sixel

# Default search provider.
provider: flixhq

# Directory where downloaded files are saved. Defaults to home directory.
dl_path: ""

# Quality selection: leave empty to show an fzf prompt, or set to "best"
# to always auto-select the highest available quality.
quality: ""

# Hide the terminal window while the player is open and restore it when
# playback ends (Hyprland via hyprctl, other Linux via xdotool).
minimize_on_play: true

# Shift audio relative to video (seconds; positive delays audio, negative
# advances it) — use if a stream's audio plays slightly early/late.
audio_delay: 0

# Extra arguments appended to every mpv invocation (ignored on VLC/IINA/Android).
# mpv_args:
#   - "--hwdec=auto"
#   - "--volume=80"

# Lifecycle hooks — shell commands run at key playback events.
# LUFFY_TITLE, LUFFY_URL, LUFFY_SEASON, LUFFY_EPISODE, LUFFY_EP_NAME,
# LUFFY_PROVIDER, LUFFY_ACTION, LUFFY_STREAM_URL, and LUFFY_POSITION
# are set in the environment for every hook.
# hooks:
#   on_play: 'notify-send "Now playing" "$LUFFY_TITLE"'
#   on_exit: 'echo "$LUFFY_TITLE stopped at ${LUFFY_POSITION}s" >> ~/pandaflix.log'
#   on_download: 'notify-send "Downloading" "$LUFFY_TITLE"'
```

> [!IMPORTANT]
> To see poster images, your terminal emulator must support a graphics protocol.
> Supported terminals include kitty, ghostty, WezTerm, and foot (sixel).
>
> kitty, ghostty, and WezTerm are auto-detected; set `image_backend` in config only to override the detected format.

## Hooks & MpvArgs

Shell commands can be run at three lifecycle points, and extra mpv flags can be injected via config. See `docs/hooks.md` for the full reference.

### MpvArgs

Extra arguments appended verbatim to every mpv invocation (ignored on VLC / IINA / Android):

```yaml
mpv_args:
  - "--hwdec=auto"
  - "--volume=80"
```

### Hooks

```yaml
hooks:
  on_play: 'notify-send "Now playing" "$LUFFY_TITLE"'
  on_exit: 'echo "$LUFFY_TITLE stopped at ${LUFFY_POSITION}s" >> ~/pandaflix.log'
  on_download: 'notify-send "Downloading" "$LUFFY_TITLE"'
```

Every hook receives `LUFFY_TITLE`, `LUFFY_URL`, `LUFFY_SEASON`, `LUFFY_EPISODE`, `LUFFY_EP_NAME`, `LUFFY_PROVIDER`, `LUFFY_ACTION`, `LUFFY_STREAM_URL`, and `LUFFY_POSITION` as environment variables.

# Providers

You can set the default provider in the config file (`~/.config/pandaflix/config.yaml`) or override it per-run with `--provider`.

| Provider | `provider:` value | Content | Notes |
|----------|-------------------|---------|-------|
| Cinejoy | `cinejoy` | Movies & TV | Default. Browser-free sealed resolver; auto-races Lisbon/Nebula/Solara. |
| VixSrc | `vixsrc` | Movies & TV | Independent backup. Listed in the Servers menu; also used automatically when Cinejoy fails. |
| Anime (sub) | `anime` or `allanime` | Anime | Subtitled. Uses AllAnime. |
| Anime (dub) | `anime-dub` or `allanime-dub` | Anime | Dubbed. Uses AllAnime. |

Legacy `cineby`/`vidking`/`videasy` configs keep working and resolve via Cinejoy; the Videasy backend is being discontinued. Example config:
```yaml
provider: cinejoy
```

---

## Debugging

Run any command with `--debug` (`-d`) to print detailed information:

```bash
pandaflix "breaking bad" -s 1 -e 1 --debug
```

Debug output includes:
- The embed URL being resolved
- The master m3u8 URL
- The referer header used
- Number of subtitle tracks found
- The exact mpv command being run

If playback fails, the debug output is almost always enough to diagnose the problem.

---

## Before Opening an Issue

Please try these steps first:

1. **Update pandaflix** — most issues are already fixed in the latest version.
   ```bash
   go install github.com/MADPANDA3D/pandaflix@latest
   ```

2. **Run with `--debug`** and read the output — it usually tells you exactly what failed.

3. **Try a different title** — the content may be unavailable on the provider.

4. **Check the [Discord](https://discord.gg/X6FzCz5vR)** — someone may have already reported the same issue.

If none of the above helps, open an issue and include:
- The exact command you ran
- The full `--debug` output
- Your OS and pandaflix version (`pandaflix --version` if available, or `git log -1 --oneline`)
- Whether the issue is with a movie or TV show, and the title
