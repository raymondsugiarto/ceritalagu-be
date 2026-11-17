package song

import (
	"context"
	"os"
	"testing"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/database"
	"github.com/yourorg/music-api/internal/storage"
)

func TestLiveAdminSongs(t *testing.T) {
	_ = os.Chdir("../../..")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), cfg.Database.Main, "")
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewAdminSongService(NewGormRepository(db), store)
	ctx := context.Background()
	st, err := svc.Stats(ctx)
	t.Logf("stats=%+v err=%v", st, err)
	for _, acc := range []SongAccess{AccessAll, AccessUnlocked, AccessLocked} {
		l, err := svc.ListSongs(ctx, ListSongsInput{Access: acc})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: total=%d items=%d", acc, l.Pagination.TotalItems, len(l.Items))
		for i, s := range l.Items {
			if i < 3 {
				t.Logf("  %s %q user=%q unlocked=%v dur=%d lyrics=%d", s.ID, s.Title, s.UserEmail, s.IsUnlocked, s.DurationSec, len(s.Lyrics))
			}
		}
		if acc == AccessAll && len(l.Items) > 0 {
			id := l.Items[0].ID
			p, _, err := svc.Audio(ctx, id, false)
			f, _, err2 := svc.Audio(ctx, id, true)
			t.Logf("audio preview=%d bytes err=%v full=%d bytes err=%v", len(p), err, len(f), err2)
			s, _ := svc.ListSongs(ctx, ListSongsInput{Search: l.Items[0].Title[:3]})
			t.Logf("search %q -> %d", l.Items[0].Title[:3], s.Pagination.TotalItems)
		}
	}
}
