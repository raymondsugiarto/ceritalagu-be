package prompt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound is returned when no template matches.
var ErrNotFound = errors.New("prompt template not found")

func (m model) toTemplate() Template {
	return Template{
		ID: m.ID, Name: m.Name, Version: m.Version, SystemPrompt: m.SystemPrompt, UserPrompt: m.UserPrompt,
		Note: m.Note, IsActive: m.IsActive, CreatedByAdminID: m.CreatedByAdminID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// List returns every saved version, newest first.
func (r *GormRepository) List(ctx context.Context) ([]Template, error) {
	var rows []model
	if err := r.db.WithContext(ctx).Order("version DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("select prompt_template: %w", err)
	}
	out := make([]Template, len(rows))
	for i, m := range rows {
		out[i] = m.toTemplate()
	}
	return out, nil
}

// Get returns one version by ID.
func (r *GormRepository) Get(ctx context.Context, id string) (Template, error) {
	var m model
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("select prompt_template %s: %w", id, err)
	}
	return m.toTemplate(), nil
}

// Create saves t as the next version (max version + 1, counting archived rows so numbers are
// never reused). Versions are immutable: an edit is always a new row.
func (r *GormRepository) Create(ctx context.Context, t Template) (Template, error) {
	now := time.Now().UTC()
	m := model{
		ID: uuid.NewString(), Name: t.Name, SystemPrompt: t.SystemPrompt, UserPrompt: t.UserPrompt,
		Note: t.Note, CreatedByAdminID: t.CreatedByAdminID, CreatedAt: now, UpdatedAt: now,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize version numbering between concurrent saves.
		if err := tx.Exec("LOCK TABLE prompt_template IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
			return fmt.Errorf("lock prompt_template: %w", err)
		}
		var maxVersion int
		if err := tx.Unscoped().Model(&model{}).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return fmt.Errorf("select max version: %w", err)
		}
		m.Version = maxVersion + 1
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("insert prompt_template: %w", err)
		}
		return nil
	})
	if err != nil {
		return Template{}, fmt.Errorf("create prompt_template: %w", err)
	}
	return m.toTemplate(), nil
}

// Activate makes id the only active version, in one transaction.
func (r *GormRepository) Activate(ctx context.Context, id string) error {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model{}).Where("is_active = ? AND id <> ?", true, id).
			Updates(map[string]any{"is_active": false, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("deactivate other prompt_template: %w", err)
		}
		res := tx.Model(&model{}).Where("id = ?", id).Updates(map[string]any{"is_active": true, "updated_at": now})
		if res.Error != nil {
			return fmt.Errorf("activate prompt_template %s: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("activate prompt_template: %w", err)
	}
	return nil
}

// DeactivateAll turns every version off, so generation falls back to Default.
func (r *GormRepository) DeactivateAll(ctx context.Context) error {
	err := r.db.WithContext(ctx).Model(&model{}).Where("is_active = ?", true).
		Updates(map[string]any{"is_active": false, "updated_at": time.Now().UTC()}).Error
	if err != nil {
		return fmt.Errorf("deactivate prompt_template: %w", err)
	}
	return nil
}

// Archive soft-deletes an inactive version. Jobs keep its ID.
func (r *GormRepository) Archive(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ? AND is_active = ?", id, false).Delete(&model{})
	if res.Error != nil {
		return fmt.Errorf("archive prompt_template %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
