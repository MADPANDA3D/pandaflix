package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DownloadYTDLP downloads a URL using yt-dlp. It is intended for providers
// (e.g. YouTube) whose URLs yt-dlp handles natively.
func DownloadYTDLP(basePath, dlPath, name, url, referer, userAgent string, debug bool) error {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return fmt.Errorf("yt-dlp not found: please install yt-dlp")
	}

	if dlPath == "" {
		dlPath = filepath.Join(basePath, "Downloads", "pandaflix")
	} else {
		dlPath = filepath.Join(dlPath, "pandaflix")
	}
	if err := os.MkdirAll(dlPath, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	cleanName := sanitizeFilename(name)
	outputTemplate := filepath.Join(dlPath, cleanName+".%(ext)s")

	args := []string{
		url,
		"--no-skip-unavailable-fragments",
		"--fragment-retries", "infinite",
		"--retries", "infinite",
		"-N", "16",
		"-o", outputTemplate,
	}
	if referer != "" {
		args = append(args, "--referer", referer)
	}
	if userAgent != "" {
		args = append(args, "--user-agent", userAgent)
	}

	if debug {
		fmt.Printf("[yt-dlp] Running: yt-dlp %s\n", strings.Join(args, " "))
	}

	cmd := exec.Command("yt-dlp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yt-dlp failed: %w", err)
	}

	fmt.Println("[download] Complete!")
	return nil
}
