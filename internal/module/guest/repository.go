package guest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a guest session does not exist.
var ErrNotFound = errors.New("guest session not found")

type model struct {
	ID              string         `gorm:"column:id;primaryKey"`
	IPHash          string         `gorm:"column:ip_hash"`
	UserAgent       string         `gorm:"column:user_agent"`
	ClaimedByUserID string         `gorm:"column:claimed_by_user_id"`
	ClaimedAt       *time.Time     `gorm:"column:claimed_at"`
	LastSeenAt      *time.Time     `gorm:"column:last_seen_at"`
	CreatedAt       time.Time      `gorm:"column:created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (model) TableName() string { return "guest_session" }

// GormRepository stores guest sessions in PostgreSQL.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// Ensure creates the session row for v if missing, otherwise refreshes last_seen_at and the IP hash.
func (r *GormRepository) Ensure(ctx context.Context, v Visitor) error {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m model
		err := tx.Set("gorm:query_option", "FOR UPDATE").First(&m, "id = ?", v.SessionID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			m = model{ID: v.SessionID, IPHash: v.IPHash, UserAgent: v.UserAgent, LastSeenAt: &now, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&m).Error; err != nil {
				return fmt.Errorf("insert guest_session: %w", err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("select guest_session %s: %w", v.SessionID, err)
		}
		if err := tx.Model(&m).Updates(map[string]any{"ip_hash": v.IPHash, "last_seen_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("update guest_session %s: %w", v.SessionID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ensure guest_session: %w", err)
	}
	return nil
}

// Claim marks the session as belonging to userID (on customer sign-in). A session another user
// already claimed is left alone; a missing session is not an error (the visitor never saved anything).
func (r *GormRepository) Claim(ctx context.Context, sessionID, userID string, at time.Time) error {
	err := r.db.WithContext(ctx).Model(&model{}).
		Where("id = ? AND (claimed_by_user_id IS NULL OR claimed_by_user_id = '')", sessionID).
		Updates(map[string]any{"claimed_by_user_id": userID, "claimed_at": at, "updated_at": at}).Error
	if err != nil {
		return fmt.Errorf("claim guest_session %s: %w", sessionID, err)
	}
	return nil
}
