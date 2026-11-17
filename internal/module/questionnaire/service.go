package questionnaire

import (
	"context"
	"errors"
	"time"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/masterdata"
)

// Repository is the storage the service needs (GormRepository).
type Repository interface {
	Create(ctx context.Context, q Questionnaire) (Questionnaire, error)
	Get(ctx context.Context, id string) (Questionnaire, error)
}

// GuestSessions creates the guest_session row on first write.
type GuestSessions interface {
	Ensure(ctx context.Context, v guest.Visitor) error
}

// Options checks answer codes against the admin master data (masterdata.Service).
type Options interface {
	IsActiveCode(ctx context.Context, k masterdata.Kind, code string) (bool, error)
}

// Users creates the user from the contact step, or finds it by email (user.Service).
type Users interface {
	EnsureContact(ctx context.Context, email, phone string) (string, error)
}

// Service holds the questionnaire rules.
type Service struct {
	repo    Repository
	guests  GuestSessions
	options Options
	users   Users
	now     func() time.Time
}

// NewService builds the service.
func NewService(repo Repository, guests GuestSessions, options Options, users Users) *Service {
	return &Service{repo: repo, guests: guests, options: options, users: users,
		now: func() time.Time { return time.Now().UTC() }}
}

// Create stores submitted answers for the visitor's guest session.
func (s *Service) Create(ctx context.Context, v guest.Visitor, in CreateQuestionnaireInput) (QuestionnaireOutput, error) {
	a := in.toAnswers()
	if err := s.checkOptions(ctx, &a); err != nil {
		return QuestionnaireOutput{}, err
	}
	if err := s.guests.Ensure(ctx, v); err != nil {
		return QuestionnaireOutput{}, apperror.Internal(err)
	}
	userID, err := s.users.EnsureContact(ctx, in.Email, in.Phone)
	if err != nil {
		return QuestionnaireOutput{}, err
	}
	now := s.now()
	q, err := s.repo.Create(ctx, Questionnaire{GuestSessionID: v.SessionID, UserID: userID, Answers: a, SubmittedAt: &now})
	if err != nil {
		return QuestionnaireOutput{}, apperror.Internal(err)
	}
	return toQuestionnaireOutput(q), nil
}

// Get returns the visitor's own questionnaire. Someone else's is reported as not found.
func (s *Service) Get(ctx context.Context, v guest.Visitor, id string) (QuestionnaireOutput, error) {
	q, err := s.Owned(ctx, v, id)
	if err != nil {
		return QuestionnaireOutput{}, err
	}
	return toQuestionnaireOutput(q), nil
}

// Owned loads a questionnaire owned by v (used by the song module before creating a job).
func (s *Service) Owned(ctx context.Context, v guest.Visitor, id string) (Questionnaire, error) {
	q, err := s.repo.Get(ctx, id)
	if errors.Is(err, ErrNotFound) || (err == nil && q.GuestSessionID != v.SessionID) {
		return Questionnaire{}, apperror.NotFound("questionnaire not found")
	}
	if err != nil {
		return Questionnaire{}, apperror.Internal(err)
	}
	return q, nil
}

// checkOptions rejects codes that are not active master data. Recipient and occasion also accept
// "other" with free text ("Lainnya"); genre and mood have no "other"; mood may be empty. Free text is cleared when unused.
func (s *Service) checkOptions(ctx context.Context, a *Answers) error {
	var details []apperror.ErrorDetail
	check := func(k masterdata.Kind, field, code, otherField string, other *string) error {
		if other != nil && code == masterdata.OtherCode {
			if *other == "" {
				details = append(details, apperror.ErrorDetail{Field: otherField, Message: "is required"})
			}
			return nil
		}
		if other != nil {
			*other = ""
		}
		ok, err := s.options.IsActiveCode(ctx, k, code)
		if err != nil {
			return apperror.Internal(err)
		}
		if !ok {
			details = append(details, apperror.ErrorDetail{Field: field, Message: "pilihan ini sudah tidak tersedia"})
		}
		return nil
	}
	if err := check(masterdata.KindRecipientType, "recipient_type", a.RecipientType, "recipient_other", &a.RecipientOther); err != nil {
		return err
	}
	if err := check(masterdata.KindOccasion, "occasion", a.Occasion, "occasion_other", &a.OccasionOther); err != nil {
		return err
	}
	if err := check(masterdata.KindGenre, "genre", a.Genre, "", nil); err != nil {
		return err
	}
	if a.Mood != "" { // mood is optional
		if err := check(masterdata.KindMood, "mood", a.Mood, "", nil); err != nil {
			return err
		}
	}
	if len(details) > 0 {
		return apperror.Validation(details)
	}
	return nil
}
