package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MADPANDA3D/pandaflix/core"
)

// LookMovieBaseURL is the site origin for search, pages, and API calls.
const LookMovieBaseURL = "https://www.lookmovie2.to"

const lookmovieHTTPTimeout = 20 * time.Second

// LookMovie resolves movies and shows through LookMovie's own JSON API. IDs
// use LookMovie's IMDb-keyed slugs:
//
//	movie:  lookmovie|movie|<slug>
//	series: lookmovie|series|<slug>[|<season>|<episode>]
type LookMovie struct {
	Client *http.Client
}

func NewLookMovie(client *http.Client) *LookMovie {
	return &LookMovie{Client: client}
}

type lookmovieRequest struct {
	kind    string // movie | series
	slug    string
	season  string
	episode string
}

func lookmovieID(r lookmovieRequest) string {
	if r.kind == "series" {
		id := "lookmovie|series|" + r.slug
		if r.season != "" || r.episode != "" {
			id += "|" + r.season + "|" + r.episode
		}
		return id
	}
	return "lookmovie|movie|" + r.slug
}

func parseLookMovieID(id string) (lookmovieRequest, error) {
	parts := strings.Split(id, "|")
	if len(parts) < 3 || parts[0] != "lookmovie" || (parts[1] != "movie" && parts[1] != "series") || parts[2] == "" {
		return lookmovieRequest{}, fmt.Errorf("invalid lookmovie ID")
	}
	r := lookmovieRequest{kind: parts[1], slug: parts[2]}
	if len(parts) > 3 {
		r.season = parts[3]
	}
	if len(parts) > 4 {
		r.episode = parts[4]
	}
	return r, nil
}

// --- Provider interface -----------------------------------------------------

