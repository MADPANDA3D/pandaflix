// Command cinejoycheck exercises the movie/TV providers end-to-end without the
// interactive fzf UI: TMDB search, ID resolution, season/episode lists, and
// GetLink resolution, with an optional bounded mpv decode proof.
//
// Usage (from the app checkout):
//
//	go run ./docs/verification/cinejoycheck -q "the goonies"
//	go run ./docs/verification/cinejoycheck -provider vixsrc -q "breaking bad" -episode 1 -play
//
// -play requires mpv and decodes three seconds with null audio/video outputs.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/MADPANDA3D/pandaflix/core"
	"github.com/MADPANDA3D/pandaflix/core/providers"
)

const userAgent = "Mozilla/5.0"

func main() {
	query := flag.String("q", "", "search query")
	tmdb := flag.String("tmdb", "", "resolve a movie by TMDB id only (skips search)")
	episode := flag.Int("episode", 1, "episode number to resolve for series")
	providerName := flag.String("provider", "cinejoy", "provider to exercise: cinejoy|vixsrc|fallback")
	play := flag.Bool("play", false, "run 3s headless mpv decode proof")
	download := flag.Bool("download", false, "download the resolved stream (bound the run with an external timeout)")
	dump := flag.Bool("dump", false, "print the master playlist and its first variant, then exit")
	playCLI := flag.Bool("play-cli", false, "run mpv with the same invocation the CLI uses")
	flag.Parse()
	if *query == "" && *tmdb == "" {
		fmt.Fprintln(os.Stderr, "cinejoycheck: -q or -tmdb is required")
		os.Exit(2)
	}

	client := core.NewClient()
	base := providers.CinejoyBaseURL
	var p core.Provider
	switch strings.ToLower(*providerName) {
	case "vixsrc":
		p = providers.NewVixSrc(client)
		base = providers.VixSrcBaseURL
	case "fallback":
		p = providers.NewFallback(providers.NewCinejoy(client), providers.NewVixSrc(client), "Cinejoy", "VixSrc")
	default:
		p = providers.NewCinejoy(client)
	}
	alang := "eng,en"

	requestID := ""
	if *tmdb != "" {
		requestID = "movie|" + *tmdb + "||"
		fmt.Printf("resolving movie by tmdb id only: %s\n", *tmdb)
	} else {
		results, err := p.Search(*query)
		if err != nil {
			fatal("search", err)
		}
		var picked *core.SearchResult
		for i := range results {
			if strings.EqualFold(results[i].Title, *query) && results[i].Type == core.Movie {
				picked = &results[i]
				break
			}
		}
		if picked == nil {
			picked = &results[0]
		}
		fmt.Printf("picked: %s (%s) type=%s\n", picked.Title, picked.Year, picked.Type)

		mediaID, err := p.GetMediaID(picked.URL)
		if err != nil {
			fatal("GetMediaID", err)
		}

		if picked.Type == core.Series {
			seasons, err := p.GetSeasons(mediaID)
			if err != nil {
				fatal("GetSeasons", err)
			}
			if len(seasons) == 0 {
				fatal("GetSeasons", fmt.Errorf("no seasons"))
			}
			fmt.Printf("seasons: %d (first: %s)\n", len(seasons), seasons[0].Name)
			episodes, err := p.GetEpisodes(seasons[0].ID, true)
			if err != nil {
				fatal("GetEpisodes", err)
			}
			if *episode < 1 || *episode > len(episodes) {
				fatal("episode", fmt.Errorf("%d out of range (max %d)", *episode, len(episodes)))
			}
			fmt.Printf("episodes: %d (first: %s)\n", len(episodes), episodes[0].Name)
			requestID = episodes[*episode-1].ID
		} else {
			servers, err := p.GetServers(mediaID)
			if err != nil || len(servers) == 0 {
				fatal("GetServers", fmt.Errorf("no servers: %w", err))
			}
			requestID = servers[0].ID
		}
	}

	start := time.Now()
	streamURL, err := p.GetLink(requestID)
	if err != nil {
		fatal("GetLink", err)
	}
	fmt.Printf("resolved in %s: %s\n", time.Since(start).Round(time.Millisecond), redact(streamURL))

	if *dump {
		dumpPlaylists(streamURL, base)
		os.Exit(0)
	}
	if *download {
		dir, err := os.MkdirTemp("", "pandaflix-dl-")
		if err != nil {
			fatal("download", err)
		}
		fmt.Printf("download test dir: %s\n", dir)
		if err := core.Download(dir, dir, "download-test", streamURL, base+"/", userAgent, nil, true); err != nil {
			fatal("download", err)
		}
		fmt.Println("OK (download completed)")
		os.Exit(0)
	}
	if *playCLI {
		if err := cliMpvProof(streamURL, base, alang); err != nil {
			fatal("cli playback", err)
		}
		fmt.Println("OK (CLI-style mpv exited normally)")
		os.Exit(0)
	}
	if !*play {
		fmt.Println("OK (resolution only; pass -play for the decode proof)")
		os.Exit(0)
	}

	if err := decodeProof(streamURL, base, alang); err != nil {
		fatal("playback", err)
	}
	fmt.Println("OK (mpv decoded 3s of video and audio)")
	os.Exit(0)
}

