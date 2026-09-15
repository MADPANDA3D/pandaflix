package core

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// openSubtitleResult is one entry from the OpenSubtitles legacy search API.
type openSubtitleResult struct {
	SubDownloadLink    string `json:"SubDownloadLink"`
	SubFormat          string `json:"SubFormat"`
	SubHearingImpaired string `json:"SubHearingImpaired"`
	SubDownloadsCnt    string `json:"SubDownloadsCnt"`
	SubRating          string `json:"SubRating"`
	SubLastTS          string `json:"SubLastTS"`
}

// pickSubtitle chooses the best non-hearing-impaired SRT result. Ratings are
// weighted first (a rated upload is usually a clean release rather than a
// monetized spam upload), then download count. Falls back to the first
// downloadable entry.
func pickSubtitle(results []openSubtitleResult) string {
	best := ""
	bestScore := -1.0
	for _, r := range results {
		if r.SubDownloadLink == "" || r.SubFormat != "srt" || r.SubHearingImpaired != "0" {
			continue
		}
		rating, _ := strconv.ParseFloat(r.SubRating, 64)
		count, _ := strconv.Atoi(r.SubDownloadsCnt)
		score := rating*100000 + float64(count)
		if score > bestScore {
			best, bestScore = r.SubDownloadLink, score
		}
	}
	if best != "" {
		return best
	}
	for _, r := range results {
		if r.SubDownloadLink != "" {
			return r.SubDownloadLink
		}
	}
	return ""
}

// FetchOpenSubtitles fetches English subtitles from OpenSubtitles legacy API using an IMDB ID.
// Supports season and episode filters for TV shows to ensure precise subtitle timing.
// Downloads and decompresses the subtitle locally to prevent player and compression issues.
// Returns up to 1 local subtitle file path. Falls back gracefully on any error.
func FetchOpenSubtitles(imdbID string, season, episode, durationSecs int, client *http.Client) []string {
	if imdbID == "" {
		return nil
	}
	// Strip "tt" prefix if present
	imdbID = strings.TrimPrefix(imdbID, "tt")

	var apiURL string
	if season > 0 && episode > 0 {
		apiURL = fmt.Sprintf("https://rest.opensubtitles.org/search/episode-%d/imdbid-%s/season-%d/sublanguageid-eng", episode, imdbID, season)
	} else {
		apiURL = fmt.Sprintf("https://rest.opensubtitles.org/search/imdbid-%s/sublanguageid-eng", imdbID)
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "TemporaryUserAgent")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	var results []openSubtitleResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil
	}

	// Try candidates best-first (runtime match when known, then rating and
	// downloads) and skip spam uploads whose files often carry mismatched
	// timings.
	candidates := rankSubtitles(results, durationSecs)
	var firstNonSpam string
	for i, candidateURL := range candidates {
		if i >= 6 {
			break
		}
		localPath, err := downloadAndDecompressGzip(candidateURL, client)
		if err != nil {
			continue
		}
		if !subtitleLooksLikeSpam(localPath) {
			return []string{localPath}
		}
		if firstNonSpam == "" {
			if cleaned, cleanErr := sanitizeSubtitleSpam(localPath); cleanErr == nil {
				firstNonSpam = cleaned
			}
		}
	}
	if firstNonSpam != "" {
		return []string{firstNonSpam}
	}
	return nil
}

