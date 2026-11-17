package prompt

import (
	"time"

	"github.com/yourorg/music-api/internal/module/questionnaire"
)

// maxPromptLen bounds each prompt (system and user) so a paste accident can't blow up the AI call.
const maxPromptLen = 20000

// CreateVersionInput saves an edit as a new version. Versions are never changed after saving.
type CreateVersionInput struct {
	Name         string `json:"name"          validate:"required,max=100"`
	SystemPrompt string `json:"system_prompt" validate:"required,max=20000"`
	UserPrompt   string `json:"user_prompt"   validate:"required,max=20000"`
	Note         string `json:"note"          validate:"max=500"`
	Activate     bool   `json:"activate"` // activate right after saving
}

// TemplateOutput is one version as the admin sees it. The built-in default has ID "" and version 0.
type TemplateOutput struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Version      int       `json:"version"`
	SystemPrompt string    `json:"system_prompt"`
	UserPrompt   string    `json:"user_prompt"`
	Note         string    `json:"note"`
	IsActive     bool      `json:"is_active"`
	CreatedBy    string    `json:"created_by_admin_id,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitzero"`
}

// ListOutput is GET /admin/prompts: saved versions (newest first), the built-in default that is used
// while none is active, and the placeholders the editor offers.
type ListOutput struct {
	Versions     []TemplateOutput `json:"versions"`
	Default      TemplateOutput   `json:"default"`
	DefaultInUse bool             `json:"default_in_use"`
	Placeholders []string         `json:"placeholders"`
}

// SampleAnswers are example questionnaire answers for the playground. Codes are master-data codes
// (or "other" with the free text); they are not validated against master data so an admin can try
// hidden options too.
type SampleAnswers struct {
	RecipientType     string `json:"recipient_type"     validate:"max=50"`
	RecipientOther    string `json:"recipient_other"    validate:"max=50"`
	RecipientName     string `json:"recipient_name"     validate:"max=50"`
	RecipientNickname string `json:"recipient_nickname" validate:"max=50"`
	Occasion          string `json:"occasion"           validate:"max=50"`
	OccasionOther     string `json:"occasion_other"     validate:"max=50"`
	Genre             string `json:"genre"              validate:"max=50"`
	Voice             string `json:"voice"              validate:"omitempty,oneof=male female surprise"`
	Special           string `json:"special"            validate:"max=500"`
	Memory            string `json:"memory"             validate:"max=800"`
	Message           string `json:"message"            validate:"max=500"`
	Language          string `json:"language"           validate:"omitempty,oneof=id en"`
	Mood              string `json:"mood"               validate:"max=30"`
}

func (a SampleAnswers) toAnswers() questionnaire.Answers {
	voice := questionnaire.Voice(a.Voice)
	if voice == "" {
		voice = questionnaire.VoiceSurprise
	}
	lang := questionnaire.Language(a.Language)
	if lang == "" {
		lang = questionnaire.LanguageID
	}
	return questionnaire.Answers{
		RecipientType: a.RecipientType, RecipientOther: a.RecipientOther, RecipientName: a.RecipientName,
		RecipientNickname: a.RecipientNickname, Occasion: a.Occasion, OccasionOther: a.OccasionOther,
		Genre: a.Genre, Voice: voice, Special: a.Special, Memory: a.Memory, Message: a.Message,
		Language: lang, Mood: a.Mood,
	}
}

// PlaygroundInput is a draft (not necessarily saved) plus sample answers.
type PlaygroundInput struct {
	SystemPrompt string        `json:"system_prompt" validate:"required,max=20000"`
	UserPrompt   string        `json:"user_prompt"   validate:"required,max=20000"`
	Answers      SampleAnswers `json:"answers"`
}

// RenderOutput is the prompt exactly as the AI would receive it.
type RenderOutput struct {
	SystemPrompt        string   `json:"system_prompt"`
	UserPrompt          string   `json:"user_prompt"`
	UnknownPlaceholders []string `json:"unknown_placeholders"`
}

// LyricsOutput is the parsed AI reply (after the same voice fix-up the pipeline applies).
type LyricsOutput struct {
	Title       string `json:"title"`
	Lyrics      string `json:"lyrics"`
	StylePrompt string `json:"style_prompt"`
	VocalGender string `json:"vocal_gender"`
	Language    string `json:"language"`
	Mood        string `json:"mood"`
}

// PlaygroundOutput is the result of one playground run. A provider failure is not an HTTP error:
// OK is false and Error explains it, so the admin still sees the rendered prompt.
type PlaygroundOutput struct {
	Rendered   RenderOutput  `json:"rendered"`
	OK         bool          `json:"ok"`
	Result     *LyricsOutput `json:"result,omitempty"`
	Raw        string        `json:"raw,omitempty"`
	ErrorKind  string        `json:"error_kind,omitempty"`
	Error      string        `json:"error,omitempty"`
	Provider   string        `json:"provider,omitempty"`
	Model      string        `json:"model,omitempty"`
	DurationMS int64         `json:"duration_ms"`
}

// MusicPlaygroundInput is step 2 of the playground. Limits match the MiniMax music API.
type MusicPlaygroundInput struct {
	Title       string `json:"title"        validate:"max=255"`
	Lyrics      string `json:"lyrics"       validate:"required,max=3500"`
	StylePrompt string `json:"style_prompt" validate:"max=2000"`
}

// MusicPlaygroundOutput is one music run. Like PlaygroundOutput, a provider failure is OK=false, not an
// HTTP error. AudioBase64 is the full song for instant playback; the song is also saved as a playground
// song (SavedSongID) so it can be picked for the landing page. SaveError is set when saving failed.
type MusicPlaygroundOutput struct {
	OK               bool   `json:"ok"`
	AudioBase64      string `json:"audio_base64,omitempty"`
	Format           string `json:"format,omitempty"`
	AudioDurationSec int    `json:"audio_duration_sec,omitempty"`
	SizeBytes        int    `json:"size_bytes,omitempty"`
	ErrorKind        string `json:"error_kind,omitempty"`
	Error            string `json:"error,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	DurationMS       int64  `json:"duration_ms"`
	SavedSongID      string `json:"saved_song_id,omitempty"`
	SaveError        string `json:"save_error,omitempty"`
}

func toOutput(t Template) TemplateOutput {
	return TemplateOutput{
		ID: t.ID, Name: t.Name, Version: t.Version, SystemPrompt: t.SystemPrompt, UserPrompt: t.UserPrompt,
		Note: t.Note, IsActive: t.IsActive, CreatedBy: t.CreatedByAdminID, CreatedAt: t.CreatedAt,
	}
}
