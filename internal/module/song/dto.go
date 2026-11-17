package song

import "time"

// CreateSongJobInput is the body of POST /song-jobs.
type CreateSongJobInput struct {
	QuestionnaireID string `json:"questionnaire_id" validate:"required,uuid"`
}

// SongJobOutput is a job's progress as polled by the loading screen.
type SongJobOutput struct {
	ID                string     `json:"id"`
	QuestionnaireID   string     `json:"questionnaire_id"`
	Status            JobStatus  `json:"status"`
	StatusMessage     string     `json:"status_message"`
	ErrorCode         string     `json:"error_code,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"` // customer-safe text, never provider details
	SongID            string     `json:"song_id,omitempty"`
	RegenerateOfJobID string     `json:"regenerate_of_job_id,omitempty"`
	RegeneratesLeft   int        `json:"regenerates_left"`
	CreatedAt         time.Time  `json:"created_at"`
	FinishedAt        *time.Time `json:"finished_at"`
}

// SongOutput is a song as shown on the result page. Lyrics is only set once the song is unlocked;
// before that the page shows LyricsPreview and the rest blurred.
type SongOutput struct {
	ID                 string     `json:"id"`
	JobID              string     `json:"job_id"`
	Title              string     `json:"title"`
	Genre              string     `json:"genre"`
	VocalGender        string     `json:"vocal_gender"`
	Language           string     `json:"language"`
	Mood               string     `json:"mood"`
	DurationSec        int        `json:"duration_sec"`
	PreviewDurationSec int        `json:"preview_duration_sec"`
	PreviewURL         string     `json:"preview_url"`
	AudioURL           string     `json:"audio_url,omitempty"` // full song, only once unlocked
	LyricsPreview      string     `json:"lyrics_preview"`
	Lyrics             string     `json:"lyrics,omitempty"`
	IsUnlocked         bool       `json:"is_unlocked"`
	ExpiresAt          *time.Time `json:"expires_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

// statusMessages are the loading-screen texts (PRD §5.2).
var statusMessages = map[JobStatus]string{
	JobQueued:           "Menyiapkan lagumu…",
	JobGeneratingLyrics: "Menulis lirik spesial…",
	JobGeneratingMusic:  "Mengaransemen musik & merekam vokal…",
	JobProcessingAudio:  "Sentuhan akhir…",
	JobCompleted:        "Lagumu sudah siap!",
	JobFailed:           "Maaf, terjadi kendala.",
}

func toSongJobOutput(j Job, songID string, regeneratesLeft int) SongJobOutput {
	out := SongJobOutput{
		ID: j.ID, QuestionnaireID: j.QuestionnaireID, Status: j.Status, StatusMessage: statusMessages[j.Status],
		SongID: songID, RegenerateOfJobID: j.RegenerateOfJobID, RegeneratesLeft: regeneratesLeft,
		CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
	}
	if j.Status == JobFailed {
		out.ErrorCode, out.ErrorMessage = j.ErrorCode, UserMessage(j.ErrorCode)
	}
	return out
}

func toSongOutput(s Song) SongOutput {
	out := SongOutput{
		ID: s.ID, JobID: s.JobID, Title: s.Title, Genre: s.GenreCode, VocalGender: s.VocalGender,
		Language: s.Language, Mood: s.Mood, DurationSec: s.DurationSec, PreviewDurationSec: s.PreviewDurationSec,
		PreviewURL: "/api/v1/songs/" + s.ID + "/preview", LyricsPreview: PreviewLyrics(s.Lyrics),
		IsUnlocked: s.IsUnlocked, ExpiresAt: s.ExpiresAt, CreatedAt: s.CreatedAt,
	}
	if s.IsUnlocked {
		out.Lyrics = s.Lyrics
		out.AudioURL = "/api/v1/songs/" + s.ID + "/audio"
	}
	return out
}
