// Package storage keeps generated audio. Local is for development; production uses an
// S3-compatible private bucket with signed URLs (same method set).
package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidKey is returned for keys that are empty or escape the root.
var ErrInvalidKey = errors.New("invalid storage key")

// Local stores objects as files under root.
type Local struct {
	root string
}

// NewLocal stores files under root (created on first write).
func NewLocal(root string) *Local { return &Local{root: root} }

// Put writes data under key, e.g. "songs/<id>/full.mp3".
func (l *Local) Put(_ context.Context, key string, data []byte, _ string) error {
	path, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create dir for %s: %w", key, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	return nil
}

// Get reads the object stored under key.
func (l *Local) Get(_ context.Context, key string) ([]byte, error) {
	path, err := l.path(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is confined to root by l.path
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	return data, nil
}

func (l *Local) path(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if key == "" || strings.Contains(key, "..") || clean == "/" {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return filepath.Join(l.root, clean), nil
}
