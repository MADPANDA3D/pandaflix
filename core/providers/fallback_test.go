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

func TestFallbackGetLink(t *testing.T) {
	primary := &fakeProvider{linkErr: fmt.Errorf("primary down")}
	backup := &fakeProvider{link: "https://backup.example/master.m3u8"}
	f := NewFallback(primary, backup, "Primary", "Backup")

	link, err := f.GetLink("movie|1|Title|2000")
	if err != nil || link != backup.link {
		t.Fatalf("expected backup link, got %q err=%v", link, err)
	}
	if primary.linkCalls != 1 {
		t.Fatalf("primary should be tried once, got %d", primary.linkCalls)
	}

	primaryOK := &fakeProvider{link: "https://primary.example/master.m3u8"}
	backupOK := &fakeProvider{link: "https://backup.example/master.m3u8"}
	f2 := NewFallback(primaryOK, backupOK, "Primary", "Backup")
	link, err = f2.GetLink("movie|1|Title|2000")
	if err != nil || link != primaryOK.link {
		t.Fatalf("expected primary link, got %q err=%v", link, err)
	}
	if backupOK.linkCalls != 0 {
		t.Fatalf("backup should not be called on success")
	}
}

func TestFallbackReadOperations(t *testing.T) {
	primary := &fakeProvider{searchErr: fmt.Errorf("down"), seasonsErr: fmt.Errorf("down"), episodesErr: fmt.Errorf("down"), serversErr: fmt.Errorf("down"), mediaIDErr: fmt.Errorf("down")}
	backup := &fakeProvider{
		searchResults: []core.SearchResult{{Title: "T"}},
		mediaID:       "id",
		seasons:       []core.Season{{ID: "s"}},
		episodes:      []core.Episode{{ID: "e"}},
		servers:       []core.Server{{ID: "sv"}},
	}
	f := NewFallback(primary, backup, "Primary", "Backup")

	if results, err := f.Search("x"); err != nil || len(results) != 1 {
		t.Fatalf("search fallback failed: %v %v", results, err)
	}
	if id, err := f.GetMediaID("u"); err != nil || id != "id" {
		t.Fatalf("mediaID fallback failed: %q %v", id, err)
	}
	if seasons, err := f.GetSeasons("id"); err != nil || len(seasons) != 1 {
		t.Fatalf("seasons fallback failed: %v %v", seasons, err)
	}
	if episodes, err := f.GetEpisodes("id", true); err != nil || len(episodes) != 1 {
		t.Fatalf("episodes fallback failed: %v %v", episodes, err)
	}
	if servers, err := f.GetServers("id"); err != nil || len(servers) != 1 {
		t.Fatalf("servers fallback failed: %v %v", servers, err)
	}
}

func TestFallbackBothFail(t *testing.T) {
	primary := &fakeProvider{linkErr: fmt.Errorf("primary down")}
	backup := &fakeProvider{linkErr: fmt.Errorf("backup down")}
	f := NewFallback(primary, backup, "Primary", "Backup")

	if _, err := f.GetLink("movie|1|Title|2000"); err == nil {
		t.Fatal("expected error when both providers fail")
	}
}
