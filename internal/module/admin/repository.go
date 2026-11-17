package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound is returned when no admin matches.
var ErrNotFound = errors.New("admin not found")

// Repository is the storage the service needs.
type Repository interface {
	ByEmail(ctx context.Context, email string) (Admin, error)
	ByID(ctx context.Context, id string) (Admin, error)
	Create(ctx context.Context, a Admin) (Admin, error)
	TouchLogin(ctx context.Context, id string, at time.Time) error
}

type model struct {
	ID           string         `gorm:"column:id;primaryKey"`
	Name         string         `gorm:"column:name"`
	Email        string         `gorm:"column:email"`
	PasswordHash string         `gorm:"column:password_hash"`
	Role         string         `gorm:"column:role"`
	Status       string         `gorm:"column:status"`
	LastLoginAt  *time.Time     `gorm:"column:last_login_at"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (model) TableName() string { return "admin_user" }

// GormRepository stores admins in PostgreSQL.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// ByEmail finds an admin by lowercased email.
func (r *GormRepository) ByEmail(ctx context.Context, email string) (Admin, error) {
	return r.first(ctx, "email = ?", normalizeEmail(email))
}

// ByID finds an admin by ID.
func (r *GormRepository) ByID(ctx context.Context, id string) (Admin, error) {
	return r.first(ctx, "id = ?", id)
}

func (r *GormRepository) first(ctx context.Context, where string, arg any) (Admin, error) {
	var m model
	err := r.db.WithContext(ctx).Where(where, arg).Order("created_at").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, fmt.Errorf("select admin_user: %w", err)
	}
	return m.toEntity(), nil
}

// Create inserts a, minting its ID. The caller checks the email is not taken (no UNIQUE index).
func (r *GormRepository) Create(ctx context.Context, a Admin) (Admin, error) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	a.CreatedAt, a.UpdatedAt = now, now
	a.Email = normalizeEmail(a.Email)
	m := model{ID: a.ID, Name: a.Name, Email: a.Email, PasswordHash: a.PasswordHash, Role: string(a.Role),
		Status: string(a.Status), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return Admin{}, fmt.Errorf("insert admin_user: %w", err)
	}
	return a, nil
}

// TouchLogin records a successful sign-in.
func (r *GormRepository) TouchLogin(ctx context.Context, id string, at time.Time) error {
	err := r.db.WithContext(ctx).Model(&model{}).Where("id = ?", id).
		Updates(map[string]any{"last_login_at": at, "updated_at": at}).Error
	if err != nil {
		return fmt.Errorf("update admin_user last_login_at: %w", err)
	}
	return nil
}

func (m model) toEntity() Admin {
	return Admin{ID: m.ID, Name: m.Name, Email: m.Email, PasswordHash: m.PasswordHash, Role: Role(m.Role),
		Status: Status(m.Status), LastLoginAt: m.LastLoginAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }
