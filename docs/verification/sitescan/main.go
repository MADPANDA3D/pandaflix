// Command sitescan surveys streaming-site candidates (e.g. the FMHY /video
// list) and classifies them by the resolver/protocol signatures found in their
// HTML and JavaScript bundles. It performs read-only GETs with a browser UA and
// writes a markdown report plus optional JSON.
//
// Usage (from the app checkout):
//
//	go run ./docs/verification/sitescan -md ../../docs/SITE-SURVEY.md
//	go run ./docs/verification/sitescan -urls mylist.txt -concurrency 6
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// defaultSites is the candidate pool transcribed from the FMHY /video index.
var defaultSites = []string{
	"https://cinejoy.to", "https://www.movy.sx", "https://popcornmovies.ac", "https://bingebox.ac",
	"https://zstream.mov", "https://aether.ist", "https://kdesa.stream", "https://basementx.lol",
	"https://cinefork.net", "https://pstream.cfd", "https://rizzking.org", "https://streamwatch.online",
	"https://cinevaro.app", "https://fmfau.com", "https://icefy.top", "https://peestream.in",
	"https://www.rivestream.app", "https://watch.corsflix.net", "https://flixer.gd", "https://hexa.su",
	"https://bcine.ru", "https://www.bingey.cfd", "https://7movies.in", "https://shuttletv.su",
	"https://stellar.gdn", "https://67movies.st", "https://phantomflix.net", "https://meowtv.ru",
	"https://flickystream.mov", "https://reelix.ac", "https://coreflix.tv", "https://arrowtv.net",
	"https://neonflix.st", "https://www.cinezo.org", "https://www.flikhub.net", "https://vivarium.wtf",
	"https://movienig.ht", "https://chillflix.lol", "https://svstream.cc", "https://beta.way2movies.live",
	"https://cinema.army", "https://streamo.pro", "https://moovie.fun", "https://streamaggregator.in",
	"https://watch.spencerdevs.xyz", "https://moviebite.org", "https://tonkacine.watch", "https://cinetaro.to",
	"https://cinemaos.live", "https://www.noirx.me", "https://vuflix.co", "https://opstream.fun",
	"https://movish.to", "https://latestmovies.net", "https://vidplay.to", "https://moonflix.website",
	"https://cinegram.tv", "https://www.framemovie.online", "https://spacedom.live", "https://flixtrz.com",
	"https://zxcprime.icu", "https://cineby.gdn", "https://smovies.co", "https://fstream.app",
	"https://watchott.org", "https://nextbox.uno", "https://novera.tv", "https://novahd.cc",
	"https://netplayz.icu", "https://cinelove.live", "https://screenscape.me", "https://dulo.cx",
	"https://kofi.mov", "https://popwatch.to", "https://watchsurface.stream", "https://mappl.tv",
	"https://apexmovies.net", "https://averotv.top", "https://streamvaults.ru", "https://gaiaflix.live",
}

// signatures maps a classification label to the patterns that imply it.
var signatures = []struct {
	label string
	res   *regexp.Regexp
}{
	{"cloudflare-challenge", regexp.MustCompile(`(?i)cdn-cgi/challenge-platform|challenges\.cloudflare\.com|turnstile`)},
	{"pstream-fork", regexp.MustCompile(`(?i)pstream|p-stream|movie-web`)},
	{"vidsrc-family", regexp.MustCompile(`(?i)vidsrc|vsembed|cloudnestra|2embed`)},
	{"vixsrc", regexp.MustCompile(`(?i)vixsrc`)},
	{"videasy", regexp.MustCompile(`(?i)videasy`)},
	{"vidking", regexp.MustCompile(`(?i)vidking`)},
	{"vidlink", regexp.MustCompile(`(?i)vidlink`)},
	{"megacloud", regexp.MustCompile(`(?i)megacloud|vidcloud|videostr|streameeeeee`)},
	{"filemoon", regexp.MustCompile(`(?i)filemoon`)},
	{"voe", regexp.MustCompile(`(?i)voe\.sx|voe\.to`)},
	{"streamtape", regexp.MustCompile(`(?i)streamtape`)},
	{"dood", regexp.MustCompile(`(?i)doodstream|dood\.`)},
	{"hydrahd", regexp.MustCompile(`(?i)hydrahd`)},
	{"primesrc", regexp.MustCompile(`(?i)primesrc`)},
	{"watchseries-style", regexp.MustCompile(`(?i)watchseries|watch-tvseries|tvids`)},
	{"hls-direct", regexp.MustCompile(`\.m3u8`)},
}

var apiHostRE = regexp.MustCompile(`https?://[a-zA-Z0-9.-]*(?:api|backend|worker)[a-zA-Z0-9.-]*\.[a-zA-Z]{2,}`)
var scriptRE = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
var titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

type siteResult struct {
	URL        string   `json:"url"`
	Status     int      `json:"status"`
	Title      string   `json:"title,omitempty"`
	Size       int      `json:"size"`
	Signatures []string `json:"signatures"`
	APIHosts   []string `json:"api_hosts,omitempty"`
	Notes      string   `json:"notes,omitempty"`
}

