package i18n

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want Locale
		ok   bool
	}{
		{"id", ID, true},
		{"en", EN, true},
		{"EN", EN, true},
		{"en-US", EN, true},
		{"en_GB", EN, true},
		{" id-ID ", ID, true},
		{"fr", "", false},
		{"", "", false},
		{"*", "", false},
	}
	for _, tt := range tests {
		got, ok := Parse(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Parse(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestMatchAcceptLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		header string
		want   Locale
		ok     bool
	}{
		{"", "", false},
		{"en-US,en;q=0.9", EN, true},
		{"id-ID,id;q=0.9,en;q=0.8", ID, true},
		{"fr-FR,fr;q=0.9,en;q=0.5", EN, true},
		{"en;q=0.4, id;q=0.8", ID, true},
		{"en;q=0,id;q=0", "", false},
		{"de,fr", "", false},
		{"*", "", false},
		{"en;q=abc,id", ID, true},       // bad q-value = not acceptable
		{"id;q=0.5,en;q=0.5", ID, true}, // tie keeps header order
	}
	for _, tt := range tests {
		got, ok := MatchAcceptLanguage(tt.header)
		if got != tt.want || ok != tt.ok {
			t.Errorf("MatchAcceptLanguage(%q) = %q, %v; want %q, %v", tt.header, got, ok, tt.want, tt.ok)
		}
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                  string
		query, cookie, accept string
		want                  Locale
	}{
		{"query wins", "en", "id", "id", EN},
		{"invalid query falls to cookie", "xx", "en", "id", EN},
		{"cookie over header", "", "id", "en-US", ID},
		{"invalid cookie falls to header", "", "xx", "en-GB,en;q=0.9", EN},
		{"header only", "", "", "en", EN},
		{"unsupported header → default", "", "", "ja-JP", ID},
		{"nothing → default", "", "", "", ID},
	}
	for _, tt := range tests {
		if got := Resolve(tt.query, tt.cookie, tt.accept); got != tt.want {
			t.Errorf("%s: Resolve(%q, %q, %q) = %q; want %q", tt.name, tt.query, tt.cookie, tt.accept, got, tt.want)
		}
	}
}

func TestFromContextDefault(t *testing.T) {
	t.Parallel()
	if got := FromContext(context.Background()); got != Default {
		t.Fatalf("empty ctx = %q; want %q", got, Default)
	}
	if got := FromContext(WithLocale(context.Background(), EN)); got != EN {
		t.Fatalf("stored = %q; want en", got)
	}
}

func TestMiddleware(t *testing.T) {
	t.Parallel()
	app := fiber.New()
	app.Use(Middleware())
	app.Get("/", func(c fiber.Ctx) error { return c.SendString(string(FromContext(c.Context()))) })

	tests := []struct {
		name, target, cookie, accept string
		want                         string
	}{
		{"none", "/", "", "", "id"},
		{"header", "/", "", "en-US,en;q=0.9", "en"},
		{"cookie beats header", "/", "id", "en", "id"},
		{"query beats cookie", "/?locale=en", "id", "id", "en"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.target, nil)
		if tt.cookie != "" {
			req.Header.Set("Cookie", CookieName+"="+tt.cookie)
		}
		if tt.accept != "" {
			req.Header.Set("Accept-Language", tt.accept)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		if string(body) != tt.want {
			t.Errorf("%s: got %q; want %q", tt.name, body, tt.want)
		}
	}
}
