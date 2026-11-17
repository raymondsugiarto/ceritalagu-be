package prompt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/yourorg/music-api/internal/module/questionnaire"
)

type model struct {
	ID               string         `gorm:"column:id;primaryKey"`
	Name             string         `gorm:"column:name"`
	Version          int            `gorm:"column:version"`
	SystemPrompt     string         `gorm:"column:system_prompt"`
	UserPrompt       string         `gorm:"column:user_prompt"`
	Note             string         `gorm:"column:note"`
	IsActive         bool           `gorm:"column:is_active"`
	CreatedByAdminID string         `gorm:"column:created_by_admin_id"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (model) TableName() string { return "prompt_template" }

// hintRow is one master-data lookup (genre.style_hint, occasion/recipient_type.prompt_hint).
type hintRow struct {
	Name string
	Hint string
}

// GormRepository reads prompt templates and master-data hints from PostgreSQL.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// Active returns the active template, or Default when none is active.
func (r *GormRepository) Active(ctx context.Context) (Template, error) {
	var m model
	err := r.db.WithContext(ctx).Where("is_active = ?", true).Order("version DESC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Default, nil
	}
	if err != nil {
		return Template{}, fmt.Errorf("select active prompt_template: %w", err)
	}
	return m.toTemplate(), nil
}

// Hints looks up the admin hints for the chosen recipient type, occasion, genre and mood.
// A missing row is an empty hint, not an error.
func (r *GormRepository) Hints(ctx context.Context, a questionnaire.Answers) (Hints, error) {
	var h Hints
	var err error
	if h.RecipientTypeName, h.RecipientType, err = r.hint(ctx, "recipient_type", "prompt_hint", a.RecipientType); err != nil {
		return Hints{}, err
	}
	if h.OccasionName, h.Occasion, err = r.hint(ctx, "occasion", "prompt_hint", a.Occasion); err != nil {
		return Hints{}, err
	}
	if h.GenreName, h.Genre, err = r.hint(ctx, "genre", "style_hint", a.Genre); err != nil {
		return Hints{}, err
	}
	if h.MoodName, h.Mood, err = r.hint(ctx, "mood", "prompt_hint", a.Mood); err != nil {
		return Hints{}, err
	}
	return h, nil
}

// hint reads the option name and hint; table and column are constants from Hints, never user input.
// The row may be inactive by now (the questionnaire was validated when it was saved).
func (r *GormRepository) hint(ctx context.Context, table, column, code string) (name, hint string, err error) {
	if code == "" {
		return "", "", nil
	}
	var rows []hintRow
	err = r.db.WithContext(ctx).Table(table).
		Select("name, "+column+" AS hint").
		Where("code = ? AND deleted_at IS NULL", code).
		Order("created_at").Limit(1).Scan(&rows).Error
	if err != nil {
		return "", "", fmt.Errorf("select %s hint: %w", table, err)
	}
	if len(rows) == 0 {
		return "", "", nil
	}
	return rows[0].Name, rows[0].Hint, nil
}
