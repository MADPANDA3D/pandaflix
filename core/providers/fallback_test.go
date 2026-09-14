package providers

import (
	"fmt"
	"testing"

	"github.com/MADPANDA3D/pandaflix/core"
)

type fakeProvider struct {
	searchResults []core.SearchResult
	searchErr     error
	mediaID       string
	mediaIDErr    error
	seasons       []core.Season
	seasonsErr    error
	episodes      []core.Episode
	episodesErr   error
	servers       []core.Server
	serversErr    error
	link          string
	linkErr       error
	linkCalls     int
}

func (f *fakeProvider) Search(string) ([]core.SearchResult, error) {
	return f.searchResults, f.searchErr
}
func (f *fakeProvider) GetMediaID(string) (string, error) { return f.mediaID, f.mediaIDErr }
func (f *fakeProvider) GetSeasons(string) ([]core.Season, error) {
	return f.seasons, f.seasonsErr
}
func (f *fakeProvider) GetEpisodes(string, bool) ([]core.Episode, error) {
	return f.episodes, f.episodesErr
}
func (f *fakeProvider) GetServers(string) ([]core.Server, error) { return f.servers, f.serversErr }
func (f *fakeProvider) GetLink(string) (string, error) {
	f.linkCalls++
	return f.link, f.linkErr
}

func newTestFallback(providers ...*fakeProvider) *Fallback {
	list := make([]core.Provider, len(providers))
	names := make([]string, len(providers))
	for i, p := range providers {
		list[i] = p
		names[i] = fmt.Sprintf("P%d", i)
	}
	return NewFallback(list, names)
}

func TestFallbackServerList(t *testing.T) {
	f := newTestFallback(&fakeProvider{}, &fakeProvider{}, &fakeProvider{})
	servers, err := f.GetServers("movie|1|Title|2000")
	if err != nil || len(servers) != 3 {
		t.Fatalf("expected 3 entries, got %v err=%v", servers, err)
	}
	if servers[0].ID != "p0|movie|1|Title|2000" || servers[2].ID != "p2|movie|1|Title|2000" {
		t.Fatalf("unexpected IDs: %+v", servers)
	}
	if servers[1].Name != "P1 (Auto)" {
		t.Fatalf("unexpected name: %q", servers[1].Name)
	}

	episodes, err := f.GetEpisodes("movie|1|Title|2000", false)
	if err != nil || len(episodes) != 3 || episodes[1].ID != "p1|movie|1|Title|2000" {
		t.Fatalf("movie server list failed: %+v err=%v", episodes, err)
	}
}

func TestFallbackRouting(t *testing.T) {
	p0 := &fakeProvider{link: "https://p0/x.m3u8"}
	p1 := &fakeProvider{linkErr: fmt.Errorf("p1 down")}
	p2 := &fakeProvider{link: "https://p2/x.m3u8"}
	f := newTestFallback(p0, p1, p2)

	// Chosen provider wins.
	link, err := f.GetLink("p2|movie|1|T|2000")
	if err != nil || link != "https://p2/x.m3u8" {
		t.Fatalf("chosen provider failed: %q err=%v", link, err)
	}

	// Chosen provider fails -> next providers are tried in order.
	p0.linkCalls, p1.linkCalls, p2.linkCalls = 0, 0, 0
	link, err = f.GetLink("p1|movie|1|T|2000")
	if err != nil || link != "https://p0/x.m3u8" {
		t.Fatalf("fallback after chosen failure failed: %q err=%v", link, err)
	}
	if p1.linkCalls != 1 {
		t.Fatalf("chosen provider not tried exactly once: %d", p1.linkCalls)
	}

	// Unprefixed IDs use the default order.
	link, err = f.GetLink("movie|1|T|2000")
	if err != nil || link != "https://p0/x.m3u8" {
		t.Fatalf("default order failed: %q err=%v", link, err)
	}
}

func TestFallbackAllFail(t *testing.T) {
	f := newTestFallback(
		&fakeProvider{linkErr: fmt.Errorf("p0 down")},
		&fakeProvider{linkErr: fmt.Errorf("p1 down")},
	)
	if _, err := f.GetLink("p0|movie|1|T|2000"); err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestFallbackReadOperations(t *testing.T) {
	p0 := &fakeProvider{searchErr: fmt.Errorf("down"), mediaIDErr: fmt.Errorf("down"), seasonsErr: fmt.Errorf("down"), episodesErr: fmt.Errorf("down")}
	p1 := &fakeProvider{searchResults: nil, mediaID: "id", seasons: []core.Season{{ID: "s"}}, episodes: []core.Episode{{ID: "e"}}}
	p2 := &fakeProvider{searchResults: []core.SearchResult{{Title: "T"}}}
	f := newTestFallback(p0, p1, p2)

	if results, err := f.Search("x"); err != nil || len(results) != 1 {
		t.Fatalf("search should use the first non-empty provider: %v err=%v", results, err)
	}
	if id, err := f.GetMediaID("u"); err != nil || id != "id" {
		t.Fatalf("mediaID should use the first accepting provider: %q err=%v", id, err)
	}
	if seasons, err := f.GetSeasons("id"); err != nil || len(seasons) != 1 {
		t.Fatalf("seasons fallback failed: %v err=%v", seasons, err)
	}
	if episodes, err := f.GetEpisodes("id", true); err != nil || len(episodes) != 1 {
		t.Fatalf("episodes fallback failed: %v err=%v", episodes, err)
	}
}
