package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNoActiveProvider is returned when no provider is active for a category.
var ErrNoActiveProvider = errors.New("no active ai provider")

// ErrNotFound is returned when no provider has the given ID.
var ErrNotFound = errors.New("ai provider not found")

type model struct {
	ID              string         `gorm:"column:id;primaryKey"`
	Category        string         `gorm:"column:category"`
	AdapterType     string         `gorm:"column:adapter_type"`
	Name            string         `gorm:"column:name"`
	BaseURL         string         `gorm:"column:base_url"`
	Model           string         `gorm:"column:model"`
	APIKeyEncrypted string         `gorm:"column:api_key_encrypted"`
	APIKeyLast4     string         `gorm:"column:api_key_last4"`
	Params          map[string]any `gorm:"column:params;serializer:json"`
	TimeoutSec      int            `gorm:"column:timeout_sec"`
	MaxRetries      int            `gorm:"column:max_retries"`
	Priority        *int           `gorm:"column:priority"` // NULL on rows from before migration 000014
	IsActive        bool           `gorm:"column:is_active"`
	IsFallback      bool           `gorm:"column:is_fallback"`
	CreatedAt       time.Time      `gorm:"column:created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (model) TableName() string { return "ai_provider" }

// GormRepository stores providers in PostgreSQL.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// Create inserts p (API key already encrypted), minting its ID if empty.
func (r *GormRepository) Create(ctx context.Context, p AIProvider) (AIProvider, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	if err := r.db.WithContext(ctx).Create(toModel(p)).Error; err != nil {
		return AIProvider{}, fmt.Errorf("insert ai_provider: %w", err)
	}
	return p, nil
}

// ListActive returns every active provider of c (the caller picks one with PickByPriority).
// It returns ErrNoActiveProvider when there is none.
func (r *GormRepository) ListActive(ctx context.Context, c Category) ([]AIProvider, error) {
	var ms []model
	err := r.db.WithContext(ctx).
		Where("category = ? AND is_active = ?", string(c), true).
		Order("id").
		Find(&ms).Error
	if err != nil {
		return nil, fmt.Errorf("select active ai_provider %s: %w", c, err)
	}
	if len(ms) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoActiveProvider, c)
	}
	out := make([]AIProvider, len(ms))
	for i, m := range ms {
		out[i] = m.toEntity()
	}
	return out, nil
}

// List returns the providers of c (all categories when c is empty): active first, then by
// priority (NULL counts as 1) and name.
func (r *GormRepository) List(ctx context.Context, c Category) ([]AIProvider, error) {
	q := r.db.WithContext(ctx).Order("category, is_active DESC, COALESCE(priority, 1), name")
	if c != "" {
		q = q.Where("category = ?", string(c))
	}
	var ms []model
	if err := q.Find(&ms).Error; err != nil {
		return nil, fmt.Errorf("select ai_provider list: %w", err)
	}
	out := make([]AIProvider, len(ms))
	for i, m := range ms {
		out[i] = m.toEntity()
	}
	return out, nil
}

// Get returns one provider, or ErrNotFound.
func (r *GormRepository) Get(ctx context.Context, id string) (AIProvider, error) {
	var m model
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AIProvider{}, ErrNotFound
	}
	if err != nil {
		return AIProvider{}, fmt.Errorf("select ai_provider %s: %w", id, err)
	}
	return m.toEntity(), nil
}

// Update saves every editable column of p (category and adapter type never change).
func (r *GormRepository) Update(ctx context.Context, p AIProvider) (AIProvider, error) {
	p.UpdatedAt = time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&model{}).Where("id = ?", p.ID).Updates(map[string]any{
		"name": p.Name, "base_url": p.BaseURL, "model": p.Model, "api_key_encrypted": p.APIKeyEncrypted,
		"api_key_last4": p.APIKeyLast4, "params": jsonParams(p.Params), "timeout_sec": p.TimeoutSec,
		"max_retries": p.MaxRetries, "priority": p.Priority, "updated_at": p.UpdatedAt,
	})
	if res.Error != nil {
		return AIProvider{}, fmt.Errorf("update ai_provider %s: %w", p.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return AIProvider{}, ErrNotFound
	}
	return p, nil
}

// Activate turns id on. Other active providers of its category stay on; priority decides between them.
func (r *GormRepository) Activate(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Model(&model{}).Where("id = ?", id).
		Updates(map[string]any{"is_active": true, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return fmt.Errorf("activate ai_provider %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Deactivate turns id off.
func (r *GormRepository) Deactivate(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Model(&model{}).Where("id = ?", id).
		Updates(map[string]any{"is_active": false, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return fmt.Errorf("deactivate ai_provider %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Archive soft-deletes id (sets deleted_at; the row stays for job history and audit).
func (r *GormRepository) Archive(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&model{}).Where("id = ?", id).
		Updates(map[string]any{"deleted_at": now, "updated_at": now, "is_active": false})
	if res.Error != nil {
		return fmt.Errorf("archive ai_provider %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// jsonParams lets Updates(map) store params as JSONB like the struct serializer does.
func jsonParams(p map[string]any) any {
	if p == nil {
		return nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil
	}
	return string(b)
}

func toModel(p AIProvider) *model {
	return &model{
		ID: p.ID, Category: string(p.Category), AdapterType: string(p.AdapterType), Name: p.Name,
		BaseURL: p.BaseURL, Model: p.Model, APIKeyEncrypted: p.APIKeyEncrypted, APIKeyLast4: p.APIKeyLast4,
		Params: p.Params, TimeoutSec: p.TimeoutSec, MaxRetries: p.MaxRetries, Priority: &p.Priority,
		IsActive: p.IsActive, IsFallback: p.IsFallback, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func (m model) toEntity() AIProvider {
	priority := DefaultPriority
	if m.Priority != nil && *m.Priority >= MinPriority {
		priority = *m.Priority
	}
	return AIProvider{
		Priority: priority,
		ID:       m.ID, Category: Category(m.Category), AdapterType: AdapterType(m.AdapterType), Name: m.Name,
		BaseURL: m.BaseURL, Model: m.Model, APIKeyEncrypted: m.APIKeyEncrypted, APIKeyLast4: m.APIKeyLast4,
		Params: m.Params, TimeoutSec: m.TimeoutSec, MaxRetries: m.MaxRetries, IsActive: m.IsActive,
		IsFallback: m.IsFallback, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}
