// Package aiprovider manages the AI providers an admin configures and defines the adapter
// interfaces the generation pipeline calls. Concrete adapters live in sub-packages (minimax/, ...).
package aiprovider

import "time"

// Priority bounds accepted from the admin form. Priority 1 is tried first.
const (
	MinPriority     = 1
	MaxPriority     = 100
	DefaultPriority = 1
)

// PickByPriority returns the provider a job should use: among ps (the active providers of one
// category) the ones with the smallest Priority, and of those one chosen by intn (rand.IntN).
// It returns false when ps is empty.
func PickByPriority(ps []AIProvider, intn func(n int) int) (AIProvider, bool) {
	if len(ps) == 0 {
		return AIProvider{}, false
	}
	best := ps[0].Priority
	for _, p := range ps[1:] {
		best = min(best, p.Priority)
	}
	var top []AIProvider
	for _, p := range ps {
		if p.Priority == best {
			top = append(top, p)
		}
	}
	return top[intn(len(top))], true
}

// Category says which pipeline step a provider serves. Several providers may be active per category;
// Priority decides which one a job uses (see PickByPriority).
type Category string

// Provider categories.
const (
	CategoryLyricsStyle Category = "lyrics_style"
	CategoryMusic       Category = "music"
)

// AdapterType selects the Go adapter that talks to a provider's API.
type AdapterType string

// Known adapters.
const (
	AdapterMiniMaxText  AdapterType = "minimax_text"
	AdapterMiniMaxMusic AdapterType = "minimax_music"
	AdapterLyriaMusic   AdapterType = "lyria_music"
	AdapterMuapiSuno    AdapterType = "muapi_suno"
	AdapterSunoAPISuno  AdapterType = "sunoapi_suno"
)

// AIProvider is the domain entity (table ai_provider).
type AIProvider struct {
	ID              string
	Category        Category
	AdapterType     AdapterType
	Name            string
	BaseURL         string
	Model           string
	APIKeyEncrypted string
	APIKeyLast4     string
	Params          map[string]any
	TimeoutSec      int
	MaxRetries      int
	Priority        int // 1 = first choice; equal numbers share the load at random
	IsActive        bool
	IsFallback      bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
