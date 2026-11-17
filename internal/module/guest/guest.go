// Package guest identifies anonymous visitors with the guest_session_id cookie (PRD §5.5).
// Songs created as a guest belong to that session until a user claims them on register/login.
package guest

import "time"

// Session is the domain entity (table guest_session).
type Session struct {
	ID              string
	IPHash          string
	UserAgent       string
	ClaimedByUserID string
	ClaimedAt       *time.Time
	LastSeenAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Visitor is who is calling, resolved by the middleware for every /api/v1 request.
type Visitor struct {
	SessionID string // from the cookie, or freshly minted
	IPHash    string
	UserAgent string
}
