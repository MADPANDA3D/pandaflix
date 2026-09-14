package core

import "testing"

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
