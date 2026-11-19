// Package generation runs one song job end to end:
//
//	questionnaire -> prompt -> lyrics & style AI -> music AI -> store full audio -> preview up to the first chorus -> song
//
// It depends only on small interfaces declared here; repositories, storage and the queue/worker
// that calls Pipeline.Run are wired in internal/cli.
package generation

import (
	"context"
	"time"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/prompt"
	"github.com/yourorg/music-api/internal/module/questionnaire"
	"github.com/yourorg/music-api/internal/module/song"
)

// Jobs persists jobs and their resulting songs.
type Jobs interface {
	GetJob(ctx context.Context, id string) (song.Job, error)
	UpdateJob(ctx context.Context, job song.Job) error
	CreateSong(ctx context.Context, s song.Song) error
	// CreateAlternate stores a spare take for a later "Buat Ulang".
	CreateAlternate(ctx context.Context, a song.Alternate) error
}

// Questionnaires loads submitted answers.
type Questionnaires interface {
	Get(ctx context.Context, id string) (questionnaire.Questionnaire, error)
}

// Prompts returns the active template (prompt.Default when none is active) and master-data hints.
type Prompts interface {
	Active(ctx context.Context) (prompt.Template, error)
	Hints(ctx context.Context, a questionnaire.Answers) (prompt.Hints, error)
}

// ResolvedProvider is the active provider of a category with its API key decrypted.
type ResolvedProvider struct {
	ID          string
	Name        string
	AdapterType aiprovider.AdapterType
	Config      aiprovider.Config
}

// Snapshot is what the job stores about the provider: never the API key.
func (p ResolvedProvider) Snapshot() map[string]any {
	return map[string]any{
		"id": p.ID, "name": p.Name, "adapter_type": string(p.AdapterType),
		"model": p.Config.Model, "base_url": p.Config.BaseURL,
	}
}

// Providers resolves the active provider per category.
type Providers interface {
	Active(ctx context.Context, c aiprovider.Category) (ResolvedProvider, error)
}

// Storage keeps audio files (private bucket in production).
type Storage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
}

// Notifier tells the customer that their song is ready (notification.Service).
// It is optional: a nil Notifier skips the email.
type Notifier interface {
	SongReady(ctx context.Context, s song.Song) error
}

// Audio cuts the preview and measures audio length.
type Audio interface {
	Preview(ctx context.Context, full []byte, seconds float64, fadeSec int) ([]byte, error)
	Duration(ctx context.Context, data []byte) (int, error)
}

// Options are the tunables from settings (PRD §6.3, §12).
type Options struct {
	PreviewSeconds int           // only when the lyrics have no [Chorus]; default 60
	FadeSeconds    int           // default 3
	GuestRetention time.Duration // default 30 days
	RetryBaseDelay time.Duration // first backoff, doubled per attempt; default 2 s
}

func (o Options) withDefaults() Options {
	if o.PreviewSeconds <= 0 {
		o.PreviewSeconds = 60
	}
	if o.FadeSeconds <= 0 {
		o.FadeSeconds = 3
	}
	if o.GuestRetention <= 0 {
		o.GuestRetention = 30 * 24 * time.Hour
	}
	if o.RetryBaseDelay <= 0 {
		o.RetryBaseDelay = 2 * time.Second
	}
	return o
}
