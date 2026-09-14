package providers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/MADPANDA3D/pandaflix/core"
)

func TestLookMovieIDRoundTrip(t *testing.T) {
	movie := lookmovieID(lookmovieRequest{kind: "movie", slug: "0089218-the-goonies-1985"})
	if movie != "lookmovie|movie|0089218-the-goonies-1985" {
		t.Fatalf("unexpected movie ID: %q", movie)
	}
	parsed, err := parseLookMovieID(movie)
	if err != nil || parsed.kind != "movie" || parsed.slug != "0089218-the-goonies-1985" {
		t.Fatalf("movie parse failed: %+v err=%v", parsed, err)
	}

	series := lookmovieID(lookmovieRequest{kind: "series", slug: "0903747-breaking-bad-2008", season: "1", episode: "1"})
	if series != "lookmovie|series|0903747-breaking-bad-2008|1|1" {
		t.Fatalf("unexpected series ID: %q", series)
	}
	parsed, err = parseLookMovieID(series)
	if err != nil || parsed.season != "1" || parsed.episode != "1" {
		t.Fatalf("series parse failed: %+v err=%v", parsed, err)
	}

	for _, bad := range []string{"", "lookmovie", "lookmovie|movie", "lookmovie|show|x", "other|movie|x"} {
		if _, err := parseLookMovieID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestParseLookMovieSearch(t *testing.T) {
	body := []byte(`{"total":2,"result":[
		{"id_movie":2686,"slug":"0089218-the-goonies-1985","title":"The Goonies","year":1985},
		{"id_movie":1,"slug":"x","title":"","year":"1999"}
	]}`)
	results := parseLookMovieSearch(body, "movies", core.Movie)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Title != "The Goonies" || r.Year != "1985" || !strings.Contains(r.URL, "/movies/view/0089218-the-goonies-1985") {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestParseLookMoviePages(t *testing.T) {
	movieHTML := `<a class="play" href="/movies/play/1689845921-the-goonies-1985">play</a>`
	path, err := parseLookMoviePlayLink(movieHTML, "movie")
	if err != nil || path != "/movies/play/1689845921-the-goonies-1985" {
		t.Fatalf("movie play link: %q err=%v", path, err)
	}

	showHTML := `<a href="/shows/play/1690660631-breaking-bad-2008">play</a>`
	path, err = parseLookMoviePlayLink(showHTML, "series")
	if err != nil || path != "/shows/play/1690660631-breaking-bad-2008" {
		t.Fatalf("show play link: %q err=%v", path, err)
	}

	playHTML := `window.movie_storage = { hash: "RAMtebmx57Z4CT6su7mDqg", expires: 1789438027 }; id_movie: 2686`
	hash, expires, err := parseLookMovieStorage(playHTML)
	if err != nil || hash != "RAMtebmx57Z4CT6su7mDqg" || expires != "1789438027" {
		t.Fatalf("storage parse: %q %q err=%v", hash, expires, err)
	}
	if id := parseLookMovieMovieID(playHTML); id != "2686" {
		t.Fatalf("movie id parse: %q", id)
	}
}

func TestParseLookMovieSeasons(t *testing.T) {
	html := `window.seasons='{\"1\":{\"episodes\":{\"1\":{\"episode_number\":\"1\",\"id_episode\":\"35691\",\"title\":\"Pilot\"},\"2\":{\"episode_number\":\"2\",\"id_episode\":\"35701\",\"title\":\"Cat\'s in the Bag...\"}}},\"2\":{\"episodes\":{\"1\":{\"episode_number\":\"1\",\"id_episode\":\"35801\",\"title\":\"Seven Thirty-Seven\"}}}}';`
	seasons, err := parseLookMovieSeasons(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(seasons) != 2 || len(seasons["1"]) != 2 {
		t.Fatalf("unexpected seasons: %+v", seasons)
	}
	eps := sortedEpisodes(seasons["1"])
	if eps[0].IDEpisode != "35691" || eps[1].IDEpisode != "35701" {
		t.Fatalf("episode ordering wrong: %+v", eps)
	}
	if eps[1].Title != "Cat's in the Bag..." {
		t.Fatalf("escaped apostrophe not decoded: %q", eps[1].Title)
	}
	if _, err := parseLookMovieSeasons("<html></html>"); err == nil {
		t.Fatal("missing season data accepted")
	}
}

func TestUnescapeJSString(t *testing.T) {
	// JSON text inside the JS literal: quotes are escaped once for JS, and
	// text quotes are escaped once more for JSON.
	src := `{\"title\":\"He said \\\"hello\\\"\"}`
	got := unescapeJSString(src)
	want := `{"title":"He said \"hello\""}`
	if got != want {
		t.Fatalf("unescape mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestParseLookMovieAccess(t *testing.T) {
	body := []byte(`{"success":true,"streams":{"1080p":null,"720p":"https://cdn.example/hd.m3u8","480p":"https://cdn.example/sd.m3u8"},
		"subtitles":[{"language":"English","file":"/storage4/movies/x/subtitles/en.vtt","kind":"captions"},{"language":"Danish","file":"/storage4/movies/x/subtitles/da.vtt","kind":"captions"}]}`)
	stream, subs, err := parseLookMovieAccess(body)
	if err != nil {
		t.Fatal(err)
	}
	if stream != "https://cdn.example/hd.m3u8" {
		t.Fatalf("best stream not chosen: %q", stream)
	}
	if len(subs) != 1 || !strings.HasSuffix(subs[0], "/storage4/movies/x/subtitles/en.vtt") {
		t.Fatalf("english subtitle not chosen: %v", subs)
	}

	none := []byte(`{"success":true,"streams":{"1080p":null},"subtitles":[]}`)
	if _, _, err := parseLookMovieAccess(none); err == nil {
		t.Fatal("expected error when no stream available")
	}
	denied := []byte(`{"success":false,"streams":{},"subtitles":[]}`)
	if _, _, err := parseLookMovieAccess(denied); err == nil {
		t.Fatal("expected error for denied access")
	}
}

func TestDecodeStringOrArray(t *testing.T) {
	if got := decodeStringOrArray([]byte(`"/x.vtt"`)); len(got) != 1 || got[0] != "/x.vtt" {
		t.Fatalf("string form failed: %v", got)
	}
	if got := decodeStringOrArray([]byte(`["/a.vtt","/b.vtt"]`)); len(got) != 2 || got[1] != "/b.vtt" {
		t.Fatalf("array form failed: %v", got)
	}
	if got := decodeStringOrArray([]byte(`42`)); got != nil {
		t.Fatalf("unexpected decode: %v", got)
	}
	if got := decodeStringOrArray(nil); got != nil {
		t.Fatalf("unexpected decode: %v", got)
	}
}

func TestLookMovieFetchURLGuards(t *testing.T) {
	client := &http.Client{}
	for _, bad := range []string{
		"http://www.lookmovie2.to/x",
		"https://localhost/x",
		"https://127.0.0.1/x",
		"https://x.local/x",
		"ftp://www.lookmovie2.to/x",
	} {
		if _, err := lookmovieFetch(context.Background(), client, bad, 1024); err == nil {
			t.Errorf("URL %q was accepted", bad)
		}
	}
}
