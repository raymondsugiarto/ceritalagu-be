package playground

import (
	"time"

	"github.com/yourorg/music-api/internal/pagination"
)

// ListSongsInput is the query of GET /admin/playground-songs: ?page=&page_size=&landing=true
type ListSongsInput struct {
	pagination.Input
	Landing bool `query:"landing"`
}

// UpdateSongInput is the body of PATCH /admin/playground-songs/:id (omitted fields stay as they are).
type UpdateSongInput struct {
	Title         *string `json:"title"           validate:"omitempty,max=255"`
	Caption       *string `json:"caption"         validate:"omitempty,max=255"`
	ShowOnLanding *bool   `json:"show_on_landing"`
	LandingOrder  *int    `json:"landing_order"   validate:"omitempty,min=0,max=1000"`
}

// SongOutput is a playground song for the admin.
type SongOutput struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Caption       string    `json:"caption"`
	Lyrics        string    `json:"lyrics"`
	StylePrompt   string    `json:"style_prompt"`
	AudioURL      string    `json:"audio_url"`
	Format        string    `json:"format"`
	DurationSec   int       `json:"duration_sec"`
	SizeBytes     int       `json:"size_bytes"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	ShowOnLanding bool      `json:"show_on_landing"`
	LandingOrder  int       `json:"landing_order"`
	CreatedAt     time.Time `json:"created_at"`
}

// SampleOutput is a public landing-page sample (no provider details).
type SampleOutput struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Caption     string `json:"caption"`
	Lyrics      string `json:"lyrics"`
	AudioURL    string `json:"audio_url"`
	DurationSec int    `json:"duration_sec"`
}

func toSongOutput(s Song) SongOutput {
	return SongOutput{
		ID: s.ID, Title: s.Title, Caption: s.Caption, Lyrics: s.Lyrics, StylePrompt: s.StylePrompt,
		AudioURL: "/api/v1/admin/playground-songs/" + s.ID + "/audio", Format: s.Format,
		DurationSec: s.DurationSec, SizeBytes: s.SizeBytes, Provider: s.Provider, Model: s.Model,
		ShowOnLanding: s.ShowOnLanding, LandingOrder: s.LandingOrder, CreatedAt: s.CreatedAt,
	}
}

func toSampleOutput(s Song) SampleOutput {
	return SampleOutput{ID: s.ID, Title: s.Title, Caption: s.Caption, Lyrics: s.Lyrics,
		AudioURL: "/api/v1/samples/" + s.ID + "/audio", DurationSec: s.DurationSec}
}
