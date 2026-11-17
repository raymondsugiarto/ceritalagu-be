package admin

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is enforced when an admin is created.
const MinPasswordLength = 12

// bcryptCost is a var so tests can lower it.
var bcryptCost = 12

// dummyHash is compared against when the email is unknown, so a miss takes as long as a wrong
// password and response times don't reveal which emails exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcryptCost)

// HashPassword returns the bcrypt hash of plain.
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}

// checkPassword reports whether plain matches hash. An empty hash is checked against dummyHash.
func checkPassword(hash, plain string) bool {
	h := []byte(hash)
	if len(h) == 0 {
		h = dummyHash
	}
	err := bcrypt.CompareHashAndPassword(h, []byte(plain))
	return err == nil && hash != ""
}

// errBadCredentials is returned for every failed login, whatever the reason.
var errBadCredentials = errors.New("bad credentials")
