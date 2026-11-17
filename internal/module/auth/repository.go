package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Repository-level sentinel errors.
var (
	ErrNotFound     = errors.New("auth record not found")
	ErrAlreadyUsed  = errors.New("otp already consumed")
	ErrCredentialed = errors.New("user already has a credential")
)

// Repository stores credentials and login codes.
type Repository interface {
	CredentialByUserID(ctx context.Context, userID string) (Credential, error)
	// CreateCredential returns ErrCredentialed if the user already has one.
	CreateCredential(ctx context.Context, c Credential) error

	CreateOTP(ctx context.Context, o OTP) error
	// LatestOTP is the newest code sent to email for purpose (consumed or not).
	LatestOTP(ctx context.Context, email, purpose string) (OTP, error)
	// AddOTPAttempt counts a wrong guess and returns the new total.
	AddOTPAttempt(ctx context.Context, id string, at time.Time) (int, error)
	// ConsumeOTP marks the code used; ErrAlreadyUsed if another request used it first.
	ConsumeOTP(ctx context.Context, id string, at time.Time) error
}

// ---- PostgreSQL ----

type credentialModel struct {
	ID            string     `gorm:"column:id;primaryKey"`
	UserID        string     `gorm:"column:user_id"`
	PasswordHash  string     `gorm:"column:password_hash"`
	PasswordSetAt time.Time  `gorm:"column:password_set_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
	DeletedAt     *time.Time `gorm:"column:deleted_at"`
}

func (credentialModel) TableName() string { return "user_credential" }

type otpModel struct {
	ID         string     `gorm:"column:id;primaryKey"`
	UserID     string     `gorm:"column:user_id"`
	Email      string     `gorm:"column:email"`
	Purpose    string     `gorm:"column:purpose"`
	CodeHash   string     `gorm:"column:code_hash"`
	Attempts   int        `gorm:"column:attempts"`
	ExpiresAt  time.Time  `gorm:"column:expires_at"`
	ConsumedAt *time.Time `gorm:"column:consumed_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
	DeletedAt  *time.Time `gorm:"column:deleted_at"`
}

func (otpModel) TableName() string { return "user_otp" }

// GormRepository stores auth rows in PostgreSQL. "One credential per user" is enforced here with a
// row lock on the user, since the database has no unique index (docs/database-standards.md §3).
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// CredentialByUserID returns ErrNotFound when the user has no password yet.
func (r *GormRepository) CredentialByUserID(ctx context.Context, userID string) (Credential, error) {
	var m credentialModel
	err := r.db.WithContext(ctx).Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, fmt.Errorf("select user_credential: %w", err)
	}
	return Credential{ID: m.ID, UserID: m.UserID, PasswordHash: m.PasswordHash, PasswordSetAt: m.PasswordSetAt,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}, nil
}

