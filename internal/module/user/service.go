package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/pagination"
)

// Service holds the user business rules.
type Service struct {
	repo Repository
	now  func() time.Time // injectable clock for tests
}

// NewService builds the service with a UTC clock.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// List returns one page of users, optionally filtered by name or email.
func (s *Service) List(ctx context.Context, in ListUsersInput) (pagination.ListOutput[UserOutput], error) {
	in.Normalize()
	users, total, err := s.repo.List(ctx, ListParams{
		Search: strings.TrimSpace(in.Search),
		Type:   Type(in.Type),
		Offset: in.Offset(),
		Limit:  in.PageSize,
	})
	if err != nil {
		return pagination.ListOutput[UserOutput]{}, apperror.Internal(err)
	}
	return pagination.NewList(toUserOutputs(users), in.Input, total), nil
}

// Get returns one user, or a 404 error.
func (s *Service) Get(ctx context.Context, id string) (UserOutput, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return UserOutput{}, mapRepoErr(err)
	}
	return toUserOutput(u), nil
}

// Create adds a user. The email is stored lowercased and must be unique (409).
func (s *Service) Create(ctx context.Context, in CreateUserInput) (UserOutput, error) {
	now := s.now()
	u := User{
		ID:        uuid.NewString(),
		Name:      strings.TrimSpace(in.Name),
		Email:     normalizeEmail(in.Email),
		Status:    StatusActive,
		Type:      TypeLead,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return UserOutput{}, mapRepoErr(err)
	}
	return toUserOutput(u), nil
}

// EnsureContact returns the id of the user with this email, creating a draft user when it is new
// (the guest's last questionnaire step). An existing user keeps their status; a changed phone
// number is saved.
func (s *Service) EnsureContact(ctx context.Context, email, phone string) (string, error) {
	email, phone = normalizeEmail(email), NormalizePhone(phone)
	u, err := s.repo.GetByEmail(ctx, email)
	switch {
	case err == nil:
		if phone != "" && u.Phone != phone {
			u.Phone, u.UpdatedAt = phone, s.now()
			if err := s.repo.Update(ctx, u); err != nil {
				return "", mapRepoErr(err)
			}
		}
		return u.ID, nil
	case errors.Is(err, ErrNotFound):
		now := s.now()
		u = User{ID: uuid.NewString(), Email: email, Phone: phone, Status: StatusDraft, Type: TypeLead, CreatedAt: now, UpdatedAt: now}
		if err := s.repo.Create(ctx, u); err != nil {
			if errors.Is(err, ErrEmailTaken) { // created by a parallel request
				return s.EnsureContact(ctx, email, phone)
			}
			return "", mapRepoErr(err)
		}
		return u.ID, nil
	default:
		return "", apperror.Internal(err)
	}
}

// Stats returns the customer funnel counts.
func (s *Service) Stats(ctx context.Context) (UserStatsOutput, error) {
	guests, types, err := s.repo.Funnel(ctx)
	if err != nil {
		return UserStatsOutput{}, apperror.Internal(err)
	}
	return UserStatsOutput{Guests: guests, Types: types}, nil
}

// Promote moves the user forward in the funnel (never back). Used by auth (password set) and
// payment (checkout started, order paid).
func (s *Service) Promote(ctx context.Context, id string, t Type) error {
	if err := s.repo.Promote(ctx, id, t); err != nil {
		return apperror.Internal(err)
	}
	return nil
}

// NormalizePhone keeps a leading + and the digits: "+62 812-3456 7890" -> "+6281234567890".
func NormalizePhone(p string) string {
	p = strings.TrimSpace(p)
	var b strings.Builder
	for i, r := range p {
		if (r == '+' && i == 0) || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Update changes only the fields set in in (PATCH semantics).
func (s *Service) Update(ctx context.Context, id string, in UpdateUserInput) (UserOutput, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return UserOutput{}, mapRepoErr(err)
	}
	if in.Name != nil {
		u.Name = strings.TrimSpace(*in.Name)
	}
	if in.Email != nil {
		u.Email = normalizeEmail(*in.Email)
	}
	u.UpdatedAt = s.now()

	if err := s.repo.Update(ctx, u); err != nil {
		return UserOutput{}, mapRepoErr(err)
	}
	return toUserOutput(u), nil
}

// Delete removes a user, or returns a 404 error.
func (s *Service) Delete(ctx context.Context, id string) error {
	return mapRepoErr(s.repo.Delete(ctx, id))
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// mapRepoErr is the one place where storage errors become client-facing errors.
func mapRepoErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("user not found")
	case errors.Is(err, ErrEmailTaken):
		return apperror.Conflict("email is already in use")
	default:
		return apperror.Internal(err)
	}
}
