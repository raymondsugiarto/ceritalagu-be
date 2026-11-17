package guest_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/yourorg/music-api/internal/module/guest"
)

func newApp() *fiber.App {
	app := fiber.New()
	guest.NewMiddleware("salt", true).RegisterRoutes(app)
	app.Get("/who", func(c fiber.Ctx) error {
		v := guest.FromCtx(c)
		return c.SendString(v.SessionID + "|" + v.IPHash)
	})
	return app
}

func TestMintsCookieWhenMissing(t *testing.T) {
	t.Parallel()
	resp, err := newApp().Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/who", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	cookie := strings.ToLower(resp.Header.Get("Set-Cookie"))
	for _, want := range []string{guest.CookieName + "=", "httponly", "secure", "samesite=lax"} {
		if !strings.Contains(cookie, want) {
			t.Fatalf("Set-Cookie %q missing %q", cookie, want)
		}
	}
	body, _ := io.ReadAll(resp.Body)
	id, hash, _ := strings.Cut(string(body), "|")
	if len(id) != 36 || len(hash) != 64 {
		t.Fatalf("visitor = %q", body)
	}
}

func TestKeepsValidCookie(t *testing.T) {
	t.Parallel()
	const id = "6f1c1c7e-8b8f-4a57-9a43-1f6f2d0c9a11"
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/who", nil)
	req.Header.Set("Cookie", guest.CookieName+"="+id)
	resp, err := newApp().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Header.Get("Set-Cookie") != "" {
		t.Fatal("valid cookie must not be replaced")
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), id+"|") {
		t.Fatalf("visitor = %q", body)
	}
}

func TestReplacesForgedCookie(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/who", nil)
	req.Header.Set("Cookie", guest.CookieName+"=not-a-uuid")
	resp, err := newApp().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if !strings.Contains(resp.Header.Get("Set-Cookie"), guest.CookieName+"=") {
		t.Fatal("invalid cookie must be replaced")
	}
}

func TestIPFromTrustedProxyOnly(t *testing.T) {
	t.Parallel()
	hashOf := func(trust bool, xff string) string {
		app := fiber.New(fiber.Config{
			TrustProxy:       trust,
			ProxyHeader:      fiber.HeaderXForwardedFor,
			TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0"}},
		})
		guest.NewMiddleware("salt", false).RegisterRoutes(app)
		app.Get("/who", func(c fiber.Ctx) error { return c.SendString(guest.FromCtx(c).IPHash) })
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/who", nil)
		req.Header.Set(fiber.HeaderXForwardedFor, xff)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}
	// Behind a trusted proxy two visitors get different hashes; untrusted, the header is ignored.
	if hashOf(true, "1.1.1.1") == hashOf(true, "2.2.2.2") {
		t.Fatal("trusted proxy: X-Forwarded-For must be used")
	}
	if hashOf(false, "1.1.1.1") != hashOf(false, "2.2.2.2") {
		t.Fatal("untrusted: X-Forwarded-For must be ignored (spoofing)")
	}
}
