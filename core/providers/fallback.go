package providers

import (
	"fmt"

	"github.com/MADPANDA3D/pandaflix/core"
)

// Fallback tries the primary provider first and transparently retries read
// operations on the backup provider when the primary fails. The TMDB-backed
// providers share the same request-ID format, so IDs produced by one are
// accepted by the other.
type Fallback struct {
	Primary     core.Provider
	Backup      core.Provider
	PrimaryName string
	BackupName  string
}

func NewFallback(primary, backup core.Provider, primaryName, backupName string) *Fallback {
	return &Fallback{Primary: primary, Backup: backup, PrimaryName: primaryName, BackupName: backupName}
}

func (f *Fallback) Search(query string) ([]core.SearchResult, error) {
	results, err := f.Primary.Search(query)
	if err == nil && len(results) > 0 {
		return results, nil
	}
	return f.Backup.Search(query)
}

func (f *Fallback) GetMediaID(url string) (string, error) {
	id, err := f.Primary.GetMediaID(url)
	if err == nil {
		return id, nil
	}
	return f.Backup.GetMediaID(url)
}

func (f *Fallback) GetSeasons(mediaID string) ([]core.Season, error) {
	seasons, err := f.Primary.GetSeasons(mediaID)
	if err == nil {
		return seasons, nil
	}
	return f.Backup.GetSeasons(mediaID)
}

func (f *Fallback) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	episodes, err := f.Primary.GetEpisodes(id, isSeason)
	if err == nil && len(episodes) > 0 {
		return episodes, nil
	}
	return f.Backup.GetEpisodes(id, isSeason)
}

func (f *Fallback) GetServers(episodeID string) ([]core.Server, error) {
	servers, err := f.Primary.GetServers(episodeID)
	if err == nil && len(servers) > 0 {
		return servers, nil
	}
	return f.Backup.GetServers(episodeID)
}

func (f *Fallback) GetLink(serverID string) (string, error) {
	link, err := f.Primary.GetLink(serverID)
	if err == nil {
		return link, nil
	}
	fmt.Printf("%s unavailable (%v); trying %s...\n", f.PrimaryName, err, f.BackupName)
	return f.Backup.GetLink(serverID)
}
