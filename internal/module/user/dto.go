package user

import (
	"time"

	"github.com/yourorg/music-api/internal/pagination"
)

// Naming convention:
//   - <Action><Entity>Input  = payload the handler receives (body, query, path)
//   - <Entity>Output         = what the handler returns inside "data"

// CreateUserInput is the body of POST /users.
type CreateUserInput struct {
	Name  string `json:"name"  validate:"required,min=2,max=100"`
	Email string `json:"email" validate:"required,email,max=254"`
}

// UpdateUserInput uses pointers so "field omitted" and "field set to empty" are distinguishable (PATCH semantics).
type UpdateUserInput struct {
	Name  *string `json:"name"  validate:"omitempty,min=2,max=100"`
	Email *string `json:"email" validate:"omitempty,email,max=254"`
}

// ListUsersInput is the query of GET /users: ?page=&page_size=&search=
type ListUsersInput struct {
	pagination.Input
	Search string `query:"search" validate:"omitempty,max=100"`
	Type   string `query:"type"   validate:"omitempty,oneof=lead prospect customer repeat_customer"`
}

// UserStatsOutput counts the customer funnel for the admin (GET /users/stats).
type UserStatsOutput struct {
	Guests int64          `json:"guests"` // visitor sessions that never left a contact
	Types  map[Type]int64 `json:"types"`
}

// UserOutput is a user as returned in "data".
type UserOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	Status string `json:"status"`
	// Type is the funnel stage: lead | prospect | customer | repeat_customer.
	Type        string    `json:"type"`
	HasPassword bool      `json:"has_password"`
	PaidOrders  int       `json:"paid_orders"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// toUserOutput maps the entity to its response shape.
func toUserOutput(u User) UserOutput {
	return UserOutput{ID: u.ID, Name: u.Name, Email: u.Email, Phone: u.Phone, Status: string(u.Status),
		Type: string(u.Type), HasPassword: u.HasPassword, PaidOrders: u.PaidOrders, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

func toUserOutputs(us []User) []UserOutput {
	out := make([]UserOutput, 0, len(us))
	for _, u := range us {
		out = append(out, toUserOutput(u))
	}
	return out
}
