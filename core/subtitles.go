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
func FetchOpenSubtitles(imdbID string, season, episode int, client *http.Client) []string {
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

	// Try candidates best-first (rating, then downloads) and skip spam uploads
	// whose files carry advertising cues and often mismatched timings.
	candidates := rankSubtitles(results)
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

// rankSubtitles orders non-hearing-impaired SRT results best-first: rated
// uploads, then download count.
func rankSubtitles(results []openSubtitleResult) []string {
	type scored struct {
		url   string
		score float64
	}
	var items []scored
	for _, r := range results {
		if r.SubDownloadLink == "" || r.SubFormat != "srt" || r.SubHearingImpaired != "0" {
			continue
		}
		rating, _ := strconv.ParseFloat(r.SubRating, 64)
		count, _ := strconv.Atoi(r.SubDownloadsCnt)
		items = append(items, scored{url: r.SubDownloadLink, score: rating*100000 + float64(count)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].score > items[j].score })
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
