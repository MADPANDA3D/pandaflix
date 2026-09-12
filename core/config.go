package core

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// migrateOnce guards the one-time ingestion of legacy ~/.config/luffy data.
var migrateOnce sync.Once

// pandaflixConfigDir returns the pandaflix config directory, migrating any
// legacy luffy data on first use: the old directory is moved into place when
// possible, otherwise config.yaml and history.sqlite are copied over; either
// way the old directory is deleted so pandaflix fully owns the new path.
func pandaflixConfigDir(home string) string {
	newDir := filepath.Join(home, ".config", "pandaflix")
	oldDir := filepath.Join(home, ".config", "luffy")
	migrateOnce.Do(func() {
		if _, err := os.Stat(newDir); err == nil {
			return // pandaflix already owns its directory
		}
		if info, err := os.Stat(oldDir); err != nil || !info.IsDir() {
			return // nothing to ingest
		}
		if err := os.MkdirAll(filepath.Dir(newDir), 0700); err != nil {
			return
		}
		if err := os.Rename(oldDir, newDir); err == nil {
			return // whole directory ingested atomically
		}
		// Fallback copy (e.g. cross-device), then remove the old directory.
		entries, err := os.ReadDir(oldDir)
		if err != nil {
			return
		}
		ok := true
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			src := filepath.Join(oldDir, entry.Name())
			if !(entry.Name() == "config.yaml" || entry.Name() == "history.sqlite") {
				continue
			}
			if err := copyFile(src, filepath.Join(newDir, entry.Name())); err != nil {
				ok = false
				break
			}
		}
		if ok {
			os.RemoveAll(oldDir) //nolint:errcheck // ingest is best-effort
		}
	})
	return newDir
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// HooksConfig holds shell commands to run at specific playback lifecycle points.
// Each hook is a shell command string (executed via sh -c on Unix, cmd /c on Windows).
// The following environment variables are set for every hook:
//
//	LUFFY_TITLE      – media title
//	LUFFY_URL        – provider media URL
//	LUFFY_SEASON     – season number (0 for movies)
//	LUFFY_EPISODE    – episode number (0 for movies)
//	LUFFY_EP_NAME    – episode name (empty for movies)
//	LUFFY_PROVIDER   – provider name
//	LUFFY_ACTION     – "play" or "download"
//	LUFFY_STREAM_URL – resolved stream URL (set for on_play / on_download)
//	LUFFY_POSITION   – playback position in seconds (set for on_exit only)
type HooksConfig struct {
	// OnPlay is run just before the player is launched.
	OnPlay string `yaml:"on_play"`
	// OnExit is run after the player exits (mpv/vlc closed).
	OnExit string `yaml:"on_exit"`
	// OnDownload is run just before yt-dlp / ffmpeg is launched.
	OnDownload string `yaml:"on_download"`
}

type Config struct {
	FzfPath      string `yaml:"fzf_path"`
	Player       string `yaml:"player"`
	ImageBackend string `yaml:"image_backend"`
	Provider     string `yaml:"provider"`
	DlPath       string `yaml:"dl_path"`
	Quality      string `yaml:"quality"`
	// MinimizeOnPlay hides the terminal window while the player is open and
	// restores it when playback ends (Hyprland via hyprctl, else xdotool).
	MinimizeOnPlay bool `yaml:"minimize_on_play"`
	// AudioDelay shifts audio relative to video in seconds for mpv: positive
	// values delay audio (use when audio plays early), negative advance it.
	AudioDelay float64 `yaml:"audio_delay"`
	// MpvArgs holds extra command-line arguments appended to every mpv invocation.
	// Example: ["--hwdec=auto", "--volume=80"]
	MpvArgs []string    `yaml:"mpv_args"`
	Hooks   HooksConfig `yaml:"hooks"`
	YtLang  string      `yaml:"yt_language"`
}

func LoadConfig() *Config {
	config := &Config{
		FzfPath:        "fzf",    // Default
		Player:         "mpv",    // Default player
		ImageBackend:   "sixel",  // Default image backend
		Provider:       "flixhq", // Default provider
		DlPath:         "",       // Default: use home directory
		Quality:        "",       // Default: prompt user to select quality
		MinimizeOnPlay: true,     // Default: hide terminal while the player is open
		YtLang:         "",       // Default: let youtube decide
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return config
	}

	configPath := filepath.Join(pandaflixConfigDir(home), "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Config file doesn't exist or can't be read, use defaults
		return config
	}

	// Parse YAML into config struct
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return &Config{
			FzfPath:        "fzf",
			Player:         "mpv",
			ImageBackend:   "sixel",
			Provider:       "flixhq",
			DlPath:         "",
			Quality:        "",
			MinimizeOnPlay: true,
			YtLang:         "",
		}
	}

	return config
}