var client = &http.Client{
	Timeout: 20 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

func fetch(rawURL string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return body, resp.StatusCode, err
}

func scanSite(site string) siteResult {
	res := siteResult{URL: site}
	body, status, err := fetch(site, 3<<20)
	res.Status = status
	if err != nil {
		res.Notes = err.Error()
		return res
	}
	res.Size = len(body)
	if m := titleRE.FindSubmatch(body); len(m) > 1 {
		res.Title = strings.TrimSpace(string(m[1]))
		if len(res.Title) > 60 {
			res.Title = res.Title[:60]
		}
	}

	combined := append([]byte{}, body...)
	// Pull a bounded sample of script bundles for deeper signature matching.
	scripts := scriptRE.FindAllSubmatch(body, 6)
	for _, m := range scripts {
		src := string(m[1])
		if strings.HasPrefix(src, "/") {
			if u, perr := url.Parse(site); perr == nil {
				src = u.Scheme + "://" + u.Host + src
			}
		}
		if !strings.HasPrefix(src, "http") {
			continue
		}
		js, _, jerr := fetch(src, 1<<20)
		if jerr != nil {
			continue
		}
		combined = append(combined, js...)
		if len(combined) > 8<<20 {
			break
		}
	}

	seen := map[string]bool{}
	for _, sig := range signatures {
		if sig.res.Match(combined) && !seen[sig.label] {
			seen[sig.label] = true
			res.Signatures = append(res.Signatures, sig.label)
		}
	}
	sort.Strings(res.Signatures)

	hosts := map[string]bool{}
	for _, m := range apiHostRE.FindAll(combined, -1) {
		h := string(m)
		if strings.Contains(h, "themoviedb") || strings.Contains(h, "w3.org") {
			continue
		}
		hosts[h] = true
	}
	for h := range hosts {
		res.APIHosts = append(res.APIHosts, h)
	}
	sort.Strings(res.APIHosts)
	if len(res.APIHosts) > 3 {
		res.APIHosts = res.APIHosts[:3]
	}
	return res
}

func main() {
	urlsFile := flag.String("urls", "", "optional file with one URL per line (defaults to the built-in FMHY list)")
	mdOut := flag.String("md", "", "write a markdown report to this path")
	jsonOut := flag.String("json", "", "write JSON results to this path")
	concurrency := flag.Int("concurrency", 5, "parallel site fetches")
	timeout := flag.Duration("timeout", 4*time.Minute, "overall time budget")
	flag.Parse()

	sites := defaultSites
	if *urlsFile != "" {
		data, err := os.ReadFile(*urlsFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sitescan: %v\n", err)
			os.Exit(2)
		}
		sites = nil
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				sites = append(sites, line)
			}
		}
	}

	fmt.Printf("scanning %d sites (concurrency %d)...\n", len(sites), *concurrency)
	deadline := time.Now().Add(*timeout)

	results := make([]siteResult, len(sites))
	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup
	for i, site := range sites {
		wg.Add(1)
		go func(i int, site string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if time.Now().After(deadline) {
				results[i] = siteResult{URL: site, Notes: "skipped: time budget exceeded"}
				return
			}
			results[i] = scanSite(site)
		}(i, site)
	}
	wg.Wait()

	// Cluster summary.
	clusterCount := map[string]int{}
	for _, r := range results {
		for _, s := range r.Signatures {
			clusterCount[s]++
		}
	}
	var clusters []string
	for label, count := range clusterCount {
		clusters = append(clusters, fmt.Sprintf("%s (%d)", label, count))
	}
	sort.Strings(clusters)
	fmt.Printf("clusters: %s\n", strings.Join(clusters, ", "))

	if *jsonOut != "" {
		data, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(*jsonOut, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "sitescan: %v\n", err)
		}
	}
	if *mdOut != "" {
		if err := os.WriteFile(*mdOut, []byte(report(results, clusters)), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "sitescan: %v\n", err)
		}
		fmt.Printf("report: %s\n", *mdOut)
	}
}

func report(results []siteResult, clusters []string) string {
	var b strings.Builder
	b.WriteString("# Streaming-site survey\n\n")
	b.WriteString("Generated by `go run ./docs/verification/sitescan` (read-only GETs, browser UA, bounded bundles).\n\n")
	b.WriteString("## Cluster summary\n\n")
	for _, c := range clusters {
		b.WriteString("- " + c + "\n")
	}
	b.WriteString("\n## Sites\n\n")
	b.WriteString("| Site | Status | Signatures | API hosts | Notes |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, r := range results {
		sigs := strings.Join(r.Signatures, ", ")
		if sigs == "" {
			sigs = "-"
		}
		hosts := strings.Join(r.APIHosts, ", ")
		if hosts == "" {
			hosts = "-"
		}
		notes := r.Notes
		if notes != "" {
			notes = strings.ReplaceAll(notes, "|", "/")
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n", r.URL, r.Status, sigs, hosts, notes)
	}
	b.WriteString("\n_Signature detection is heuristic; a cluster match is a worklist hint, not verification._\n")
	return b.String()
}
