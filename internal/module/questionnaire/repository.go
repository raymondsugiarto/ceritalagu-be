package questionnaire

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a questionnaire does not exist.
var ErrNotFound = errors.New("questionnaire not found")

// answersJSON is the stored shape of questionnaire.answers (snake_case, matches the API DTO).
type answersJSON struct {
	RecipientType     string `json:"recipient_type"`
	RecipientOther    string `json:"recipient_other,omitempty"`
	RecipientName     string `json:"recipient_name"`
	RecipientNickname string `json:"recipient_nickname,omitempty"`
	Occasion          string `json:"occasion"`
	OccasionOther     string `json:"occasion_other,omitempty"`
	Genre             string `json:"genre"`
	Voice             string `json:"voice"`
	Special           string `json:"special"`
	Memory            string `json:"memory"`
	Message           string `json:"message"`
	Language          string `json:"language"`
	Mood              string `json:"mood,omitempty"`
}

type model struct {
	ID             string         `gorm:"column:id;primaryKey"`
	GuestSessionID string         `gorm:"column:guest_session_id"`
	UserID         string         `gorm:"column:user_id"`
	Answers        answersJSON    `gorm:"column:answers;serializer:json"`
	SubmittedAt    *time.Time     `gorm:"column:submitted_at"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (model) TableName() string { return "questionnaire" }

// GormRepository stores questionnaires in PostgreSQL.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// Create inserts q, minting its ID if empty, and returns it.
func (r *GormRepository) Create(ctx context.Context, q Questionnaire) (Questionnaire, error) {
	if q.ID == "" {
		q.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	q.CreatedAt, q.UpdatedAt = now, now
	m := model{
		ID: q.ID, GuestSessionID: q.GuestSessionID, UserID: q.UserID, Answers: toJSON(q.Answers),
		SubmittedAt: q.SubmittedAt, CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return Questionnaire{}, fmt.Errorf("insert questionnaire: %w", err)
	}
	return q, nil
}

// Get returns ErrNotFound if the questionnaire does not exist.
func (r *GormRepository) Get(ctx context.Context, id string) (Questionnaire, error) {
	var m model
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Questionnaire{}, ErrNotFound
	}
	if err != nil {
		return Questionnaire{}, fmt.Errorf("select questionnaire %s: %w", id, err)
	}
	return Questionnaire{
		ID: m.ID, GuestSessionID: m.GuestSessionID, UserID: m.UserID, Answers: fromJSON(m.Answers),
		SubmittedAt: m.SubmittedAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}, nil
}

func toJSON(a Answers) answersJSON {
	return answersJSON{
		RecipientType: a.RecipientType, RecipientOther: a.RecipientOther, RecipientName: a.RecipientName,
		RecipientNickname: a.RecipientNickname, Occasion: a.Occasion, OccasionOther: a.OccasionOther,
		Genre: a.Genre, Voice: string(a.Voice), Special: a.Special, Memory: a.Memory,
		Message: a.Message, Language: string(a.Language), Mood: a.Mood,
	}
}

func fromJSON(a answersJSON) Answers {
	return Answers{
		RecipientType: a.RecipientType, RecipientOther: a.RecipientOther, RecipientName: a.RecipientName,
		RecipientNickname: a.RecipientNickname, Occasion: a.Occasion, OccasionOther: a.OccasionOther,
		Genre: a.Genre, Voice: Voice(a.Voice), Special: a.Special, Memory: a.Memory,
		Message: a.Message, Language: Language(a.Language), Mood: a.Mood,
	}
}
