package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type model struct {
	ID        string     `gorm:"column:id;primaryKey"`
	Name      string     `gorm:"column:name"`
	Email     string     `gorm:"column:email"`
	Phone     string     `gorm:"column:phone"`
	Status    string     `gorm:"column:status"`
	Type      string     `gorm:"column:customer_type"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

func (model) TableName() string { return `"user"` }

// listRow adds the admin list facts read from other tables.
type listRow struct {
	Model       model `gorm:"embedded"` // named: an unexported embedded type cannot be set by GORM
	HasPassword bool  `gorm:"column:has_password"`
	PaidOrders  int   `gorm:"column:paid_orders"`
}

// GormRepository stores users in PostgreSQL. Email uniqueness is checked here, not by the DB.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) live(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&model{}).Where("deleted_at IS NULL")
}

// List returns one page of users matching p.Search (name or email), newest first.
func (r *GormRepository) List(ctx context.Context, p ListParams) ([]User, int64, error) {
	q := r.live(ctx)
	if p.Search != "" {
		like := "%" + strings.ToLower(p.Search) + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(email) LIKE ? OR phone LIKE ?", like, like, like)
	}
	if p.Type != "" {
		q = q.Where("customer_type = ?", string(p.Type))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}
	var rows []listRow
	err := q.Select(`"user".*,
  EXISTS (SELECT 1 FROM user_credential c WHERE c.user_id = "user".id AND c.deleted_at IS NULL) AS has_password,
  (SELECT COUNT(*) FROM payment_order o WHERE o.user_id = "user".id AND o.status = 'paid' AND o.deleted_at IS NULL) AS paid_orders`).
		Order("created_at DESC").Offset(p.Offset).Limit(p.Limit).Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	out := make([]User, 0, len(rows))
	for _, m := range rows {
		u := m.Model.toUser()
		u.HasPassword, u.PaidOrders = m.HasPassword, m.PaidOrders
		out = append(out, u)
	}
	return out, total, nil
}

// GetByID returns ErrNotFound if the user does not exist.
func (r *GormRepository) GetByID(ctx context.Context, id string) (User, error) {
	return r.first(ctx, "id = ?", id)
}

// GetByEmail returns ErrNotFound if no user has that email (emails are stored lower-cased).
func (r *GormRepository) GetByEmail(ctx context.Context, email string) (User, error) {
	return r.first(ctx, "email = ?", strings.ToLower(email))
}

// Create returns ErrEmailTaken if another user has the same email.
func (r *GormRepository) Create(ctx context.Context, u User) error {
	if err := r.checkEmail(ctx, u.Email, ""); err != nil {
		return err
	}
	m := fromUser(u)
	if m.Status == "" {
		m.Status = string(StatusActive)
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// Update returns ErrNotFound or ErrEmailTaken.
func (r *GormRepository) Update(ctx context.Context, u User) error {
	if err := r.checkEmail(ctx, u.Email, u.ID); err != nil {
		return err
	}
	res := r.live(ctx).Where("id = ?", u.ID).Updates(map[string]any{
		"name": u.Name, "email": u.Email, "phone": u.Phone, "status": string(u.Status), "customer_type": string(u.Type),
		"updated_at": u.UpdatedAt,
	})
	if res.Error != nil {
		return fmt.Errorf("update user %s: %w", u.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete is a soft delete: it sets deleted_at.
func (r *GormRepository) Delete(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res := r.live(ctx).Where("id = ?", id).Updates(map[string]any{"deleted_at": now, "updated_at": now})
	if res.Error != nil {
		return fmt.Errorf("soft-delete user %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Promote moves the user to t unless they are already at t or further along the funnel.
func (r *GormRepository) Promote(ctx context.Context, id string, t Type) error {
	ranks := "CASE COALESCE(customer_type, '')"
	args := []any{}
	for _, x := range Types {
		ranks += " WHEN ? THEN ?"
		args = append(args, string(x), x.Rank())
	}
	ranks += " ELSE 0 END"
	res := r.live(ctx).Where("id = ?", id).Where(ranks+" < ?", append(args, t.Rank())...).
		Updates(map[string]any{"customer_type": string(t), "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return fmt.Errorf("promote user %s to %s: %w", id, t, res.Error)
	}
	return nil
}

// Funnel counts live users per customer_type, plus guest sessions with no user: not claimed by a
// sign-in and with no questionnaire that left a contact. Read-only raw SELECTs.
func (r *GormRepository) Funnel(ctx context.Context) (int64, map[Type]int64, error) {
	var rows []struct {
		Type string
		N    int64
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT COALESCE(customer_type, '') AS type, COUNT(*) AS n FROM "user"
WHERE deleted_at IS NULL GROUP BY customer_type`).Scan(&rows).Error; err != nil {
		return 0, nil, fmt.Errorf("count users by type: %w", err)
	}
	types := make(map[Type]int64, len(rows))
	for _, x := range rows {
		types[Type(x.Type)] += x.N
	}
	var guests int64
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM guest_session g
WHERE g.deleted_at IS NULL AND COALESCE(g.claimed_by_user_id, '') = ''
  AND NOT EXISTS (SELECT 1 FROM questionnaire q WHERE q.guest_session_id = g.id
                  AND COALESCE(q.user_id, '') <> '' AND q.deleted_at IS NULL)`).Scan(&guests).Error; err != nil {
		return 0, nil, fmt.Errorf("count guests: %w", err)
	}
	return guests, types, nil
}

// MarkSignedIn activates a draft user, sets email_verified_at once and last_login_at every time.
func (r *GormRepository) MarkSignedIn(ctx context.Context, id string, at time.Time) error {
	res := r.live(ctx).Where("id = ?", id).Updates(map[string]any{
		"status":            gorm.Expr("CASE WHEN status = ? THEN ? ELSE status END", string(StatusDraft), string(StatusActive)),
		"email_verified_at": gorm.Expr("COALESCE(email_verified_at, ?)", at),
		"last_login_at":     at,
		"updated_at":        at,
	})
	if res.Error != nil {
		return fmt.Errorf("mark user %s signed in: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) first(ctx context.Context, where string, arg any) (User, error) {
	var m model
	err := r.live(ctx).Where(where, arg).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("select user: %w", err)
	}
	return m.toUser(), nil
}

func (r *GormRepository) checkEmail(ctx context.Context, email, exceptID string) error {
	q := r.live(ctx).Where("email = ?", strings.ToLower(email))
	if exceptID != "" {
		q = q.Where("id <> ?", exceptID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return fmt.Errorf("check user email: %w", err)
	}
	if n > 0 {
		return ErrEmailTaken
	}
	return nil
}

func (m model) toUser() User {
	return User{ID: m.ID, Name: m.Name, Email: m.Email, Phone: m.Phone, Status: Status(m.Status), Type: Type(m.Type), CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func fromUser(u User) model {
	return model{ID: u.ID, Name: u.Name, Email: u.Email, Phone: u.Phone, Status: string(u.Status), Type: string(u.Type), CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

var _ Repository = (*GormRepository)(nil)
