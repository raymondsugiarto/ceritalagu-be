package user_test

import (
	"context"
	"testing"

	"github.com/yourorg/music-api/internal/module/user"
)

func TestEnsureContactCreatesThenReuses(t *testing.T) {
	t.Parallel()
	repo := user.NewMemoryRepository()
	svc := user.NewService(repo)
	ctx := context.Background()

	id, err := svc.EnsureContact(ctx, " Nadia@Example.com ", "+62 812-3456-7890")
	if err != nil || id == "" {
		t.Fatalf("create: %q %v", id, err)
	}
	u, _ := repo.GetByID(ctx, id)
	if u.Email != "nadia@example.com" || u.Phone != "+6281234567890" || u.Status != user.StatusDraft {
		t.Fatalf("stored %+v", u)
	}

	again, err := svc.EnsureContact(ctx, "nadia@example.com", "0812 0000 1111")
	if err != nil || again != id {
		t.Fatalf("same email should reuse the user: %q vs %q (%v)", again, id, err)
	}
	u, _ = repo.GetByID(ctx, id)
	if u.Phone != "081200001111" || u.Status != user.StatusDraft {
		t.Fatalf("phone not updated: %+v", u)
	}
}

func TestNormalizePhone(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"+62 812-3456": "+628123456", "(021) 555 1234": "0215551234", "62+81": "6281"} {
		if got := user.NormalizePhone(in); got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
}
