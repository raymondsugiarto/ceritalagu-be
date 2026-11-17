// Package prompt holds the admin-managed, versioned prompt templates for the lyrics & style AI
// and renders them with questionnaire answers.
package prompt

import (
	"strings"
	"time"

	"github.com/yourorg/music-api/internal/module/questionnaire"
)

// Template is the domain entity (table prompt_template).
type Template struct {
	ID               string
	Name             string
	Version          int
	SystemPrompt     string
	UserPrompt       string
	Note             string
	IsActive         bool
	CreatedByAdminID string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Hints are admin master-data texts that enrich the prompt (genre.style_hint, occasion.prompt_hint, ...).
// The *Name fields are the option names (e.g. "Ibu" for code "mother"); empty when the code has no row.
type Hints struct {
	RecipientType     string
	Occasion          string
	Genre             string
	Mood              string
	RecipientTypeName string
	OccasionName      string
	GenreName         string
	MoodName          string
}

// label is what the AI reads for a choice: the free text for "Lainnya", else the option name,
// else the raw code (older answers stored free text in the code field).
func label(code, other, name string) string {
	switch {
	case other != "":
		return other
	case name != "":
		return name
	default:
		return code
	}
}

// Placeholders lists every {{name}} a template may use, for the admin editor.
var Placeholders = []string{
	"recipient_type", "recipient_hint", "name", "nickname", "occasion", "occasion_hint",
	"genre", "genre_hint", "voice", "special", "memory", "message", "language", "mood", "mood_hint",
}

// Render fills {{placeholders}} in both prompts. Unknown placeholders are left as they are,
// so a typo is visible in the playground.
func Render(t Template, a questionnaire.Answers, h Hints) (system, user string) {
	nickname := a.RecipientNickname
	if nickname == "" {
		nickname = a.RecipientName
	}
	mood := label(a.Mood, "", h.MoodName)
	moodHint := h.Mood
	if mood == "" {
		mood = "sesuai momen"
		moodHint = "match the occasion"
	}
	r := strings.NewReplacer(
		"{{recipient_type}}", label(a.RecipientType, a.RecipientOther, h.RecipientTypeName),
		"{{recipient_hint}}", h.RecipientType,
		"{{name}}", a.RecipientName,
		"{{nickname}}", nickname,
		"{{occasion}}", label(a.Occasion, a.OccasionOther, h.OccasionName),
		"{{occasion_hint}}", h.Occasion,
		"{{genre}}", label(a.Genre, "", h.GenreName),
		"{{genre_hint}}", h.Genre,
		"{{voice}}", voiceText(a.Voice),
		"{{special}}", a.Special,
		"{{memory}}", a.Memory,
		"{{message}}", a.Message,
		"{{language}}", languageText(a.Language),
		"{{mood_hint}}", moodHint,
		"{{mood}}", mood,
	)
	return r.Replace(t.SystemPrompt), r.Replace(t.UserPrompt)
}

func voiceText(v questionnaire.Voice) string {
	switch v {
	case questionnaire.VoiceMale:
		return "male"
	case questionnaire.VoiceFemale:
		return "female"
	default:
		return "choose male or female, whichever suits the song best"
	}
}

func languageText(l questionnaire.Language) string {
	if l == questionnaire.LanguageEN {
		return "English"
	}
	return "Bahasa Indonesia"
}
