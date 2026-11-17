package playground

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound is returned for a missing or archived playground song.
var ErrNotFound = errors.New("playground song not found")

// ListParams is the repository-level query.
type ListParams struct {
	LandingOnly bool
	Offset      int
	Limit       int
}

type model struct {
	ID               string     `gorm:"column:id;primaryKey"`
	Title            string     `gorm:"column:title"`
	Caption          string     `gorm:"column:caption"`
	Lyrics           string     `gorm:"column:lyrics"`
	StylePrompt      string     `gorm:"column:style_prompt"`
	AudioKey         string     `gorm:"column:audio_key"`
	Format           string     `gorm:"column:format"`
	DurationSec      int        `gorm:"column:duration_sec"`
	SizeBytes        int        `gorm:"column:size_bytes"`
	Provider         string     `gorm:"column:provider"`
	Model            string     `gorm:"column:model"`
	ShowOnLanding    bool       `gorm:"column:show_on_landing"`
	LandingOrder     int        `gorm:"column:landing_order"`
	CreatedByAdminID string     `gorm:"column:created_by_admin_id"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at"`
}

func (model) TableName() string { return "playground_song" }

// GormRepository stores playground songs in PostgreSQL.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) live(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&model{}).Where("deleted_at IS NULL")
}

// Create inserts a song.
func (r *GormRepository) Create(ctx context.Context, s Song) error {
	m := fromSong(s)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("insert playground_song: %w", err)
	}
	return nil
}

// Get returns ErrNotFound for a missing or archived song.
func (r *GormRepository) Get(ctx context.Context, id string) (Song, error) {
	var m model
	err := r.live(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Song{}, ErrNotFound
	}
	if err != nil {
		return Song{}, fmt.Errorf("select playground_song %s: %w", id, err)
	}
	return m.toSong(), nil
}

// List returns one page, newest first; landing songs are sorted by landing_order instead.
func (r *GormRepository) List(ctx context.Context, p ListParams) ([]Song, int64, error) {
	q := r.live(ctx)
	order := "created_at DESC"
	if p.LandingOnly {
		q = q.Where("show_on_landing = ?", true)
		order = "COALESCE(landing_order, 0) ASC, created_at DESC"
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count playground_song: %w", err)
	}
	var rows []model
	if err := q.Order(order).Offset(p.Offset).Limit(p.Limit).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list playground_song: %w", err)
	}
	out := make([]Song, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toSong())
	}
	return out, total, nil
}

// Update saves the editable fields.
func (r *GormRepository) Update(ctx context.Context, s Song) error {
	res := r.live(ctx).Where("id = ?", s.ID).Updates(map[string]any{
		"title": s.Title, "caption": s.Caption, "show_on_landing": s.ShowOnLanding,
		"landing_order": s.LandingOrder, "updated_at": s.UpdatedAt,
	})
	if res.Error != nil {
		return fmt.Errorf("update playground_song %s: %w", s.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Archive is a soft delete (sets deleted_at). The audio file stays in storage.
func (r *GormRepository) Archive(ctx context.Context, id string, at time.Time) error {
	res := r.live(ctx).Where("id = ?", id).Updates(map[string]any{"deleted_at": at, "updated_at": at})
	if res.Error != nil {
		return fmt.Errorf("archive playground_song %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (m model) toSong() Song {
	return Song{
		ID: m.ID, Title: m.Title, Caption: m.Caption, Lyrics: m.Lyrics, StylePrompt: m.StylePrompt,
		AudioKey: m.AudioKey, Format: m.Format, DurationSec: m.DurationSec, SizeBytes: m.SizeBytes,
		Provider: m.Provider, Model: m.Model, ShowOnLanding: m.ShowOnLanding, LandingOrder: m.LandingOrder,
		CreatedByAdminID: m.CreatedByAdminID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fromSong(s Song) model {
	return model{
		ID: s.ID, Title: s.Title, Caption: s.Caption, Lyrics: s.Lyrics, StylePrompt: s.StylePrompt,
		AudioKey: s.AudioKey, Format: s.Format, DurationSec: s.DurationSec, SizeBytes: s.SizeBytes,
		Provider: s.Provider, Model: s.Model, ShowOnLanding: s.ShowOnLanding, LandingOrder: s.LandingOrder,
		CreatedByAdminID: s.CreatedByAdminID, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}