type lookmovieSearchResult struct {
	IDMovie int    `json:"id_movie"`
	IDShow  int    `json:"id_show"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Year    any    `json:"year"`
}

func (l *LookMovie) Search(query string) ([]core.SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)

	var results []core.SearchResult
	for _, kind := range []struct {
		api      string
		path     string
		mediaTyp core.MediaType
	}{
		{"movies", "movies", core.Movie},
		{"shows", "shows", core.Series},
	} {
		body, err := lookmovieFetch(context.Background(), l.Client, LookMovieBaseURL+"/api/v1/"+kind.api+"/do-search/?"+params.Encode(), 2<<20)
		if err != nil {
			continue
		}
		results = append(results, parseLookMovieSearch(body, kind.path, kind.mediaTyp)...)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results")
	}
	return results, nil
}

func parseLookMovieSearch(body []byte, path string, mediaType core.MediaType) []core.SearchResult {
	var data struct {
		Result []lookmovieSearchResult `json:"result"`
	}
	if json.Unmarshal(body, &data) != nil {
		return nil
	}
	var out []core.SearchResult
	for _, r := range data.Result {
		title := strings.TrimSpace(r.Title)
		if title == "" || r.Slug == "" {
			continue
		}
		year := ""
		switch v := r.Year.(type) {
		case float64:
			year = fmt.Sprintf("%d", int(v))
		case string:
			year = v
		}
		if len(year) > 4 {
			year = year[:4]
		}
		out = append(out, core.SearchResult{
			Title: title,
			URL:   fmt.Sprintf("%s/%s/view/%s", LookMovieBaseURL, path, r.Slug),
			Type:  mediaType,
			Year:  year,
		})
	}
	return out
}

func (l *LookMovie) GetMediaID(mediaURL string) (string, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[1] != "view" {
		return "", fmt.Errorf("invalid lookmovie URL")
	}
	switch parts[0] {
	case "movies":
		return lookmovieID(lookmovieRequest{kind: "movie", slug: parts[2]}), nil
	case "shows":
		return lookmovieID(lookmovieRequest{kind: "series", slug: parts[2]}), nil
	}
	return "", fmt.Errorf("invalid lookmovie URL")
}

func (l *LookMovie) GetSeasons(mediaID string) ([]core.Season, error) {
	req, err := parseLookMovieID(mediaID)
	if err != nil {
		return nil, err
	}
	if req.kind != "series" {
		return nil, nil
	}
	seasons, err := l.lookmovieSeasons(req.slug)
	if err != nil {
		return nil, err
	}
	var out []core.Season
	for _, number := range sortedSeasonNumbers(seasons) {
		out = append(out, core.Season{
			ID:   lookmovieID(lookmovieRequest{kind: "series", slug: req.slug, season: number}),
			Name: "Season " + number,
		})
	}
	return out, nil
}

func (l *LookMovie) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	if !isSeason {
		// Movie path (recommendation flow): the episode list IS the server list.
		return []core.Episode{{ID: id, Name: "LookMovie"}}, nil
	}
	req, err := parseLookMovieID(id)
	if err != nil {
		return nil, err
	}
	if req.kind != "series" || req.season == "" {
		return nil, fmt.Errorf("invalid lookmovie season ID")
	}
	seasons, err := l.lookmovieSeasons(req.slug)
	if err != nil {
		return nil, err
	}
	season, ok := seasons[req.season]
	if !ok {
		return nil, fmt.Errorf("lookmovie: season %s not found", req.season)
	}
	var out []core.Episode
	for _, ep := range sortedEpisodes(season) {
		out = append(out, core.Episode{
			ID:   lookmovieID(lookmovieRequest{kind: "series", slug: req.slug, season: req.season, episode: ep.EpisodeNumber}),
			Name: fmt.Sprintf("E%02d - %s", atoiSafe(ep.EpisodeNumber), ep.Title),
		})
	}
	return out, nil
}

func (l *LookMovie) GetServers(id string) ([]core.Server, error) {
	return []core.Server{{ID: id, Name: "LookMovie"}}, nil
}

func (l *LookMovie) GetLink(serverID string) (string, error) {
	req, err := parseLookMovieID(serverID)
	if err != nil {
		return "", err
	}
	if req.kind == "series" && (req.season == "" || req.episode == "") {
		return "", fmt.Errorf("lookmovie: series resolution requires season and episode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), lookmovieHTTPTimeout)
	defer cancel()

	// View page -> play page (carries the hash/expires credentials).
	viewPath := "movies/view/"
	if req.kind == "series" {
		viewPath = "shows/view/"
	}
	viewBody, err := lookmovieFetch(ctx, l.Client, LookMovieBaseURL+"/"+viewPath+req.slug, 4<<20)
	if err != nil {
		return "", fmt.Errorf("lookmovie: view page: %w", err)
	}
	playPath, err := parseLookMoviePlayLink(string(viewBody), req.kind)
	if err != nil {
		return "", err
	}
	playBody, err := lookmovieFetch(ctx, l.Client, LookMovieBaseURL+playPath, 6<<20)
	if err != nil {
		return "", fmt.Errorf("lookmovie: play page: %w", err)
	}
	hash, expires, err := parseLookMovieStorage(string(playBody))
	if err != nil {
		return "", err
	}

	var accessURL string
	if req.kind == "series" {
		seasons, err := parseLookMovieSeasons(string(playBody))
		if err != nil {
			return "", err
		}
		season, ok := seasons[req.season]
		if !ok {
			return "", fmt.Errorf("lookmovie: season %s not found", req.season)
		}
		var episodeID string
		for _, ep := range season {
			if ep.EpisodeNumber == req.episode {
				episodeID = ep.IDEpisode
				break
			}
		}
		if episodeID == "" {
			return "", fmt.Errorf("lookmovie: episode %s not found", req.episode)
		}
		accessURL = fmt.Sprintf("%s/api/v1/security/episode-access?id_episode=%s&hash=%s&expires=%s",
			LookMovieBaseURL, url.QueryEscape(episodeID), url.QueryEscape(hash), url.QueryEscape(expires))
	} else {
		idMovie := parseLookMovieMovieID(string(playBody))
		if idMovie == "" {
			return "", fmt.Errorf("lookmovie: movie id not found")
		}
		accessURL = fmt.Sprintf("%s/api/v1/security/movie-access?id_movie=%s&hash=%s&expires=%s",
			LookMovieBaseURL, url.QueryEscape(idMovie), url.QueryEscape(hash), url.QueryEscape(expires))
	}

	accessBody, err := lookmovieFetch(ctx, l.Client, accessURL, 4<<20)
	if err != nil {
		return "", fmt.Errorf("lookmovie: access: %w", err)
	}
	stream, subtitles, err := parseLookMovieAccess(accessBody)
	if err != nil {
		return "", err
	}
	if err := lookmovieValidateStream(ctx, l.Client, stream); err != nil {
		return "", err
	}

	link := stream
	if len(subtitles) > 0 {
		link += "|subs=" + url.QueryEscape(subtitles[0])
	}
	return link, nil
}

// --- Pages and API parsing (pure helpers) -----------------------------------

// lookmovieSeasons fetches the show's play page and parses its season data.
func (l *LookMovie) lookmovieSeasons(slug string) (map[string][]lookmovieEpisode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), lookmovieHTTPTimeout)
	defer cancel()

	viewBody, err := lookmovieFetch(ctx, l.Client, LookMovieBaseURL+"/shows/view/"+slug, 4<<20)
	if err != nil {
		return nil, fmt.Errorf("lookmovie: show page: %w", err)
	}
	playPath, err := parseLookMoviePlayLink(string(viewBody), "series")
	if err != nil {
		return nil, err
	}
	playBody, err := lookmovieFetch(ctx, l.Client, LookMovieBaseURL+playPath, 6<<20)
	if err != nil {
		return nil, fmt.Errorf("lookmovie: show play page: %w", err)
	}
	return parseLookMovieSeasons(string(playBody))
}

type lookmovieEpisode struct {
	EpisodeNumber string `json:"episode_number"`
	IDEpisode     string `json:"id_episode"`
	Title         string `json:"title"`
}

var (
	lookmovieMoviePlayRE = regexp.MustCompile(`href="(/movies/play/[^"]+)"`)
	lookmovieShowPlayRE  = regexp.MustCompile(`href="(/shows/play/[^"]+)"`)
	lookmovieHashRE      = regexp.MustCompile(`hash:\s*["']([^"']+)["']`)
	lookmovieExpiresRE   = regexp.MustCompile(`expires:\s*(\d+)`)
	lookmovieMovieIDRE   = regexp.MustCompile(`id_movie:\s*(\d+)`)
	lookmovieSeasonsRE   = regexp.MustCompile(`window\.seasons='(.*?)';`)
)

