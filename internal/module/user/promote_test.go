package user_test

import (
	"context"
	"testing"

	"github.com/yourorg/music-api/internal/module/user"
)

func TestPromoteOnlyMovesForward(t *testing.T) {
	ctx := context.Background()
	repo := user.NewMemoryRepository()
	svc := user.NewService(repo)
	id, err := svc.EnsureContact(ctx, "a@b.c", "0812")
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		to, want user.Type
	}{
		{user.TypeProspect, user.TypeProspect},
		{user.TypeCustomer, user.TypeCustomer},
		{user.TypeProspect, user.TypeCustomer}, // never back
		{user.TypeRepeatCustomer, user.TypeRepeatCustomer},
	}
	for _, s := range steps {
		if err := svc.Promote(ctx, id, s.to); err != nil {
			t.Fatal(err)
		}
		u, _ := repo.GetByID(ctx, id)
		if u.Type != s.want {
			t.Fatalf("after promote to %s: type = %s, want %s", s.to, u.Type, s.want)
		}
	}
}

func TestEnsureContactCreatesLead(t *testing.T) {
	ctx := context.Background()
	repo := user.NewMemoryRepository()
	id, err := user.NewService(repo).EnsureContact(ctx, "x@y.z", "")
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := repo.GetByID(ctx, id); u.Type != user.TypeLead {
		t.Fatalf("type = %s, want lead", u.Type)
	}
}
