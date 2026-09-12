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
	"strings"
	"time"

	"github.com/MADPANDA3D/pandaflix/core"
)

// VixSrcBaseURL is the site origin for API, embed, and playlist requests.
const VixSrcBaseURL = "https://vixsrc.to"

const (
	vixsrcHTTPTimeout = 15 * time.Second
	vixsrcResolveTTL  = 45 * time.Second
)

// VixSrc is a browser-free provider backed by VixSrc's public API: TMDB ids map
// to an embed token, the embed page names the master playlist, and the playlist
// endpoint (with the required h=1 flag) returns the HLS master.
type VixSrc struct {
	Client *http.Client
}

func NewVixSrc(client *http.Client) *VixSrc {
	return &VixSrc{Client: client}
}

// --- Provider interface -----------------------------------------------------

func (v *VixSrc) Search(query string) ([]core.SearchResult, error) {
	return tmdbSearch(v.Client, query, VixSrcBaseURL)
}

func (v *VixSrc) GetMediaID(mediaURL string) (string, error) {
	r, err := tmdbMediaFromURL(mediaURL)
	if err != nil {
		return "", err
	}
	return tmdbMediaID(r), nil
}

func (v *VixSrc) GetSeasons(mediaID string) ([]core.Season, error) {
	show, err := parseTMDBMediaID(mediaID)
	if err != nil {
		return nil, err
	}
	if show.kind != "series" {
		return nil, nil
	}
	return tmdbSeasons(v.Client, show)
}

func (v *VixSrc) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	if !isSeason {
		// Movie path (recommendation flow): the episode list IS the server list.
		return []core.Episode{{ID: id, Name: "VixSrc"}}, nil
	}

	season, err := parseTMDBMediaID(id)
	if err != nil {
		return nil, err
	}
	if season.kind != "series" || season.season == "" {
		return nil, fmt.Errorf("invalid vixsrc season ID")
	}
	return tmdbEpisodes(v.Client, season)
}

func (v *VixSrc) GetServers(id string) ([]core.Server, error) {
	// Resolution goes straight to the VixSrc master playlist; the caller ID
	// carries the full request identity.
	return []core.Server{{ID: id, Name: "VixSrc (Auto)"}}, nil
}

