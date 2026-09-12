package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/MADPANDA3D/pandaflix/core"
)

// tmdbRequest is the provider-independent request identity carried through the
// Provider interface for TMDB-backed providers (Cinejoy, VixSrc, ...):
//
//	movie:  movie|<tmdb>|<title>|<year>
//	series: series|<tmdb>|<season>|<episode>|<title>|<year>
//
// Pipe characters are removed from free-text parts so the encoding stays
// unambiguous. Show-level series IDs (series|<tmdb>|<title>|<year>) are also
// accepted where only the show identity is needed (GetSeasons).
type tmdbRequest struct {
	kind    string // movie | series
	tmdb    string
	season  string
	episode string
	title   string
	year    string
}

func tmdbMediaID(r tmdbRequest) string {
	sanitize := func(s string) string {
		return strings.ReplaceAll(s, "|", "-")
	}
	if r.kind == "series" {
		return strings.Join([]string{"series", r.tmdb, r.season, r.episode, sanitize(r.title), r.year}, "|")
	}
	return strings.Join([]string{"movie", r.tmdb, sanitize(r.title), r.year}, "|")
}

func parseTMDBMediaID(id string) (r tmdbRequest, err error) {
	parts := strings.Split(id, "|")
	if len(parts) < 2 {
		return r, fmt.Errorf("invalid media ID")
	}
	r.kind = parts[0]
	r.tmdb = parts[1]
	if r.tmdb == "" || !(r.kind == "movie" || r.kind == "series") {
		return r, fmt.Errorf("invalid media ID")
	}
	switch r.kind {
	case "movie":
		if len(parts) > 2 {
			r.title = parts[2]
		}
		if len(parts) > 3 {
			r.year = parts[3]
		}
	case "series":
		// Series IDs come in two shapes:
		//   show:  series|<tmdb>|<title>|<year>            (GetSeasons input)
		//   ep:    series|<tmdb>|<season>|<episode>|<title>|<year>
		// Season/episode presence is enforced by resolve/GetEpisodes.
		if len(parts) > 2 {
			r.season = parts[2]
		}
		if len(parts) > 3 {
			r.episode = parts[3]
		}
		if len(parts) > 4 {
			r.title = parts[4]
		}
		if len(parts) > 5 {
			r.year = parts[5]
		}
	}
	return r, nil
}

// tmdbMediaFromURL parses the "/{movie|tv}/{id}?title=&year=" URL shape shared
// by the TMDB-backed providers.
func tmdbMediaFromURL(mediaURL string) (tmdbRequest, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return tmdbRequest{}, err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return tmdbRequest{}, fmt.Errorf("invalid provider URL")
	}
	kind := parts[0]
	if kind == "tv" {
		kind = "series"
	} else if kind != "movie" {
		return tmdbRequest{}, fmt.Errorf("invalid provider URL")
	}
	return tmdbRequest{
		kind:  kind,
		tmdb:  parts[1],
		title: u.Query().Get("title"),
		year:  u.Query().Get("year"),
	}, nil
}

// tmdbSearch searches TMDB multi and maps the results into provider URLs
// rooted at baseURL.
func tmdbSearch(client *http.Client, query, baseURL string) ([]core.SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("include_adult", "false")
	params.Set("language", "en-US")
	params.Set("page", "1")
	params.Set("api_key", core.TMDB_API_KEY)

	req, err := core.NewRequest("GET", fmt.Sprintf("%s/search/multi?%s", core.TMDB_BASE_URL, params.Encode()))
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var results []core.SearchResult
	for _, r := range data.Results {
		if r.MediaType != "movie" && r.MediaType != "tv" {
			continue
		}
		title := r.Title
		if title == "" {
			title = r.Name
		}
		year := r.ReleaseDate
		if year == "" {
			year = r.FirstAirDate
		}
		if len(year) > 4 {
			year = year[:4]
		}
		mediaType := core.Movie
		if r.MediaType == "tv" {
			mediaType = core.Series
		}
		poster := ""
		if r.PosterPath != "" {
			poster = core.TMDB_IMAGE_BASE_URL + r.PosterPath
		}
		results = append(results, core.SearchResult{
			Title:  title,
			URL:    fmt.Sprintf("%s/%s/%d?title=%s&year=%s", baseURL, r.MediaType, r.ID, url.QueryEscape(title), year),
			Type:   mediaType,
			Poster: poster,
			Year:   year,
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results")
	}
	return results, nil
}

// tmdbSeasons lists a show's seasons, encoding season IDs via tmdbMediaID.
func tmdbSeasons(client *http.Client, show tmdbRequest) ([]core.Season, error) {
	req, err := core.NewRequest("GET", fmt.Sprintf("%s/tv/%s?api_key=%s", core.TMDB_BASE_URL, show.tmdb, core.TMDB_API_KEY))
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbShowDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var seasons []core.Season
	for _, s := range data.Seasons {
		if s.SeasonNumber == 0 {
			continue
		}
		name := s.Name
		if name == "" {
			name = fmt.Sprintf("Season %d", s.SeasonNumber)
		}
		seasons = append(seasons, core.Season{
			ID:   tmdbMediaID(tmdbRequest{kind: "series", tmdb: show.tmdb, season: fmt.Sprintf("%d", s.SeasonNumber), title: show.title, year: show.year}),
			Name: name,
		})
	}
	return seasons, nil
}

// tmdbEpisodes lists a season's episodes, encoding episode IDs via tmdbMediaID.
func tmdbEpisodes(client *http.Client, season tmdbRequest) ([]core.Episode, error) {
	req, err := core.NewRequest("GET", fmt.Sprintf("%s/tv/%s/season/%s?api_key=%s", core.TMDB_BASE_URL, season.tmdb, season.season, core.TMDB_API_KEY))
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbSeasonDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var episodes []core.Episode
	for _, e := range data.Episodes {
		episodes = append(episodes, core.Episode{
			ID:   tmdbMediaID(tmdbRequest{kind: "series", tmdb: season.tmdb, season: season.season, episode: fmt.Sprintf("%d", e.EpisodeNumber), title: season.title, year: season.year}),
			Name: fmt.Sprintf("E%02d - %s", e.EpisodeNumber, e.Name),
		})
	}
	return episodes, nil
}
