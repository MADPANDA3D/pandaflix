package core

import "testing"

func TestDetectImageBackend(t *testing.T) {
	// Kitty-protocol terminals are detected via identifiers/TERM/TERM_PROGRAM.
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("WEZTERM_PANE", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "xterm-kitty")
	if got := DetectImageBackend("sixel"); got != "kitty" {
		t.Fatalf("kitty TERM not detected: %q", got)
	}

	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "ghostty")
	if got := DetectImageBackend("sixel"); got != "kitty" {
		t.Fatalf("ghostty TERM_PROGRAM not detected: %q", got)
	}

	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "1")
	if got := DetectImageBackend("sixel"); got != "kitty" {
		t.Fatalf("kitty window id not detected: %q", got)
	}

	// Sixel terminals keep the sixel renderer.
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("TERM", "foot")
	if got := DetectImageBackend("kitty"); got != "sixel" {
		t.Fatalf("foot not detected: %q", got)
	}

	// Unknown terminals fall back to the configured backend.
	t.Setenv("TERM", "xterm-256color")
	if got := DetectImageBackend("sixel"); got != "sixel" {
		t.Fatalf("configured fallback broken: %q", got)
	}
}
