package user

import (
	"context"
	"errors"
	"testing"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/pagination"
)

func ptr[T any](v T) *T { return &v }

func TestService_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		seed     []CreateUserInput
		req      CreateUserInput
		wantCode apperror.Code // empty = success
	}{
		{name: "creates user and normalizes email", req: CreateUserInput{Name: " Ada ", Email: " ADA@Example.com "}},
		{
			name:     "rejects duplicate email case-insensitively",
			seed:     []CreateUserInput{{Name: "Ada", Email: "ada@example.com"}},
			req:      CreateUserInput{Name: "Other", Email: "ADA@example.com"},
			wantCode: apperror.CodeConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			svc := NewService(NewMemoryRepository())
			for _, s := range tt.seed {
				if _, err := svc.Create(ctx, s); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}

			got, err := svc.Create(ctx, tt.req)

			if tt.wantCode != "" {
				assertCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != "Ada" || got.Email != "ada@example.com" {
				t.Errorf("got %+v, want trimmed name and lower-cased email", got)
			}
		})
	}
}

func TestService_UpdateAndDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(NewMemoryRepository())

	u, err := svc.Create(ctx, CreateUserInput{Name: "Ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.Update(ctx, u.ID, UpdateUserInput{Name: ptr("Ada Lovelace")})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Ada Lovelace" || updated.Email != "ada@example.com" {
		t.Errorf("partial update changed wrong fields: %+v", updated)
	}

	if err := svc.Delete(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Get(ctx, u.ID)
	assertCode(t, err, apperror.CodeNotFound)
}

func TestService_ListPaginates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(NewMemoryRepository())
	for _, e := range []string{"a@x.io", "b@x.io", "c@x.io"} {
		if _, err := svc.Create(ctx, CreateUserInput{Name: "User", Email: e}); err != nil {
			t.Fatal(err)
		}
	}

	out, err := svc.List(ctx, ListUsersInput{Input: pagination.Input{Page: 2, PageSize: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Pagination.TotalItems != 3 || out.Pagination.TotalPages != 2 {
		t.Errorf("got %d items, pagination %+v", len(out.Items), out.Pagination)
	}
}

func assertCode(t *testing.T, err error, want apperror.Code) {
	t.Helper()
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("want *apperror.Error with code %s, got %v", want, err)
	}
	if appErr.Code != want {
		t.Fatalf("code = %s, want %s", appErr.Code, want)
	}
}