// parseLookMoviePlayLink extracts the play page path from a view page.
func parseLookMoviePlayLink(html, kind string) (string, error) {
	re := lookmovieMoviePlayRE
	if kind == "series" {
		re = lookmovieShowPlayRE
	}
	match := re.FindStringSubmatch(html)
	if len(match) < 2 {
		return "", fmt.Errorf("lookmovie: play link not found")
	}
	return match[1], nil
}

// parseLookMovieStorage extracts the hash/expires credentials from a play page.
func parseLookMovieStorage(html string) (hash, expires string, err error) {
	h := lookmovieHashRE.FindStringSubmatch(html)
	e := lookmovieExpiresRE.FindStringSubmatch(html)
	if len(h) < 2 || len(e) < 2 {
		return "", "", fmt.Errorf("lookmovie: play credentials not found")
	}
	return h[1], e[1], nil
}

// parseLookMovieMovieID extracts id_movie from a movie play page.
func parseLookMovieMovieID(html string) string {
	m := lookmovieMovieIDRE.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// parseLookMovieSeasons parses the embedded season/episode JSON from a show
// play page.
func parseLookMovieSeasons(html string) (map[string][]lookmovieEpisode, error) {
	m := lookmovieSeasonsRE.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil, fmt.Errorf("lookmovie: season data not found")
	}
	raw := unescapeJSString(m[1])
	var data map[string]struct {
		Episodes map[string]lookmovieEpisode `json:"episodes"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("lookmovie: invalid season data")
	}
	seasons := make(map[string][]lookmovieEpisode, len(data))
	for number, season := range data {
		for _, ep := range season.Episodes {
			seasons[number] = append(seasons[number], ep)
		}
	}
	return seasons, nil
}

// parseLookMovieAccess parses the stream/subtitle response of
// movie-access/episode-access and returns the best stream URL plus English
// subtitle URLs.
func parseLookMovieAccess(body []byte) (string, []string, error) {
	var data struct {
		Success   bool               `json:"success"`
		Streams   map[string]*string `json:"streams"`
		Subtitles []struct {
			Language string          `json:"language"`
			File     json.RawMessage `json:"file"`
			Kind     string          `json:"kind"`
		} `json:"subtitles"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", nil, fmt.Errorf("lookmovie: invalid access response (%d bytes): %w", len(body), err)
	}
	if !data.Success {
		return "", nil, fmt.Errorf("lookmovie: access denied")
	}
	// Movie responses use keys like "1080p"; episode responses use "1080".
	stream := ""
	for _, quality := range []string{"1080p", "1080", "720p", "720", "480p", "480", "360p", "360"} {
		if u, ok := data.Streams[quality]; ok && u != nil && *u != "" {
			stream = *u
			break
		}
	}
	if stream == "" {
		return "", nil, fmt.Errorf("lookmovie: no stream available")
	}
	var subtitles []string
	for _, sub := range data.Subtitles {
		if !strings.EqualFold(sub.Language, "English") {
			continue
		}
		for _, file := range decodeStringOrArray(sub.File) {
			if file != "" {
				subtitles = append(subtitles, LookMovieBaseURL+file)
			}
		}
	}
	return stream, subtitles, nil
}

