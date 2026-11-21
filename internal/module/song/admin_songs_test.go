package song_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/yourorg/music-api/internal/module/song"
)

// adminRepo is a minimal AdminSongRepository for the take-2 audio path.
type adminRepo struct {
	songs map[string]song.Song
	alts  map[string]song.Alternate // by song id
}

func (r adminRepo) ListAdminSongs(context.Context, song.AdminSongParams) ([]song.AdminSong, int64, error) {
	return nil, 0, nil
}
func (r adminRepo) CountSongs(context.Context) (song.SongStatsOutput, error) {
	return song.SongStatsOutput{}, nil
}
func (r adminRepo) GetSong(_ context.Context, id string) (song.Song, error) {
	if s, ok := r.songs[id]; ok {
		return s, nil
	}
	return song.Song{}, song.ErrSongNotFound
}
func (r adminRepo) SongAnswers(context.Context, string) (song.AdminSongAnswersOutput, error) {
	return song.AdminSongAnswersOutput{}, nil
}
func (r adminRepo) SongAlternate(_ context.Context, id string) (song.Alternate, error) {
	if a, ok := r.alts[id]; ok {
		return a, nil
	}
	return song.Alternate{}, song.ErrAlternateNotFound
}

type keyAudio struct{}

func (keyAudio) Get(_ context.Context, key string) ([]byte, error) { return []byte(key), nil }

func TestAdminAlternateAudio(t *testing.T) {
	ctx := context.Background()
	repo := adminRepo{
		songs: map[string]song.Song{"s1": {ID: "s1", Title: "A"}, "s2": {ID: "s2", Title: "B"}},
		alts:  map[string]song.Alternate{"s1": {ID: "a1", FullAudioKey: "alt/full.mp3", PreviewAudioKey: "alt/preview.mp3"}},
	}
	svc := song.NewAdminSongService(repo, keyAudio{})

	data, sg, err := svc.AlternateAudio(ctx, "s1", true)
	if err != nil || string(data) != "alt/full.mp3" || sg.Title != "A" {
		t.Fatalf("full take 2 = %q, %q, %v", data, sg.Title, err)
	}
	if data, _, _ := svc.AlternateAudio(ctx, "s1", false); string(data) != "alt/preview.mp3" {
		t.Fatalf("preview take 2 = %q", data)
	}
	if _, _, err := svc.AlternateAudio(ctx, "s2", true); status(err) != http.StatusNotFound {
		t.Fatalf("song without take 2: status %d, want 404", status(err))
	}
	if _, _, err := svc.AlternateAudio(ctx, "missing", true); status(err) != http.StatusNotFound {
		t.Fatalf("missing song: status %d, want 404", status(err))
	}
}
