package aiprovider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrUnknownAdapter is returned by Registry when no adapter is registered for an AdapterType.
var ErrUnknownAdapter = errors.New("unknown ai adapter")

// Config is what an adapter needs for one call: the provider row with the API key decrypted.
// It never leaves the server and is never logged.
type Config struct {
	BaseURL    string
	Model      string
	APIKey     string
	Params     map[string]any
	Timeout    time.Duration
	MaxRetries int
}

// LyricsStyleRequest is the rendered prompt (prompt template + questionnaire answers).
type LyricsStyleRequest struct {
	SystemPrompt string
	UserPrompt   string
}

// LyricsStyleResult is the structured output the lyrics & style AI must return.
type LyricsStyleResult struct {
	Title       string
	Lyrics      string // with section tags: [Verse], [Chorus], [Bridge], ...
	StylePrompt string
	VocalGender string // male | female
	Language    string // id | en
	Mood        string
	Raw         string // provider response, kept for debugging in the admin job detail
}

// MusicRequest is the input of the music AI.
type MusicRequest struct {
	Lyrics      string
	StylePrompt string
	Title       string // optional; adapters that accept a title (Suno) use it
	VocalGender string // optional: male | female; adapters that accept it (Suno) use it
}

// MusicResult is the generated full song.
type MusicResult struct {
	Audio       []byte
	Format      string // e.g. mp3
	DurationSec int
	// Words are the sung lyrics with timestamps, when the provider returns them (Suno does).
	// The preview is cut at the end of the first chorus using them; empty means "estimate".
	Words []AlignedWord
	// Alternate is a second take of the same lyrics from the same request, when the provider makes
	// one (Suno always returns two). It is kept as a spare for "Buat Ulang". Nil when there is none.
	Alternate *MusicTrack
}

// MusicTrack is one extra take returned alongside the main MusicResult.
type MusicTrack struct {
	Audio       []byte
	Format      string
	DurationSec int
	Words       []AlignedWord
}

// AlignedWord is one sung word and where it sits in the audio. Text may carry a section tag
// before the word (Suno returns "[Chorus]\nWord").
type AlignedWord struct {
	Text     string
	StartSec float64
	EndSec   float64
}

// LyricsStyleProvider turns answers (as a rendered prompt) into lyrics + a style prompt.
type LyricsStyleProvider interface {
	GenerateLyricsStyle(ctx context.Context, req LyricsStyleRequest, cfg Config) (LyricsStyleResult, error)
	TestConnection(ctx context.Context, cfg Config) error
}

// MusicProvider turns lyrics + a style prompt into audio.
type MusicProvider interface {
	GenerateMusic(ctx context.Context, req MusicRequest, cfg Config) (MusicResult, error)
	TestConnection(ctx context.Context, cfg Config) error
}

// Registry maps an AdapterType to its implementation. Adapters are registered at startup (internal/cli).
type Registry struct {
	lyrics map[AdapterType]LyricsStyleProvider
	music  map[AdapterType]MusicProvider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		lyrics: make(map[AdapterType]LyricsStyleProvider),
		music:  make(map[AdapterType]MusicProvider),
	}
}

// RegisterLyricsStyle adds a lyrics & style adapter.
func (r *Registry) RegisterLyricsStyle(t AdapterType, p LyricsStyleProvider) { r.lyrics[t] = p }

// RegisterMusic adds a music adapter.
func (r *Registry) RegisterMusic(t AdapterType, p MusicProvider) { r.music[t] = p }

// LyricsStyle returns the adapter for t, or ErrUnknownAdapter.
func (r *Registry) LyricsStyle(t AdapterType) (LyricsStyleProvider, error) {
	p, ok := r.lyrics[t]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAdapter, t)
	}
	return p, nil
}

// Music returns the adapter for t, or ErrUnknownAdapter.
func (r *Registry) Music(t AdapterType) (MusicProvider, error) {
	p, ok := r.music[t]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAdapter, t)
	}
	return p, nil
}

// AdapterInfo describes an adapter for the admin form: which category it serves and its defaults.
type AdapterInfo struct {
	Type           AdapterType
	Category       Category
	Label          string
	Help           string // short setup note shown under the adapter picker
	DefaultBaseURL string
	DefaultModel   string
	Models         []string    // suggestions; the admin may type another model name
	Params         []ParamSpec // adapter-specific settings stored in AIProvider.Params
}

// ParamKind is how a param's value is stored in JSON (and so how the adapter reads it).
type ParamKind string

// Param kinds.
const (
	ParamString ParamKind = "string" // JSON string, one line
	ParamText   ParamKind = "text"   // JSON string, multi-line
	ParamInt    ParamKind = "int"    // JSON number, whole
	ParamFloat  ParamKind = "float"  // JSON number
)

// ParamSpec describes one adapter-specific setting for the admin form. With Options the form shows
// a dropdown of those values (stored as Kind). An empty value is not stored, so the adapter uses
// its own default (Placeholder says which).
type ParamSpec struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Kind        ParamKind `json:"kind"`
	Options     []string  `json:"options,omitempty"`
	Min         *float64  `json:"min,omitempty"`
	Max         *float64  `json:"max,omitempty"`
	Placeholder string    `json:"placeholder,omitempty"`
	Help        string    `json:"help,omitempty"`
}

// Bound is a helper for ParamSpec.Min / Max literals.
func Bound(v float64) *float64 { return &v }
