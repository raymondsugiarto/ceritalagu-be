package secret_test

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/yourorg/music-api/internal/secret"
)

func newKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	return base64.StdEncoding.EncodeToString(k)
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	box, err := secret.NewBox(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := box.Encrypt("sk-live-1234abcd")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := box.Decrypt(enc); err != nil || got != "sk-live-1234abcd" {
		t.Fatalf("got %q, %v", got, err)
	}
	if secret.Last4("sk-live-1234abcd") != "abcd" {
		t.Fatal("Last4")
	}
}

func TestWrongKeyAndBadInput(t *testing.T) {
	t.Parallel()
	a, _ := secret.NewBox(newKey(t))
	b, _ := secret.NewBox(newKey(t))
	enc, _ := a.Encrypt("x")
	if _, err := b.Decrypt(enc); !errors.Is(err, secret.ErrInvalidCiphertext) {
		t.Fatalf("want ErrInvalidCiphertext, got %v", err)
	}
	if _, err := secret.NewBox(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("short key must fail")
	}
}
