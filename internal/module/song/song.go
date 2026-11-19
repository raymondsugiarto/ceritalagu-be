// Package song owns generation jobs (song_job) and their results (song): status tracking,
// preview vs full access and claiming guest songs.
package song

import "time"

// JobStatus is the pipeline state shown to the user while a song is generated.
type JobStatus string

// Job statuses, in pipeline order.
const (
	JobQueued           JobStatus = "queued"
	JobGeneratingLyrics JobStatus = "generating_lyrics"
	JobGeneratingMusic  JobStatus = "generating_music"
	JobProcessingAudio  JobStatus = "processing_audio"
	JobCompleted        JobStatus = "completed"
	JobFailed           JobStatus = "failed"
)

// IsFinal reports whether no further step will run.
func (s JobStatus) IsFinal() bool { return s == JobCompleted || s == JobFailed }

// Job is one generation run (table song_job).
type Job struct {
	ID                string
	QuestionnaireID   string
	GuestSessionID    string
	UserID            string
	RegenerateOfJobID string
	Status            JobStatus
	Attempt           int
	PromptTemplateID  string
	ErrorCode         string // aiprovider.ErrorKind or a pipeline code; drives the message shown to the user
	ErrorMessage      string // internal detail for the admin job view, never shown to customers
	// LyricsProviderSnapshot and MusicProviderSnapshot freeze the provider used (no API key).
	LyricsProviderSnapshot map[string]any
	MusicProviderSnapshot  map[string]any
	LyricsStyleOutput      map[string]any
	Timings                map[string]int64 // step -> milliseconds
	StartedAt              *time.Time
	FinishedAt             *time.Time
	// RevealAt is set on a "Buat Ulang" served from a spare take: the job is already completed, but
	// customers see the usual progress steps until this time (see visibleJob).
	RevealAt  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Alternate is a spare take of a music request (table song_alternate), stored ready to become a song.
type Alternate struct {
	ID                 string
	JobID              string // job whose music request produced the take
	RootJobID          string // original job of that regenerate chain
	SongID             string // id the song gets; the audio keys already live under it
	FullAudioKey       string
	PreviewAudioKey    string
	DurationSec        int
	PreviewDurationSec int
	UsedByJobID        string // empty while free
	UsedAt             *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Song is the result of a completed job (table song).
type Song struct {
	ID                 string
	JobID              string
	QuestionnaireID    string
	OwnerUserID        string
	GuestSessionID     string
	Title              string
	Lyrics             string
	StylePrompt        string
	VocalGender        string
	Language           string
	Mood               string
	GenreCode          string
	FullAudioKey       string // private; served only via signed URL after an entitlement check
	PreviewAudioKey    string // separate 60 s file
	DurationSec        int
	PreviewDurationSec int
	IsUnlocked         bool
	UnlockedAt         *time.Time
	ExpiresAt          *time.Time // guest retention while unclaimed
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
