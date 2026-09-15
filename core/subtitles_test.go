package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickSubtitle(t *testing.T) {
	results := []openSubtitleResult{
		{SubDownloadLink: "https://x/1.srt", SubFormat: "srt", SubHearingImpaired: "1", SubDownloadsCnt: "999999", SubRating: "10.0"},
		{SubDownloadLink: "https://x/2.srt", SubFormat: "srt", SubHearingImpaired: "0", SubDownloadsCnt: "100"},
		{SubDownloadLink: "https://x/3.srt", SubFormat: "srt", SubHearingImpaired: "0", SubDownloadsCnt: "50000"},
		{SubDownloadLink: "https://x/4.zip", SubFormat: "zip", SubHearingImpaired: "0", SubDownloadsCnt: "9999999", SubRating: "10.0"},
		{SubDownloadLink: "https://x/5.srt", SubFormat: "srt", SubHearingImpaired: "0", SubDownloadsCnt: "69000", SubRating: "10.0"},
	}
	// The rated non-HI release wins over the higher-download unrated one.
	if got := pickSubtitle(results); got != "https://x/5.srt" {
		t.Fatalf("rated release not preferred: %q", got)
	}

	// Without ratings, download count decides.
	unrated := []openSubtitleResult{
		{SubDownloadLink: "https://x/a.srt", SubFormat: "srt", SubHearingImpaired: "0", SubDownloadsCnt: "100"},
		{SubDownloadLink: "https://x/b.srt", SubFormat: "srt", SubHearingImpaired: "0", SubDownloadsCnt: "5000"},
	}
	if got := pickSubtitle(unrated); got != "https://x/b.srt" {
		t.Fatalf("download count tie-break broken: %q", got)
	}

	// When no non-HI srt exists, fall back to the first downloadable entry.
	mixed := []openSubtitleResult{
		{SubDownloadLink: "", SubFormat: "srt", SubHearingImpaired: "0"},
		{SubDownloadLink: "https://x/only.zip", SubFormat: "zip", SubHearingImpaired: "1"},
	}
	if got := pickSubtitle(mixed); got != "https://x/only.zip" {
		t.Fatalf("fallback entry not chosen: %q", got)
	}

	if got := pickSubtitle(nil); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestRankSubtitles(t *testing.T) {
	results := []openSubtitleResult{
		{SubDownloadLink: "https://x/hi.srt", SubFormat: "srt", SubHearingImpaired: "1", SubDownloadsCnt: "999999", SubRating: "10"},
		{SubDownloadLink: "https://x/low.srt", SubFormat: "srt", SubHearingImpaired: "0", SubDownloadsCnt: "10", SubRating: "2"},
		{SubDownloadLink: "https://x/high.srt", SubFormat: "srt", SubHearingImpaired: "0", SubDownloadsCnt: "100", SubRating: "9"},
	}
	ranked := rankSubtitles(results)
	if len(ranked) != 2 || ranked[0] != "https://x/high.srt" {
		t.Fatalf("unexpected ranking: %v", ranked)
	}
}

func TestSubtitleSpamFiltering(t *testing.T) {
	dir := t.TempDir()
	spamPath := filepath.Join(dir, "spam.srt")
	spam := "1\n00:00:06,000 --> 00:00:12,074\nWatch Online Movies and Series for FREE\nwww.osdb.link/lm\n\n2\n00:00:40,000 --> 00:00:41,700\nLunchtime!\n"
	if err := os.WriteFile(spamPath, []byte(spam), 0600); err != nil {
		t.Fatal(err)
	}
	if !subtitleLooksLikeSpam(spamPath) {
		t.Fatal("spam subtitle not detected")
	}
	cleanPath, err := sanitizeSubtitleSpam(spamPath)
	if err != nil {
		t.Fatal(err)
	}
	cleaned, err := os.ReadFile(cleanPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(cleaned)
	if strings.Contains(text, "osdb.link") || strings.Contains(text, "Watch Online") {
		t.Fatalf("spam cues survived cleaning: %q", text)
	}
	if !strings.Contains(text, "Lunchtime!") {
		t.Fatalf("real cue lost: %q", text)
	}

	goodPath := filepath.Join(dir, "good.srt")
	if err := os.WriteFile(goodPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello there\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if subtitleLooksLikeSpam(goodPath) {
		t.Fatal("clean subtitle flagged as spam")
	}
}