// CreateCredential locks the user row so two parallel "set password" calls cannot both insert.
func (r *GormRepository) CreateCredential(ctx context.Context, c Credential) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT id FROM "user" WHERE id = ? FOR UPDATE`, c.UserID).Error; err != nil {
			return fmt.Errorf("lock user %s: %w", c.UserID, err)
		}
		var n int64
		if err := tx.Model(&credentialModel{}).Where("user_id = ? AND deleted_at IS NULL", c.UserID).Count(&n).Error; err != nil {
			return fmt.Errorf("count user_credential: %w", err)
		}
		if n > 0 {
			return ErrCredentialed
		}
		m := credentialModel{ID: c.ID, UserID: c.UserID, PasswordHash: c.PasswordHash, PasswordSetAt: c.PasswordSetAt,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("insert user_credential: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create user_credential: %w", err)
	}
	return nil
}

// CreateOTP inserts a code.
func (r *GormRepository) CreateOTP(ctx context.Context, o OTP) error {
	m := otpModel{ID: o.ID, UserID: o.UserID, Email: o.Email, Purpose: o.Purpose, CodeHash: o.CodeHash,
		Attempts: o.Attempts, ExpiresAt: o.ExpiresAt, CreatedAt: o.CreatedAt, UpdatedAt: o.CreatedAt}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("insert user_otp: %w", err)
	}
	return nil
}

// LatestOTP returns ErrNotFound when no code was ever sent.
func (r *GormRepository) LatestOTP(ctx context.Context, email, purpose string) (OTP, error) {
	var m otpModel
	err := r.db.WithContext(ctx).Where("email = ? AND purpose = ? AND deleted_at IS NULL", strings.ToLower(email), purpose).
		Order("created_at DESC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return OTP{}, ErrNotFound
	}
	if err != nil {
		return OTP{}, fmt.Errorf("select user_otp: %w", err)
	}
	return OTP{ID: m.ID, UserID: m.UserID, Email: m.Email, Purpose: m.Purpose, CodeHash: m.CodeHash, Attempts: m.Attempts,
		ExpiresAt: m.ExpiresAt, ConsumedAt: m.ConsumedAt, CreatedAt: m.CreatedAt}, nil
}

// AddOTPAttempt increments attempts atomically.
func (r *GormRepository) AddOTPAttempt(ctx context.Context, id string, at time.Time) (int, error) {
	var attempts int
	err := r.db.WithContext(ctx).Raw(
		`UPDATE user_otp SET attempts = COALESCE(attempts, 0) + 1, updated_at = ? WHERE id = ? RETURNING attempts`, at, id,
	).Scan(&attempts).Error
	if err != nil {
		return 0, fmt.Errorf("count user_otp attempt: %w", err)
	}
	return attempts, nil
}

// ConsumeOTP only succeeds once per code.
func (r *GormRepository) ConsumeOTP(ctx context.Context, id string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&otpModel{}).Where("id = ? AND consumed_at IS NULL", id).
		Updates(map[string]any{"consumed_at": at, "updated_at": at})
	if res.Error != nil {
		return fmt.Errorf("consume user_otp: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrAlreadyUsed
	}
	return nil
}

// ---- In memory (tests) ----

// MemoryRepository is a thread-safe in-memory Repository.
type MemoryRepository struct {
	mu    sync.Mutex
	creds []Credential
	otps  []OTP
}

// NewMemoryRepository returns an empty store.
func NewMemoryRepository() *MemoryRepository { return &MemoryRepository{} }

// CredentialByUserID implements Repository.
func (r *MemoryRepository) CredentialByUserID(_ context.Context, userID string) (Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.creds {
		if c.UserID == userID {
			return c, nil
		}
	}
	return Credential{}, ErrNotFound
}

// CreateCredential implements Repository.
func (r *MemoryRepository) CreateCredential(_ context.Context, c Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.creds {
		if x.UserID == c.UserID {
			return ErrCredentialed
		}
	}
	r.creds = append(r.creds, c)
	return nil
}

// CreateOTP implements Repository.
func (r *MemoryRepository) CreateOTP(_ context.Context, o OTP) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.otps = append(r.otps, o)
	return nil
}

// LatestOTP implements Repository.
func (r *MemoryRepository) LatestOTP(_ context.Context, email, purpose string) (OTP, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.otps) - 1; i >= 0; i-- {
		if o := r.otps[i]; strings.EqualFold(o.Email, email) && o.Purpose == purpose {
			return o, nil
		}
	}
	return OTP{}, ErrNotFound
}

// AddOTPAttempt implements Repository.
func (r *MemoryRepository) AddOTPAttempt(_ context.Context, id string, _ time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.otps {
		if r.otps[i].ID == id {
			r.otps[i].Attempts++
			return r.otps[i].Attempts, nil
		}
	}
	return 0, ErrNotFound
}

// ConsumeOTP implements Repository.
func (r *MemoryRepository) ConsumeOTP(_ context.Context, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.otps {
		if r.otps[i].ID == id {
			if r.otps[i].ConsumedAt != nil {
				return ErrAlreadyUsed
			}
			r.otps[i].ConsumedAt = &at
			return nil
		}
	}
	return ErrNotFound
}

var (
	_ Repository = (*GormRepository)(nil)
	_ Repository = (*MemoryRepository)(nil)
)
