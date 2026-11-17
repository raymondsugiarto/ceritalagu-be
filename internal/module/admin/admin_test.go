package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/app"
)

type memRepo struct{ byID map[string]Admin }

func newMemRepo() *memRepo { return &memRepo{byID: map[string]Admin{}} }

func (m *memRepo) ByEmail(_ context.Context, email string) (Admin, error) {
	for _, a := range m.byID {
		if a.Email == normalizeEmail(email) {
			return a, nil
		}
	}
	return Admin{}, ErrNotFound
}

func (m *memRepo) ByID(_ context.Context, id string) (Admin, error) {
	a, ok := m.byID[id]
	if !ok {
		return Admin{}, ErrNotFound
	}
	return a, nil
}

func (m *memRepo) Create(_ context.Context, a Admin) (Admin, error) {
	a.ID = "id-" + a.Email
	m.byID[a.ID] = a
	return a, nil
}

func (m *memRepo) TouchLogin(_ context.Context, id string, at time.Time) error {
	a := m.byID[id]
	a.LastLoginAt = &at
	m.byID[id] = a
	return nil
}

const pw = "correct-horse-battery"

func init() { bcryptCost = bcrypt.MinCost } // keep the tests fast; production uses 12

func setup(t *testing.T) (*Service, *memRepo) {
	t.Helper()
	repo := newMemRepo()
	svc := NewService(repo, NewSigner("master", time.Hour))
	if _, err := svc.Create(context.Background(), CreateAdminInput{Name: "Ops", Email: "Ops@Example.com", Password: pw, Role: RoleSuperAdmin}); err != nil {
		t.Fatal(err)
	}
	return svc, repo
}

func TestSignerRejectsTamperedExpiredAndOldPassword(t *testing.T) {
	s := NewSigner("master", time.Hour)
	a := Admin{ID: "a1", PasswordHash: "h1"}
	tok := s.Issue(a)
	c, err := s.parse(tok)
	if err != nil || c.AdminID != "a1" || c.matches(a) != nil {
		t.Fatalf("valid token: %v %+v", err, c)
	}
	if _, err := s.parse(tok + "x"); err == nil {
		t.Fatal("tampered signature accepted")
	}
	if _, err := NewSigner("other", time.Hour).parse(tok); err == nil {
		t.Fatal("token from another key accepted")
	}
	if c.matches(Admin{ID: "a1", PasswordHash: "h2"}) == nil {
		t.Fatal("token survived a password change")
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := s.parse(tok); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestCreateRules(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()
	cases := []CreateAdminInput{
		{Name: "X", Email: "ops@example.com", Password: pw, Role: RoleSuperAdmin}, // duplicate (case-insensitive)
		{Name: "X", Email: "x@example.com", Password: "short", Role: RoleSuperAdmin},
		{Name: "X", Email: "x@example.com", Password: pw, Role: "root"},
		{Name: "", Email: "x@example.com", Password: pw, Role: RoleOperator},
	}
	for i, in := range cases {
		if _, err := svc.Create(ctx, in); err == nil {
			t.Errorf("case %d: expected an error", i)
		}
	}
}

func TestLoginAndAuthenticate(t *testing.T) {
	svc, repo := setup(t)
	ctx := context.Background()

	for _, in := range []LoginInput{{Email: "ops@example.com", Password: "wrong"}, {Email: "nobody@example.com", Password: pw}} {
		if _, _, err := svc.Login(ctx, in); err == nil || !strings.Contains(err.Error(), "UNAUTHORIZED") {
			t.Fatalf("login %s: %v", in.Email, err)
		}
	}
	out, tok, err := svc.Login(ctx, LoginInput{Email: "OPS@example.com", Password: pw})
	if err != nil || out.LastLoginAt == nil {
		t.Fatalf("login: %v %+v", err, out)
	}
	if _, err := svc.Authenticate(ctx, tok); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	a := repo.byID[out.ID]
	a.Status = StatusSuspended
	repo.byID[out.ID] = a
	if _, err := svc.Authenticate(ctx, tok); err == nil {
		t.Fatal("suspended admin still authenticated")
	}
	if _, _, err := svc.Login(ctx, LoginInput{Email: "ops@example.com", Password: pw}); err == nil {
		t.Fatal("suspended admin signed in")
	}
}

func TestHTTPLoginGuardAndRoles(t *testing.T) {
	t.Setenv("CONFIG_DIR", "example")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	svc, _ := setup(t)
	if _, err := svc.Create(context.Background(), CreateAdminInput{Name: "Op", Email: "op@example.com", Password: pw, Role: RoleOperator}); err != nil {
		t.Fatal(err)
	}
	guard := NewGuard(svc)
	superOnly := moduleFunc(func(r fiber.Router) {
		r.Get("/admin/secret", guard.Require(RoleSuperAdmin), func(c fiber.Ctx) error { return c.SendString(FromCtx(c).Email) })
	})
	srv := app.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), app.NewHealth(), NewHandler(svc, guard, false), superOnly)

	call := func(method, path, body, cookie string) (int, []*http.Cookie) {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			req.Header.Set("Cookie", CookieName+"="+cookie)
		}
		resp, err := srv.Fiber().Test(req, fiber.TestConfig{Timeout: 30 * time.Second}) // bcrypt is slow under -race
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode, resp.Cookies()
	}
	status := func(method, path, body, cookie string) int {
		t.Helper()
		code, _ := call(method, path, body, cookie)
		return code
	}
	login := func(email string) string {
		t.Helper()
		code, cookies := call(http.MethodPost, "/api/v1/admin/auth/login", `{"email":"`+email+`","password":"`+pw+`"}`, "")
		if code != http.StatusOK {
			t.Fatalf("login %s: %d", email, code)
		}
		for _, ck := range cookies {
			if ck.Name == CookieName {
				if !ck.HttpOnly || ck.Path != CookiePath || ck.SameSite != http.SameSiteStrictMode {
					t.Fatalf("cookie flags: %+v", ck)
				}
				return ck.Value
			}
		}
		t.Fatal("no session cookie")
		return ""
	}

	if got := status(http.MethodGet, "/api/v1/admin/auth/me", "", ""); got != http.StatusUnauthorized {
		t.Fatalf("me without cookie: %d", got)
	}
	superTok, opTok := login("ops@example.com"), login("op@example.com")
	if got := status(http.MethodGet, "/api/v1/admin/auth/me", "", superTok); got != http.StatusOK {
		t.Fatalf("me: %d", got)
	}
	if got := status(http.MethodGet, "/api/v1/admin/secret", "", superTok); got != http.StatusOK {
		t.Fatalf("super admin: %d", got)
	}
	if got := status(http.MethodGet, "/api/v1/admin/secret", "", opTok); got != http.StatusForbidden {
		t.Fatalf("operator: %d", got)
	}
	if got := status(http.MethodGet, "/api/v1/admin/secret", "", "forged.token"); got != http.StatusUnauthorized {
		t.Fatalf("forged: %d", got)
	}

	// Brute force: the limiter answers 429 after 5 tries per minute (2 used above).
	var last int
	for range 4 {
		last = status(http.MethodPost, "/api/v1/admin/auth/login", `{"email":"ops@example.com","password":"nope-nope"}`, "")
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("limiter: %d", last)
	}
}

type moduleFunc func(r fiber.Router)

func (f moduleFunc) RegisterRoutes(r fiber.Router) { f(r) }
