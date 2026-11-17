package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// errInvalidSession covers a missing, forged, expired or outdated session token.
var errInvalidSession = errors.New("invalid admin session")

// Signer issues and checks session tokens: base64url(id|expiry|stamp) + "." + HMAC-SHA256.
// The stamp is derived from the password hash, so changing the password ends every session.
// Tokens are stateless; the middleware still loads the admin on each request, so a suspended
// account is locked out at once.
type Signer struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewSigner derives the signing key from the master key with a fixed label, so the API key
// encryption key itself is never used for signing.
func NewSigner(masterKey string, ttl time.Duration) *Signer {
	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte("admin-session-v1"))
	return &Signer{key: mac.Sum(nil), ttl: ttl, now: time.Now}
}

// TTL is how long a session lasts.
func (s *Signer) TTL() time.Duration { return s.ttl }

// Issue returns a token for a.
func (s *Signer) Issue(a Admin) string {
	exp := s.now().Add(s.ttl).Unix()
	payload := a.ID + "|" + strconv.FormatInt(exp, 10) + "|" + stamp(a.PasswordHash)
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return enc + "." + s.sign(enc)
}

// claims is what a valid token says.
type claims struct {
	AdminID string
	Stamp   string
}

// parse checks the signature and expiry.
func (s *Signer) parse(token string) (claims, error) {
	enc, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(enc))) {
		return claims{}, errInvalidSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return claims{}, errInvalidSession
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return claims{}, errInvalidSession
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || s.now().Unix() >= exp {
		return claims{}, errInvalidSession
	}
	return claims{AdminID: parts[0], Stamp: parts[2]}, nil
}

func (s *Signer) sign(enc string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(enc))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func stamp(passwordHash string) string {
	sum := sha256.Sum256([]byte(passwordHash))
	return hex.EncodeToString(sum[:6])
}

// matches reports whether c still belongs to a (same password).
func (c claims) matches(a Admin) error {
	if c.Stamp != stamp(a.PasswordHash) {
		return fmt.Errorf("%w: password changed", errInvalidSession)
	}
	return nil
}
