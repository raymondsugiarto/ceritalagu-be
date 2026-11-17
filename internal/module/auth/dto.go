package auth

import "github.com/yourorg/music-api/internal/module/user"

// EmailInput is the body of POST /auth/lookup and POST /auth/otp.
type EmailInput struct {
	Email string `json:"email" validate:"required,email,max=254"`
}

// VerifyOTPInput is the body of POST /auth/otp/verify.
type VerifyOTPInput struct {
	Email string `json:"email" validate:"required,email,max=254"`
	Code  string `json:"code"  validate:"required,len=6,numeric"`
}

// SetPasswordInput is the body of POST /auth/password.
type SetPasswordInput struct {
	SetupToken string `json:"setup_token" validate:"required,max=500"`
	Password   string `json:"password"    validate:"required,max=200"` // length rules (Indonesian message) in checkPassword
}

// LoginInput is the body of POST /auth/login.
type LoginInput struct {
	Email    string `json:"email"    validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,max=72"`
}

// LookupOutput tells the login form what to show next.
type LookupOutput struct {
	Email  string `json:"email"`
	Method Method `json:"method"`
}

// OTPSentOutput is returned after a code was emailed.
type OTPSentOutput struct {
	Email          string `json:"email"`
	ExpiresInSec   int    `json:"expires_in_sec"`
	ResendAfterSec int    `json:"resend_after_sec"`
}

// OTPVerifiedOutput carries the token that allows setting the first password.
type OTPVerifiedOutput struct {
	SetupToken   string `json:"setup_token"`
	ExpiresInSec int    `json:"expires_in_sec"`
}

// CustomerOutput is the signed-in customer.
type CustomerOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	Status string `json:"status"`
}

func toCustomerOutput(u user.User) CustomerOutput {
	return CustomerOutput{ID: u.ID, Name: u.Name, Email: u.Email, Phone: u.Phone, Status: string(u.Status)}
}
