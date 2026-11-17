// Package audit records who changed what in the back office (table audit_log).
// Diffs must never contain secrets: callers mask API keys before recording.
package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Entry is one audit_log row.
type Entry struct {
	ActorType  string // admin | user | system
	ActorID    string
	Action     string // e.g. ai_provider.create
	EntityType string
	EntityID   string
	Diff       map[string]any
}

type model struct {
	ID         string         `gorm:"column:id;primaryKey"`
	ActorType  string         `gorm:"column:actor_type"`
	ActorID    string         `gorm:"column:actor_id"`
	Action     string         `gorm:"column:action"`
	EntityType string         `gorm:"column:entity_type"`
	EntityID   string         `gorm:"column:entity_id"`
	Diff       map[string]any `gorm:"column:diff;serializer:json"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (model) TableName() string { return "audit_log" }

// GormRecorder writes entries to PostgreSQL.
type GormRecorder struct {
	db *gorm.DB
}

// NewGormRecorder wraps db.
func NewGormRecorder(db *gorm.DB) *GormRecorder { return &GormRecorder{db: db} }

// Record inserts e.
func (r *GormRecorder) Record(ctx context.Context, e Entry) error {
	now := time.Now().UTC()
	m := model{ID: uuid.NewString(), ActorType: e.ActorType, ActorID: e.ActorID, Action: e.Action,
		EntityType: e.EntityType, EntityID: e.EntityID, Diff: e.Diff, CreatedAt: now, UpdatedAt: now}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("insert audit_log: %w", err)
	}
	return nil
}
