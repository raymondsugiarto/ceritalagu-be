// Package secret encrypts provider API keys at rest with AES-256-GCM.
// The 32-byte master key comes from config (env), never from the database.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrInvalidCiphertext is returned when a stored value cannot be decrypted with the master key.
var ErrInvalidCiphertext = errors.New("invalid ciphertext")

// Box encrypts and decrypts short secrets.
type Box struct {
	aead cipher.AEAD
}

// NewBox takes a base64-encoded 32-byte key (e.g. `openssl rand -base64 32`).
func NewBox(base64Key string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decode master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Encrypt returns base64(nonce || ciphertext).
func (b *Box) Encrypt(plain string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt.
func (b *Box) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", ErrInvalidCiphertext
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	return string(plain), nil
}

// Last4 is the suffix shown in the admin UI (sk-****abcd).
func Last4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}
