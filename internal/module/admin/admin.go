// Package admin is the back-office account: password login, a signed session cookie, and the
// middleware that guards every /api/v1/admin route. Admins are created with `server admin create`.
package admin

import "time"

// Role decides what an admin may change.
type Role string

// Roles (see migration 000002).
const (
	RoleSuperAdmin Role = "super_admin" // providers, API keys, prompts
	RoleOperator   Role = "operator"    // jobs, moderation
)

// Status of an admin account. Suspended admins cannot sign in and their sessions stop working.
type Status string

// Statuses.
const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

// Admin is the domain entity (table admin_user).
type Admin struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
	Role         Role
	Status       Status
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ValidRole reports whether r is a known role.
func ValidRole(r Role) bool { return r == RoleSuperAdmin || r == RoleOperator }
