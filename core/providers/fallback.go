package providers

import (
	"fmt"
	"strings"

	"github.com/MADPANDA3D/pandaflix/core"
)

// Fallback presents several providers as one: searches use the first provider
// that answers, metadata is served by whichever provider understands the ID,
// and playback retries the remaining providers when one fails.
//
// Server IDs are prefixed with "p<i>|" to record which provider the user
// picked; GetLink still retries the others as a safety net.
type Fallback struct {
	Providers []core.Provider
	Names     []string
}

func NewFallback(providers []core.Provider, names []string) *Fallback {
	return &Fallback{Providers: providers, Names: names}
}

func (f *Fallback) providerCount() int {
	if len(f.Providers) < len(f.Names) {
		return len(f.Providers)
	}
	return len(f.Names)
}

// Search returns the first non-empty result set, in provider order.
func (f *Fallback) Search(query string) ([]core.SearchResult, error) {
	var lastErr error
	for i := 0; i < f.providerCount(); i++ {
		results, err := f.Providers[i].Search(query)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no results")
}

// GetMediaID asks each provider to parse the URL until one accepts it.
func (f *Fallback) GetMediaID(url string) (string, error) {
	var lastErr error
	for i := 0; i < f.providerCount(); i++ {
		id, err := f.Providers[i].GetMediaID(url)
		if err == nil {
			return id, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no provider accepted the URL")
}

func (f *Fallback) GetSeasons(mediaID string) ([]core.Season, error) {
	var lastErr error
	for i := 0; i < f.providerCount(); i++ {
		seasons, err := f.Providers[i].GetSeasons(mediaID)
		if err == nil {
			return seasons, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

func (f *Fallback) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	if !isSeason {
		// Movie flows use the "episode" list as the server list: expose every
		// provider as a selectable entry.
		servers := f.serverEntries(id)
		episodes := make([]core.Episode, 0, len(servers))
		for _, s := range servers {
			episodes = append(episodes, core.Episode{ID: s.ID, Name: s.Name})
		}
		return episodes, nil
	}
	var lastErr error
	for i := 0; i < f.providerCount(); i++ {
		episodes, err := f.Providers[i].GetEpisodes(id, isSeason)
		if err == nil && len(episodes) > 0 {
			return episodes, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no episodes found")
}

func (f *Fallback) GetServers(id string) ([]core.Server, error) {
	return f.serverEntries(id), nil
}

// serverEntries lists one selectable entry per provider, each carrying the
// request ID and the provider index to route on.
func (f *Fallback) serverEntries(id string) []core.Server {
	servers := make([]core.Server, 0, f.providerCount())
	for i := 0; i < f.providerCount(); i++ {
		servers = append(servers, core.Server{
			ID:   fmt.Sprintf("p%d|%s", i, id),
			Name: f.Names[i] + " (Auto)",
		})
	}
	return servers
}

// GetLink routes to the chosen provider and retries the others in order when
// it fails.
func (f *Fallback) GetLink(serverID string) (string, error) {
	chosen, id, ok := splitFallbackID(serverID)
	if !ok {
		// Unprefixed ID (legacy): use the default order.
		chosen, id = 0, serverID
	}

	order := make([]int, 0, f.providerCount())
	for i := 0; i < f.providerCount(); i++ {
		order = append(order, i)
	}
	// Try the chosen provider first, then the rest in listed order.
	for i, idx := range order {
		if idx == chosen && i != 0 {
			order[0], order[i] = order[i], order[0]
			break
		}
	}

	var lastErr error
	for attempt, idx := range order {
		link, err := f.Providers[idx].GetLink(id)
		if err == nil {
			return link, nil
		}
		lastErr = err
		if attempt+1 < len(order) {
			fmt.Printf("%s unavailable (%v); trying %s...\n", f.Names[idx], err, f.Names[order[attempt+1]])
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all providers failed")
	}
	return "", fmt.Errorf("all providers failed: %w", lastErr)
}

// splitFallbackID parses a "p<i>|<id>" server ID.
func splitFallbackID(serverID string) (providerIndex int, id string, ok bool) {
	if !strings.HasPrefix(serverID, "p") {
		return 0, "", false
	}
	bar := strings.Index(serverID, "|")
	if bar < 2 {
		return 0, "", false
	}
	var idx int
	if _, err := fmt.Sscanf(serverID[:bar], "p%d", &idx); err != nil || idx < 0 {
		return 0, "", false
	}
	return idx, serverID[bar+1:], true
}
