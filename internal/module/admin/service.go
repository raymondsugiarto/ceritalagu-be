package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yourorg/music-api/internal/apperror"
)

// Service signs admins in and checks sessions.
type Service struct {
	repo   Repository
	signer *Signer
	now    func() time.Time
}

// NewService builds the service.
func NewService(repo Repository, signer *Signer) *Service {
	return &Service{repo: repo, signer: signer, now: func() time.Time { return time.Now().UTC() }}
}

// Login checks the credentials and returns the admin with a session token. Every failure gets the
// same 401 so callers cannot tell a wrong password from an unknown or suspended account.
func (s *Service) Login(ctx context.Context, in LoginInput) (AdminOutput, string, error) {
	a, err := s.repo.ByEmail(ctx, in.Email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return AdminOutput{}, "", apperror.Internal(err)
	}
	if !checkPassword(a.PasswordHash, in.Password) || a.Status != StatusActive {
		return AdminOutput{}, "", apperror.Unauthorized("email atau password salah").Wrap(errBadCredentials)
	}
	now := s.now()
	if err := s.repo.TouchLogin(ctx, a.ID, now); err != nil {
		return AdminOutput{}, "", apperror.Internal(err)
	}
	a.LastLoginAt = &now
	return toOutput(a), s.signer.Issue(a), nil
}

// Authenticate turns a session token into the active admin it belongs to.
func (s *Service) Authenticate(ctx context.Context, token string) (Admin, error) {
	unauthorized := apperror.Unauthorized("silakan masuk sebagai admin")
	c, err := s.signer.parse(token)
	if err != nil {
		return Admin{}, unauthorized.Wrap(err)
	}
	a, err := s.repo.ByID(ctx, c.AdminID)
	if errors.Is(err, ErrNotFound) {
		return Admin{}, unauthorized.Wrap(err)
	}
	if err != nil {
		return Admin{}, apperror.Internal(err)
	}
	if err := c.matches(a); err != nil {
		return Admin{}, unauthorized.Wrap(err)
	}
	if a.Status != StatusActive {
		return Admin{}, unauthorized.Wrap(errors.New("admin suspended"))
	}
	return a, nil
}

// Create adds an admin (CLI only). The email must not be taken.
func (s *Service) Create(ctx context.Context, in CreateAdminInput) (AdminOutput, error) {
	in.Email = normalizeEmail(in.Email)
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Email == "" || !strings.Contains(in.Email, "@"):
		return AdminOutput{}, errors.New("a valid email is required")
	case in.Name == "":
		return AdminOutput{}, errors.New("name is required")
	case !ValidRole(in.Role):
		return AdminOutput{}, fmt.Errorf("role must be %s or %s", RoleSuperAdmin, RoleOperator)
	case utf8.RuneCountInString(in.Password) < MinPasswordLength:
		return AdminOutput{}, fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	if _, err := s.repo.ByEmail(ctx, in.Email); err == nil {
		return AdminOutput{}, fmt.Errorf("an admin with email %s already exists", in.Email)
	} else if !errors.Is(err, ErrNotFound) {
		return AdminOutput{}, err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return AdminOutput{}, err
	}
	a, err := s.repo.Create(ctx, Admin{Name: in.Name, Email: in.Email, PasswordHash: hash, Role: in.Role, Status: StatusActive})
	if err != nil {
		return AdminOutput{}, err
	}
	return toOutput(a), nil
}
