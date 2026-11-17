package song

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Admin view of what the customer typed in the questionnaire (steps 1-8) behind a song.

// ErrAnswersNotFound is returned when the song or its questionnaire does not exist.
var ErrAnswersNotFound = errors.New("song answers not found")

// AdminSongAnswersOutput is the body of GET /admin/songs/:id/answers.
// Codes come with their master-data names (empty when the code is "other" or no longer exists).
type AdminSongAnswersOutput struct {
	QuestionnaireID   string     `json:"questionnaire_id"`
	RecipientType     string     `json:"recipient_type"`      // 1
	RecipientTypeName string     `json:"recipient_type_name"` // 1
	RecipientOther    string     `json:"recipient_other"`     // 1
	RecipientName     string     `json:"recipient_name"`      // 2
	RecipientNickname string     `json:"recipient_nickname"`  // 2
	Occasion          string     `json:"occasion"`            // 3
	OccasionName      string     `json:"occasion_name"`       // 3
	OccasionOther     string     `json:"occasion_other"`      // 3
	Genre             string     `json:"genre"`               // 4
	GenreName         string     `json:"genre_name"`          // 4
	Voice             string     `json:"voice"`               // 4
	Language          string     `json:"language"`            // 4
	Mood              string     `json:"mood"`                // 4
	MoodName          string     `json:"mood_name"`           // 4
	Special           string     `json:"special"`             // 5
	Memory            string     `json:"memory"`              // 6
	Message           string     `json:"message"`             // 7
	Email             string     `json:"email"`               // 8
	Phone             string     `json:"phone"`               // 8
	SubmittedAt       *time.Time `json:"submitted_at"`
}

type adminAnswersRow struct {
	QuestionnaireID string     `gorm:"column:questionnaire_id"`
	Answers         answersRaw `gorm:"column:answers;serializer:json"`
	RecipientName   string     `gorm:"column:recipient_type_name"`
	OccasionName    string     `gorm:"column:occasion_name"`
	GenreName       string     `gorm:"column:genre_name"`
	MoodName        string     `gorm:"column:mood_name"`
	Email           string     `gorm:"column:email"`
	Phone           string     `gorm:"column:phone"`
	SubmittedAt     *time.Time `gorm:"column:submitted_at"`
}

// answersRaw mirrors the stored questionnaire.answers JSONB.
type answersRaw struct {
	RecipientType     string `json:"recipient_type"`
	RecipientOther    string `json:"recipient_other"`
	RecipientName     string `json:"recipient_name"`
	RecipientNickname string `json:"recipient_nickname"`
	Occasion          string `json:"occasion"`
	OccasionOther     string `json:"occasion_other"`
	Genre             string `json:"genre"`
	Voice             string `json:"voice"`
	Special           string `json:"special"`
	Memory            string `json:"memory"`
	Message           string `json:"message"`
	Language          string `json:"language"`
	Mood              string `json:"mood"`
}

// SongAnswers loads the questionnaire answers behind song id.
func (r *GormRepository) SongAnswers(ctx context.Context, id string) (AdminSongAnswersOutput, error) {
	const q = `
SELECT q.id AS questionnaire_id, q.answers, q.submitted_at,
       COALESCE(u.email, '') AS email, COALESCE(u.phone, '') AS phone,
       COALESCE((SELECT name FROM recipient_type WHERE code = q.answers->>'recipient_type' ORDER BY deleted_at NULLS FIRST LIMIT 1), '') AS recipient_type_name,
       COALESCE((SELECT name FROM occasion WHERE code = q.answers->>'occasion' ORDER BY deleted_at NULLS FIRST LIMIT 1), '') AS occasion_name,
       COALESCE((SELECT name FROM genre WHERE code = q.answers->>'genre' ORDER BY deleted_at NULLS FIRST LIMIT 1), '') AS genre_name,
       COALESCE((SELECT name FROM mood WHERE code = q.answers->>'mood' ORDER BY deleted_at NULLS FIRST LIMIT 1), '') AS mood_name
FROM song s
JOIN questionnaire q ON q.id = s.questionnaire_id
LEFT JOIN "user" u ON u.id = q.user_id
WHERE s.id = ? AND s.deleted_at IS NULL
LIMIT 1`
	var rows []adminAnswersRow
	if err := r.db.WithContext(ctx).Raw(q, id).Scan(&rows).Error; err != nil {
		return AdminSongAnswersOutput{}, fmt.Errorf("select song answers %s: %w", id, err)
	}
	if len(rows) == 0 {
		return AdminSongAnswersOutput{}, ErrAnswersNotFound
	}
	m := rows[0]
	a := m.Answers
	return AdminSongAnswersOutput{
		QuestionnaireID: m.QuestionnaireID,
		RecipientType:   a.RecipientType, RecipientTypeName: m.RecipientName, RecipientOther: a.RecipientOther,
		RecipientName: a.RecipientName, RecipientNickname: a.RecipientNickname,
		Occasion: a.Occasion, OccasionName: m.OccasionName, OccasionOther: a.OccasionOther,
		Genre: a.Genre, GenreName: m.GenreName, Voice: a.Voice, Language: a.Language, Mood: a.Mood, MoodName: m.MoodName,
		Special: a.Special, Memory: a.Memory, Message: a.Message,
		Email: m.Email, Phone: m.Phone, SubmittedAt: m.SubmittedAt,
	}, nil
}

// Answers returns the questionnaire answers behind a song.
func (s *AdminSongService) Answers(ctx context.Context, id string) (AdminSongAnswersOutput, error) {
	out, err := s.repo.SongAnswers(ctx, id)
	if errors.Is(err, ErrAnswersNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		return AdminSongAnswersOutput{}, apperror.NotFound("Jawaban kuesioner tidak ditemukan.")
	}
	if err != nil {
		return AdminSongAnswersOutput{}, apperror.Internal(err)
	}
	return out, nil
}

func (h *AdminSongHandler) answers(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Answers(c.Context(), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}
