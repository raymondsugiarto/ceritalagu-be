package questionnaire

import (
	"strings"
	"time"
)

// CreateQuestionnaireInput is the body of POST /questionnaires: the 7 answers (PRD §5.1).
// Codes (recipient_type, occasion, genre) come from the master-data option lists.
type CreateQuestionnaireInput struct {
	RecipientType     string `json:"recipient_type"     validate:"required,max=50"`
	RecipientOther    string `json:"recipient_other"    validate:"omitempty,max=50"`
	RecipientName     string `json:"recipient_name"     validate:"required,max=50"`
	RecipientNickname string `json:"recipient_nickname" validate:"omitempty,max=50"`
	Occasion          string `json:"occasion"           validate:"required,max=50"`
	OccasionOther     string `json:"occasion_other"     validate:"omitempty,max=50"`
	Genre             string `json:"genre"              validate:"required,max=50"`
	Voice             string `json:"voice"              validate:"required,oneof=male female surprise"`
	Special           string `json:"special"            validate:"required,min=10,max=500"`
	Memory            string `json:"memory"             validate:"required,min=10,max=800"`
	Message           string `json:"message"            validate:"required,min=10,max=500"`
	Language          string `json:"language"           validate:"required,oneof=id en"`
	Mood              string `json:"mood"               validate:"omitempty,max=30"`
	// Last step: who the song is sent to. Submitting creates (or finds) the user by email.
	Email string `json:"email" validate:"required,email,max=254"`
	Phone string `json:"phone" validate:"required,min=8,max=30"`
}

// QuestionnaireOutput is a questionnaire as returned in "data".
type QuestionnaireOutput struct {
	ID                string     `json:"id"`
	RecipientType     string     `json:"recipient_type"`
	RecipientOther    string     `json:"recipient_other,omitempty"`
	RecipientName     string     `json:"recipient_name"`
	RecipientNickname string     `json:"recipient_nickname"`
	Occasion          string     `json:"occasion"`
	OccasionOther     string     `json:"occasion_other,omitempty"`
	Genre             string     `json:"genre"`
	Voice             string     `json:"voice"`
	Special           string     `json:"special"`
	Memory            string     `json:"memory"`
	Message           string     `json:"message"`
	Language          string     `json:"language"`
	Mood              string     `json:"mood"`
	SubmittedAt       *time.Time `json:"submitted_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (in CreateQuestionnaireInput) toAnswers() Answers {
	t := strings.TrimSpace
	return Answers{
		RecipientType: t(in.RecipientType), RecipientOther: t(in.RecipientOther), RecipientName: t(in.RecipientName),
		RecipientNickname: t(in.RecipientNickname), Occasion: t(in.Occasion), OccasionOther: t(in.OccasionOther),
		Genre: t(in.Genre), Voice: Voice(in.Voice),
		Special: t(in.Special), Memory: t(in.Memory), Message: t(in.Message),
		Language: Language(in.Language), Mood: t(in.Mood),
	}
}

func toQuestionnaireOutput(q Questionnaire) QuestionnaireOutput {
	a := q.Answers
	return QuestionnaireOutput{
		ID: q.ID, RecipientType: a.RecipientType, RecipientOther: a.RecipientOther, RecipientName: a.RecipientName,
		RecipientNickname: a.RecipientNickname, Occasion: a.Occasion, OccasionOther: a.OccasionOther, Genre: a.Genre, Voice: string(a.Voice), Special: a.Special, Memory: a.Memory,
		Message: a.Message, Language: string(a.Language), Mood: a.Mood, SubmittedAt: q.SubmittedAt, CreatedAt: q.CreatedAt,
	}
}
