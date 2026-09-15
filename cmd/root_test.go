package cmd

import (
	"testing"

	"github.com/MADPANDA3D/pandaflix/core"
)

func TestPosterNameFromLabel(t *testing.T) {
	cases := map[string]string{
		"[series] The Blacklist (2013)": "The Blacklist (2013)",
		"[movie] The Goonies (1985)":    "The Goonies (1985)",
		"[movie] Title":                 "Title",
		"plain":                         "plain",
	}
	for label, want := range cases {
		if got := posterNameFromLabel(label); got != want {
			t.Errorf("posterNameFromLabel(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestBuildEpisodePlaylist(t *testing.T) {
	mid := buildEpisodePlaylist("https://x/ep.m3u8", 1, 3, true)
	wantMid := core.PlaylistPlaceholder + "\nhttps://x/ep.m3u8\n" + core.PlaylistPlaceholder
	if mid != wantMid {
		t.Fatalf("middle episode playlist wrong:\n got %q\nwant %q", mid, wantMid)
	}
	first := buildEpisodePlaylist("https://x/ep.m3u8", 0, 3, true)
	if first != "https://x/ep.m3u8\n"+core.PlaylistPlaceholder {
		t.Fatalf("first episode playlist wrong: %q", first)
	}
	last := buildEpisodePlaylist("https://x/ep.m3u8", 2, 3, true)
	if last != core.PlaylistPlaceholder+"\nhttps://x/ep.m3u8" {
		t.Fatalf("last episode playlist wrong: %q", last)
	}
	single := buildEpisodePlaylist("https://x/ep.m3u8", 0, 1, true)
	if single != "https://x/ep.m3u8" {
		t.Fatalf("single episode should stay plain: %q", single)
	}
	if off := buildEpisodePlaylist("https://x/ep.m3u8", 1, 3, false); off != "https://x/ep.m3u8" {
		t.Fatalf("auto_next=false should stay plain: %q", off)
	}
}