func (v *VixSrc) GetLink(serverID string) (string, error) {
	req, err := parseTMDBMediaID(serverID)
	if err != nil {
		return "", err
	}
	if req.kind == "series" && (req.season == "" || req.episode == "") {
		return "", fmt.Errorf("vixsrc: series resolution requires season and episode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), vixsrcResolveTTL)
	defer cancel()

	embedURL, err := v.vixsrcEmbedURL(ctx, req)
	if err != nil {
		return "", err
	}
	embedded, err := vixsrcFetch(ctx, v.Client, embedURL, 4<<20)
	if err != nil {
		return "", fmt.Errorf("vixsrc: embed page: %w", err)
	}
	playlistURL, token, expires, err := vixsrcParseMasterPlaylist(embedded)
	if err != nil {
		return "", err
	}

	master := vixsrcMasterURL(playlistURL, token, expires)
	if err := vixsrcValidateMaster(ctx, v.Client, master); err != nil {
		return "", err
	}
	return master, nil
}

func (v *VixSrc) vixsrcEmbedURL(ctx context.Context, req tmdbRequest) (string, error) {
	apiURL := fmt.Sprintf("%s/api/movie/%s", VixSrcBaseURL, req.tmdb)
	if req.kind == "series" {
		apiURL = fmt.Sprintf("%s/api/tv/%s/%s/%s", VixSrcBaseURL, req.tmdb, req.season, req.episode)
	}
	body, err := vixsrcFetch(ctx, v.Client, apiURL, 1<<20)
	if err != nil {
		return "", fmt.Errorf("vixsrc: api: %w", err)
	}
	src, err := vixsrcParseAPISrc(body)
	if err != nil {
		return "", err
	}
	return VixSrcBaseURL + src, nil
}

// --- Protocol helpers (pure, unit-tested) ----------------------------------

func vixsrcParseAPISrc(body []byte) (string, error) {
	var data struct {
		Src string `json:"src"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("vixsrc: invalid api response")
	}
	if data.Src == "" || !strings.HasPrefix(data.Src, "/embed/") {
		return "", fmt.Errorf("vixsrc: unexpected embed path")
	}
	return data.Src, nil
}

var (
	vixsrcPlaylistRE = regexp.MustCompile(`url:\s*'(https://vixsrc\.to/playlist/[0-9]+)'`)
	vixsrcTokenRE    = regexp.MustCompile(`'token':\s*'([^']+)'`)
	vixsrcExpiresRE  = regexp.MustCompile(`'expires':\s*'([^']+)'`)
)

func vixsrcParseMasterPlaylist(html []byte) (playlistURL, token, expires string, err error) {
	pMatch := vixsrcPlaylistRE.FindSubmatch(html)
	tMatch := vixsrcTokenRE.FindSubmatch(html)
	eMatch := vixsrcExpiresRE.FindSubmatch(html)
	if len(pMatch) < 2 || len(tMatch) < 2 || len(eMatch) < 2 {
		return "", "", "", fmt.Errorf("vixsrc: master playlist metadata not found")
	}
	return string(pMatch[1]), string(tMatch[1]), string(eMatch[1]), nil
}

// vixsrcMasterURL builds the playlist request URL. The h=1 flag is required;
// without it the endpoint answers 403.
func vixsrcMasterURL(playlistURL, token, expires string) string {
	params := url.Values{}
	params.Set("token", token)
	params.Set("expires", expires)
	params.Set("asn", "")
	params.Set("h", "1")
	return playlistURL + "?" + params.Encode()
}

// vixsrcFirstVariant returns the first video variant URI in an HLS master.
func vixsrcFirstVariant(master string) (string, error) {
	lines := strings.Split(master, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#EXT-X-STREAM-INF:") {
			continue
		}
		for _, next := range lines[i+1:] {
			next = strings.TrimSpace(next)
			if next == "" {
				continue
			}
			if strings.HasPrefix(next, "#") {
				break
			}
			if strings.HasPrefix(next, "https://") {
				return next, nil
			}
			break
		}
	}
	return "", fmt.Errorf("vixsrc: no video variant found")
}

// vixsrcValidateMaster fetches the master and its first variant playlist.
func vixsrcValidateMaster(ctx context.Context, client *http.Client, masterURL string) error {
	body, err := vixsrcFetch(ctx, client, masterURL, 2<<20)
	if err != nil {
		return fmt.Errorf("vixsrc: master playlist: %w", err)
	}
	if !strings.HasPrefix(string(body), "#EXTM3U") {
		return fmt.Errorf("vixsrc: master is not an HLS playlist")
	}
	variant, err := vixsrcFirstVariant(string(body))
	if err != nil {
		return err
	}
	variantBody, err := vixsrcFetch(ctx, client, variant, 2<<20)
	if err != nil {
		return fmt.Errorf("vixsrc: variant playlist: %w", err)
	}
	if !strings.HasPrefix(string(variantBody), "#EXTM3U") {
		return fmt.Errorf("vixsrc: variant is not an HLS playlist")
	}
	return nil
}

// vixsrcFetch is the hardened HTTP helper for VixSrc endpoints: HTTPS only,
// no local hosts, bounded bodies, browser headers.
func vixsrcFetch(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	host := strings.ToLower(u.Hostname())
	if u.Scheme != "https" || u.User != nil || host == "" || !strings.Contains(host, ".") || net.ParseIP(host) != nil ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || host == "localhost" {
		return nil, fmt.Errorf("unsupported URL")
	}

	reqCtx, cancel := context.WithTimeout(ctx, vixsrcHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", VixSrcBaseURL+"/")

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
