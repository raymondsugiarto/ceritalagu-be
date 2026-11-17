// Package questionnaire stores the 7-step answers a guest or user fills before generating a song.
package questionnaire

import "time"

// Voice is the vocal choice in step 4.
type Voice string

// Voice options. VoiceSurprise lets the lyrics & style AI decide.
const (
	VoiceMale     Voice = "male"
	VoiceFemale   Voice = "female"
	VoiceSurprise Voice = "surprise"
)

// Language is the song language.
type Language string

// Supported languages (MVP).
const (
	LanguageID Language = "id"
	LanguageEN Language = "en"
)

// Answers maps the questionnaire steps (stored as JSONB in questionnaire.answers).
type Answers struct {
	RecipientType     string   // 1. Lagu ini untuk siapa (recipient_type.code, or "other")
	RecipientOther    string   // 1. free text when RecipientType is "other"
	RecipientName     string   // 2. Nama
	RecipientNickname string   // 2. Panggilan (optional)
	Occasion          string   // 3. occasion.code, or "other"
	OccasionOther     string   // 3. free text when Occasion is "other"
	Genre             string   // 4. genre.code
	Voice             Voice    // 4. pria / wanita / kejutkan aku
	Special           string   // 5. Apa yang membuatnya spesial
	Memory            string   // 6. Kenangan favorit
	Message           string   // 7. Pesan dari hati
	Language          Language // id | en
	Mood              string   // optional
}

// Questionnaire is the domain entity (table questionnaire).
type Questionnaire struct {
	ID             string
	GuestSessionID string
	UserID         string
	Answers        Answers
	SubmittedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