// rankSubtitles orders non-hearing-impaired SRT results best-first. When the
// stream duration is known, subtitles whose last timestamp is closest to it
// come first (that is what keeps timing aligned); ties and unknown durations
// fall back to rating then download count.
func rankSubtitles(results []openSubtitleResult, durationSecs int) []string {
	type scored struct {
		url      string
		diffSecs int
		hasTS    bool
		score    float64
	}
	var items []scored
	for _, r := range results {
		if r.SubDownloadLink == "" || r.SubFormat != "srt" || r.SubHearingImpaired != "0" {
			continue
		}
		rating, _ := strconv.ParseFloat(r.SubRating, 64)
		count, _ := strconv.Atoi(r.SubDownloadsCnt)
		item := scored{url: r.SubDownloadLink, score: rating*100000 + float64(count)}
		if ts, ok := parseSubtitleClock(r.SubLastTS); ok && durationSecs > 0 {
			item.hasTS = true
			item.diffSecs = ts - durationSecs
			if item.diffSecs < 0 {
				item.diffSecs = -item.diffSecs
			}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if durationSecs > 0 && items[i].hasTS != items[j].hasTS {
			return items[i].hasTS
		}
		if durationSecs > 0 && items[i].hasTS && items[j].hasTS && items[i].diffSecs != items[j].diffSecs {
			return items[i].diffSecs < items[j].diffSecs
		}
		return items[i].score > items[j].score
	})
	urls := make([]string, 0, len(items))
	for _, item := range items {
		urls = append(urls, item.url)
	}
	if len(urls) == 0 {
		for _, r := range results {
			if r.SubDownloadLink != "" {
				urls = append(urls, r.SubDownloadLink)
			}
		}
	}
	return urls
}

// parseSubtitleClock parses "HH:MM:SS" or "HH:MM:SS.mmm" into seconds.
func parseSubtitleClock(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 {
		return 0, false
	}
	hours, err1 := strconv.Atoi(parts[0])
	minutes, err2 := strconv.Atoi(parts[1])
	secondsPart := parts[2]
	if idx := strings.Index(secondsPart, "."); idx != -1 {
		secondsPart = secondsPart[:idx]
	}
	seconds, err3 := strconv.Atoi(secondsPart)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return hours*3600 + minutes*60 + seconds, true
}

// StreamDuration estimates an HLS stream's total duration in seconds by
// summing the media playlist's EXTINF values. ok is false when unavailable.
func StreamDuration(streamURL, referer string, client *http.Client) (int, bool) {
	fetch := func(rawURL string) (string, bool) {
		req, err := NewRequest("GET", rawURL)
		if err != nil {
			return "", false
		}
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", false
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", false
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return "", false
		}
		return string(data), true
	}

	body, ok := fetch(streamURL)
	if !ok {
		return 0, false
	}
	// Master playlist: follow the first variant.
	if strings.Contains(body, "#EXT-X-STREAM-INF:") {
		var next string
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			if !strings.HasPrefix(strings.TrimSpace(line), "#EXT-X-STREAM-INF:") {
				continue
			}
			for _, candidate := range lines[i+1:] {
				candidate = strings.TrimSpace(candidate)
				if candidate == "" || strings.HasPrefix(candidate, "#") {
					continue
				}
				next = candidate
				break
			}
			break
		}
		if next == "" {
			return 0, false
		}
		body, ok = fetch(resolvePlaylistURL(streamURL, next))
		if !ok {
			return 0, false
		}
	}
	total := 0.0
	found := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		value := strings.TrimPrefix(line, "#EXTINF:")
		if idx := strings.Index(value, ","); idx != -1 {
			value = value[:idx]
		}
		if seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			total += seconds
			found = true
		}
	}
	if !found || total <= 0 {
		return 0, false
	}
	return int(total), true
}

// subtitleSpamMarkers are advertising strings found in junk uploads.
var subtitleSpamMarkers = []string{
	"osdb.link", "opensubtitles", "open subtitles",
	"watch online", "watch movies", "watch tv",
	"subscene", "addic7ed", "yts.mx", "yify",
	"download free", "free movies", "www.",
}

// subtitleLooksLikeSpam reports whether the first cues contain advertising.
func subtitleLooksLikeSpam(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	head := strings.ToLower(string(data))
	if len(head) > 4000 {
		head = head[:4000]
	}
	for _, marker := range subtitleSpamMarkers {
		if strings.Contains(head, marker) {
			return true
		}
	}
	return false
}

