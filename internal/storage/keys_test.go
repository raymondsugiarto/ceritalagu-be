package storage_test

import (
	"testing"

	"github.com/yourorg/music-api/internal/storage"
)

func TestKeyLayout(t *testing.T) {
	t.Parallel()
	if got := storage.UserSongKey("u1", "s1", "full.mp3"); got != "users/u1/songs/s1/full.mp3" {
		t.Errorf("user key %q", got)
	}
	if got := storage.GuestSongKey("g1", "s1", "preview.mp3"); got != "guests/g1/songs/s1/preview.mp3" {
		t.Errorf("guest key %q", got)
	}
	if got := storage.PlaygroundKey("p1", "mp3"); got != "playground/p1.mp3" {
		t.Errorf("playground key %q", got)
	}
}
