// Package auth signs customers in.
//
// A customer is created by the questionnaire contact step (email + phone) as a draft user without a
// password. Their first sign-in uses a 6-digit code sent by email; after the code is accepted they set
// a password, which creates their user_credential row. From then on they sign in with email + password.
//
//	POST /auth/lookup        email → "otp" (no password yet) or "password"
//	POST /auth/otp           send a login code (users without a password)
//	POST /auth/otp/verify    code → short-lived setup token
//	POST /auth/password      setup token + new password → user_credential + session
//	POST /auth/login         email + password → session
//	POST /auth/logout        clear the session
//	GET  /auth/me            the signed-in customer
package auth

import "time"

// Credential is the customer's password (table user_credential). At most one live row per user.
type Credential struct {
	ID            string
	UserID        string
	PasswordHash  string
	PasswordSetAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PurposeLogin is the only OTP purpose today.
const PurposeLogin = "login"

// OTP is one emailed login code (table user_otp). Only its HMAC is stored.
type OTP struct {
	ID         string
	UserID     string
	Email      string
	Purpose    string
	CodeHash   string
	Attempts   int
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// Method is how an email signs in.
type Method string

// Sign-in methods returned by lookup.
const (
	MethodOTP      Method = "otp"      // no password yet: email a code, then set one
	MethodPassword Method = "password" // has a user_credential
)

// Rules for codes, tokens and passwords.
const (
	OTPLength         = 6
	OTPTTL            = 10 * time.Minute
	OTPResendAfter    = 60 * time.Second
	OTPMaxAttempts    = 5
	SetupTokenTTL     = 15 * time.Minute
	SessionTTL        = 30 * 24 * time.Hour
	MinPasswordLength = 8
	MaxPasswordLength = 72 // bcrypt ignores bytes past 72
)
