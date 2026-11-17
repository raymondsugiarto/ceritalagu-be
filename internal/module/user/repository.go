package user

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
)

// Repository-level sentinel errors. The service translates these into apperror values.
var (
	ErrNotFound   = errors.New("user not found")
	ErrEmailTaken = errors.New("email already taken")
)

// Repository is defined where it is consumed (here, by Service).
// Swap MemoryRepository for a Postgres/MySQL implementation without touching the service.
type Repository interface {
	List(ctx context.Context, p ListParams) ([]User, int64, error)
	GetByID(ctx context.Context, id string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	Create(ctx context.Context, u User) error
	Update(ctx context.Context, u User) error
	Delete(ctx context.Context, id string) error
	// MarkSignedIn records a successful customer sign-in: a draft user becomes active, the email
	// counts as verified (first time only) and last_login_at is set. Suspended users stay suspended.
	MarkSignedIn(ctx context.Context, id string, at time.Time) error
	// Promote moves the user forward to t in the funnel; a user already at t or beyond is unchanged.
	Promote(ctx context.Context, id string, t Type) error
	// Funnel counts live users per type and visitor sessions that never became a user.
	Funnel(ctx context.Context) (guests int64, types map[Type]int64, err error)
}

// MemoryRepository is a thread-safe in-memory store, used for local dev and tests.
type MemoryRepository struct {
	mu    sync.RWMutex
	byID  map[string]User
	order []string // insertion order for stable listing
}

// NewMemoryRepository returns an empty store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{byID: make(map[string]User)}
}

// List returns one page of users matching p.Search, plus the total match count.
func (r *MemoryRepository) List(_ context.Context, p ListParams) ([]User, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	search := strings.ToLower(p.Search)
	matched := make([]User, 0, len(r.order))
	for _, id := range r.order {
		u := r.byID[id]
		if p.Type != "" && u.Type != p.Type {
			continue
		}
		if search == "" || strings.Contains(strings.ToLower(u.Name), search) || strings.Contains(strings.ToLower(u.Email), search) {
			matched = append(matched, u)
		}
	}

	total := int64(len(matched))
	start := min(p.Offset, len(matched))
	end := min(start+p.Limit, len(matched))
	return matched[start:end], total, nil
}

// GetByID returns ErrNotFound if the user does not exist.
func (r *MemoryRepository) GetByID(_ context.Context, id string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.byID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

// GetByEmail returns ErrNotFound if no user has that email (case-insensitive).
func (r *MemoryRepository) GetByEmail(_ context.Context, email string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.byID {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

// Create returns ErrEmailTaken if another user has the same email (case-insensitive).
func (r *MemoryRepository) Create(_ context.Context, u User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.emailTakenLocked(u.Email, "") {
		return ErrEmailTaken
	}
	r.byID[u.ID] = u
	r.order = append(r.order, u.ID)
	return nil
}

// Update returns ErrNotFound or ErrEmailTaken.
func (r *MemoryRepository) Update(_ context.Context, u User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byID[u.ID]; !ok {
		return ErrNotFound
	}
	if r.emailTakenLocked(u.Email, u.ID) {
		return ErrEmailTaken
	}
	r.byID[u.ID] = u
	return nil
}

// Delete returns ErrNotFound if the user does not exist.
func (r *MemoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byID[id]; !ok {
		return ErrNotFound
	}
	delete(r.byID, id)
	r.order = slices.DeleteFunc(r.order, func(x string) bool { return x == id })
	return nil
}

// MarkSignedIn returns ErrNotFound if the user does not exist.
func (r *MemoryRepository) MarkSignedIn(_ context.Context, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.byID[id]
	if !ok {
		return ErrNotFound
	}
	if u.Status == StatusDraft {
		u.Status = StatusActive
	}
	u.UpdatedAt = at
	r.byID[id] = u
	return nil
}

// Promote is a no-op for an unknown user, like the SQL update.
func (r *MemoryRepository) Promote(_ context.Context, id string, t Type) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if u, ok := r.byID[id]; ok && u.Type.Rank() < t.Rank() {
		u.Type = t
		r.byID[id] = u
	}
	return nil
}

// Funnel counts users per type; the memory store has no guest sessions.
func (r *MemoryRepository) Funnel(_ context.Context) (int64, map[Type]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := map[Type]int64{}
	for _, u := range r.byID {
		types[u.Type]++
	}
	return 0, types, nil
}

func (r *MemoryRepository) emailTakenLocked(email, exceptID string) bool {
	for id, u := range r.byID {
		if id != exceptID && strings.EqualFold(u.Email, email) {
			return true
		}
	}
	return false
}