// decodeStringOrArray accepts a JSON value that may be a string or an array of
// strings (subtitle files are sometimes grouped).
func decodeStringOrArray(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return []string{single}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	return nil
}

// unescapeJSString decodes a single-quoted JavaScript string literal body.
// The site serializes JSON into a JS string, so sequences such as \" and \'
// must be decoded before the JSON parser runs.
func unescapeJSString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\':
			b.WriteByte('\\')
		case '\'':
			b.WriteByte('\'')
		case '"':
			b.WriteByte('"')
		case '/':
			b.WriteByte('/')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func sortedSeasonNumbers(seasons map[string][]lookmovieEpisode) []string {
	numbers := make([]string, 0, len(seasons))
	for number := range seasons {
		numbers = append(numbers, number)
	}
	sortNumericStrings(numbers)
	return numbers
}

func sortedEpisodes(episodes []lookmovieEpisode) []lookmovieEpisode {
	sortNumericEpisodes(episodes)
	return episodes
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func sortNumericStrings(values []string) {
	sort.Slice(values, func(i, j int) bool { return atoiSafe(values[i]) < atoiSafe(values[j]) })
}

func sortNumericEpisodes(episodes []lookmovieEpisode) {
	sort.Slice(episodes, func(i, j int) bool {
		return atoiSafe(episodes[i].EpisodeNumber) < atoiSafe(episodes[j].EpisodeNumber)
	})
}

// lookmovieValidateStream confirms the stream URL serves an HLS manifest.
func lookmovieValidateStream(ctx context.Context, client *http.Client, streamURL string) error {
	body, err := lookmovieFetch(ctx, client, streamURL, 2<<20)
	if err != nil {
		return fmt.Errorf("lookmovie: stream unreachable: %w", err)
	}
	if !strings.HasPrefix(string(body), "#EXTM3U") {
		return fmt.Errorf("lookmovie: stream is not an HLS playlist")
	}
	return nil
}

// lookmovieFetch is the hardened HTTP helper: HTTPS only, no local hosts,
// bounded bodies, browser headers.
func lookmovieFetch(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	host := strings.ToLower(u.Hostname())
	if u.Scheme != "https" || u.User != nil || host == "" || !strings.Contains(host, ".") || net.ParseIP(host) != nil ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || host == "localhost" {
		return nil, fmt.Errorf("unsupported URL")
	}

	reqCtx, cancel := context.WithTimeout(ctx, lookmovieHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", LookMovieBaseURL+"/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
