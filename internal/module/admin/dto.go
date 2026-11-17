package admin

import "time"

// LoginInput is the sign-in form.
type LoginInput struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,max=200"`
}

// AdminOutput is the signed-in admin (never the password hash).
type AdminOutput struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        Role       `json:"role"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

func toOutput(a Admin) AdminOutput {
	return AdminOutput{ID: a.ID, Name: a.Name, Email: a.Email, Role: a.Role, LastLoginAt: a.LastLoginAt}
}

// CreateAdminInput is used by the `server admin create` command.
type CreateAdminInput struct {
	Name     string
	Email    string
	Password string
	Role     Role
}
