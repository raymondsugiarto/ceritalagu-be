package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

var errInvalidToken = errors.New("invalid token")

// Token kinds; the kind is signed, so a setup token can never be used as a session.
const (
	kindSession = "session"
	kindSetup   = "setup"
)

// Keys derives every secret this module needs from the master key with fixed labels, so the API key
// encryption key itself is never used directly.
type Keys struct {
	token []byte
	otp   []byte
}

// NewKeys derives the keys.
func NewKeys(masterKey string) Keys {
	derive := func(label string) []byte {
		mac := hmac.New(sha256.New, []byte(masterKey))
		mac.Write([]byte(label))
		return mac.Sum(nil)
	}
	return Keys{token: derive("customer-token-v1"), otp: derive("customer-otp-v1")}
}

// issue returns base64url(kind|subject|expiry|stamp) + "." + HMAC-SHA256.
func (k Keys) issue(kind, subject, stamp string, exp time.Time) string {
	payload := kind + "|" + subject + "|" + strconv.FormatInt(exp.Unix(), 10) + "|" + stamp
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return enc + "." + k.sign(enc)
}

type tokenClaims struct {
	Subject string
	Stamp   string
}

// parse checks the signature, kind and expiry.
func (k Keys) parse(token, kind string, now time.Time) (tokenClaims, error) {
	enc, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(k.sign(enc))) {
		return tokenClaims{}, errInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return tokenClaims{}, errInvalidToken
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 || parts[0] != kind {
		return tokenClaims{}, errInvalidToken
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || now.Unix() >= exp {
		return tokenClaims{}, errInvalidToken
	}
	return tokenClaims{Subject: parts[1], Stamp: parts[3]}, nil
}

func (k Keys) sign(enc string) string {
	mac := hmac.New(sha256.New, k.token)
	mac.Write([]byte(enc))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// hashOTP binds the code to the email and purpose: HMAC(email|purpose|code).
func (k Keys) hashOTP(email, purpose, code string) string {
	mac := hmac.New(sha256.New, k.otp)
	mac.Write([]byte(email + "|" + purpose + "|" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

// stamp ties a session to the current password: changing it ends every session.
func stamp(passwordHash string) string {
	sum := sha256.Sum256([]byte(passwordHash))
	return hex.EncodeToString(sum[:6])
}

// newCode returns a uniformly random numeric code of OTPLength digits.
func newCode() (string, error) {
	limit := big.NewInt(1)
	for range OTPLength {
		limit.Mul(limit, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("random otp: %w", err)
	}
	return fmt.Sprintf("%0*d", OTPLength, n), nil
}