// sanitizeSubtitleSpam removes cue blocks that contain advertising markers,
// returning the path of the cleaned file (in the same directory).
func sanitizeSubtitleSpam(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	blocks := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n\n")
	var kept []string
	for _, block := range blocks {
		lower := strings.ToLower(block)
		spam := false
		for _, marker := range subtitleSpamMarkers {
			if strings.Contains(lower, marker) {
				spam = true
				break
			}
		}
		if !spam && strings.TrimSpace(block) != "" {
			kept = append(kept, block)
		}
	}
	if len(kept) == 0 {
		return "", fmt.Errorf("no usable cues")
	}
	cleanedPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".clean.srt"
	if err := os.WriteFile(cleanedPath, []byte(strings.Join(kept, "\n\n")+"\n"), 0600); err != nil {
		return "", err
	}
	return cleanedPath, nil
}

func downloadAndDecompressGzip(subURL string, client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", subURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "TemporaryUserAgent")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download subtitle: status %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if strings.HasSuffix(strings.ToLower(subURL), ".gz") || resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	tempFile, err := os.CreateTemp("", "pandaflix-sub-*.srt")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	_, err = io.Copy(tempFile, reader)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

// GetIMDBIDFromTMDB fetches the IMDB ID for a given TMDB ID and media type.
func GetIMDBIDFromTMDB(tmdbID, mediaType string, client *http.Client) string {
	if tmdbID == "" {
		return ""
	}
	if mediaType == "" {
		mediaType = "movie"
	}
	// TMDB API expects "tv" instead of "series"
	if mediaType == "series" {
		mediaType = "tv"
	}
	apiURL := fmt.Sprintf("%s/%s/%s/external_ids?api_key=%s", TMDB_BASE_URL, mediaType, tmdbID, TMDB_API_KEY)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var data struct {
		IMDBID string `json:"imdb_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	return data.IMDBID
}

// GetIMDBIDByTitle searches TMDB by title to find the IMDB ID.
func GetIMDBIDByTitle(title string, mediaType MediaType, client *http.Client) string {
	if title == "" {
		return ""
	}
	tmdbMediaType := string(mediaType)
	if tmdbMediaType == "series" {
		tmdbMediaType = "tv"
	}

	endpoint := fmt.Sprintf(
		"%s/search/multi?api_key=%s&query=%s",
		TMDB_BASE_URL,
		TMDB_API_KEY,
		url.QueryEscape(title),
	)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var data struct {
		Results []struct {
			ID        int    `json:"id"`
			MediaType string `json:"media_type"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}

	for _, r := range data.Results {
		if r.MediaType == "movie" || r.MediaType == "tv" {
			if tmdbMediaType != "" && r.MediaType != tmdbMediaType {
				continue
			}
			imdbID := GetIMDBIDFromTMDB(fmt.Sprintf("%d", r.ID), r.MediaType, client)
			if imdbID != "" {
				return imdbID
			}
		}
	}

	for _, r := range data.Results {
		if r.MediaType == "movie" || r.MediaType == "tv" {
			imdbID := GetIMDBIDFromTMDB(fmt.Sprintf("%d", r.ID), r.MediaType, client)
			if imdbID != "" {
				return imdbID
			}
		}
	}

	return ""
}

// ExtractTMDBIDFromURL tries to extract a TMDB ID from provider URLs.
// Supports patterns like /movie/123, /tv/123, /embed/movie/123, /embed/tv/123/1/1
func ExtractTMDBIDFromURL(rawURL string) (tmdbID, mediaType string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if (p == "movie" || p == "tv") && i+1 < len(parts) {
			id := parts[i+1]
			// Verify it looks like a numeric ID
			if len(id) > 0 && id[0] >= '0' && id[0] <= '9' {
				return id, p
			}
		}
	}
	return "", ""
}
