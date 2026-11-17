package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yourorg/music-api/internal/storage"
)

func TestLocalPutGet(t *testing.T) {
	t.Parallel()
	s := storage.NewLocal(t.TempDir())
	ctx := context.Background()
	if err := s.Put(ctx, "songs/abc/preview.mp3", []byte("data"), "audio/mpeg"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "songs/abc/preview.mp3")
	if err != nil || string(got) != "data" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	t.Parallel()
	s := storage.NewLocal(t.TempDir())
	for _, key := range []string{"", "../etc/passwd", "songs/../../x"} {
		if err := s.Put(context.Background(), key, nil, ""); !errors.Is(err, storage.ErrInvalidKey) {
			t.Errorf("key %q: want ErrInvalidKey, got %v", key, err)
		}
	}
}