func dumpPlaylists(masterURL, base string) {
	fetch := func(u string) string {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			fatal("dump", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Referer", base+"/")
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			fatal("dump", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		fmt.Printf("--- HTTP %d for %s\n", resp.StatusCode, redact(u))
		return string(body)
	}

	master := fetch(masterURL)
	fmt.Println(master)
	var variant string
	lines := strings.Split(master, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXT-X-STREAM-INF:") {
			for _, next := range lines[i+1:] {
				next = strings.TrimSpace(next)
				if next == "" {
					continue
				}
				if !strings.HasPrefix(next, "#") {
					variant = next
				}
				break
			}
			break
		}
	}
	if variant != "" {
		fmt.Println("=== first variant ===")
		fmt.Println(fetch(variant))
	}
}

func cliMpvProof(streamURL, base, alang string) error {
	// Mirror the CLI behavior: split provider suffixes (|referer=, |subs=)
	// like cmd.resolveStreamURL does, then hand everything to mpv.
	streamURL, referer, subs := splitStreamSuffixes(streamURL, base)
	fmt.Printf("quality stage: skipped (master passthrough)\n")

	// Mirror buildPlayerCmd's default (non-darwin, non-vlc) mpv invocation.
	args := []string{
		streamURL,
		"--referrer=" + referer,
		"--user-agent=" + userAgent,
		"--force-media-title=Playing CLI replication",
	}
	for _, sub := range subs {
		args = append(args, "--sub-file="+sub)
	}
	args = append(args, "--input-ipc-server=/tmp/cinejoycheck-mpv.sock")
	os.Remove("/tmp/cinejoycheck-mpv.sock")
	args = append(args, "--http-header-fields=Origin: "+base)
	args = append(args, "--alang="+alang)

	cmd := exec.Command("mpv", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	timer := time.AfterFunc(60*time.Second, func() {
		cmd.Process.Kill() //nolint:errcheck
	})
	err := cmd.Wait()
	timer.Stop()
	os.Remove("/tmp/cinejoycheck-mpv.sock")
	return err
}

func decodeProof(streamURL, base, alang string) error {
	streamURL, referer, subs := splitStreamSuffixes(streamURL, base)
	args := []string{"--no-config", "--vo=null", "--ao=null", "--length=3",
		"--network-timeout=10", "--ytdl=no",
		"--user-agent=" + userAgent,
		"--referrer=" + referer,
		"--http-header-fields=Origin: " + base,
		"--alang=" + alang,
		"--term-status-msg=PROBE time=${time-pos}"}
	for _, sub := range subs {
		args = append(args, "--sub-file="+sub)
	}
	args = append(args, streamURL)
	cmd := exec.Command("mpv", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	// Hard bailout in case mpv hangs.
	timer := time.AfterFunc(45*time.Second, func() {
		cmd.Process.Kill() //nolint:errcheck
	})
	err := cmd.Wait()
	timer.Stop()
	return err
}

// splitStreamSuffixes mimics cmd.resolveStreamURL's handling of the
// |referer= and |subs= suffixes some providers append to stream URLs.
func splitStreamSuffixes(streamURL, base string) (string, string, []string) {
	referer := base + "/"
	var subs []string
	if idx := strings.Index(streamURL, "|referer="); idx != -1 {
		value := streamURL[idx+9:]
		streamURL = streamURL[:idx]
		if next := strings.Index(value, "|"); next != -1 {
			streamURL += value[next:]
			value = value[:next]
		}
		if decoded, err := url.QueryUnescape(value); err == nil && decoded != "" {
			referer = decoded
		}
	}
	if idx := strings.Index(streamURL, "|subs="); idx != -1 {
		value := streamURL[idx+6:]
		streamURL = streamURL[:idx]
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		for _, sub := range strings.Split(value, ",") {
			if sub = strings.TrimSpace(sub); sub != "" {
				subs = append(subs, sub)
			}
		}
	}
	return streamURL, referer, subs
}

func redact(raw string) string {
	if len(raw) < 90 {
		return raw
	}
	return raw[:45] + "..." + raw[len(raw)-20:]
}

func fatal(stage string, err error) {
	fmt.Fprintf(os.Stderr, "%s failed: %v\n", stage, err)
	os.Exit(1)
}
