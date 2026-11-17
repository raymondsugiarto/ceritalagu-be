package app_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/app"
	"github.com/yourorg/music-api/internal/module/user"
)

func newTestServer(t *testing.T) *app.App {
	t.Helper()
	// The tracked templates, so tests don't depend on per-machine config/resources.
	t.Setenv("CONFIG_DIR", "example")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return app.New(cfg, log, app.NewHealth(),
		user.NewHandler(user.NewService(user.NewMemoryRepository()), func(c fiber.Ctx) error { return c.Next() }),
	)
}

type envelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Errors  []struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"errors"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
	requestID string
}

func do(t *testing.T, s *app.App, method, path, body string) (int, envelope) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.Fiber().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode %s %s: %v", method, path, err)
	}
	// Every response must carry the full envelope; errors must be an array, never null.
	for _, key := range []string{"code", "message", "errors", "timestamp"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("%s %s: missing %q in %v", method, path, key, raw)
		}
	}
	if string(raw["errors"]) == "null" {
		t.Fatalf("%s %s: errors is null", method, path)
	}
	b, _ := json.Marshal(raw)
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	env.requestID = resp.Header.Get("X-Request-ID")
	return resp.StatusCode, env
}

func TestUserAPI(t *testing.T) {
	s := newTestServer(t)

	var createdID string
	t.Run("create returns 201 with SUCCESS envelope", func(t *testing.T) {
		status, env := do(t, s, http.MethodPost, "/api/v1/admin/users", `{"name":"Ada","email":"ada@example.com"}`)
		if status != http.StatusCreated || env.Code != "SUCCESS" || len(env.Errors) != 0 || env.Timestamp.IsZero() {
			t.Fatalf("status = %d, env = %+v", status, env)
		}
		var out struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(env.Data, &out)
		createdID = out.ID
	})

	t.Run("list returns items and pagination", func(t *testing.T) {
		status, env := do(t, s, http.MethodGet, "/api/v1/admin/users?page=1&page_size=5", "")
		var out struct {
			Items      []json.RawMessage `json:"items"`
			Pagination struct {
				PageSize   int `json:"page_size"`
				TotalItems int `json:"total_items"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(env.Data, &out); err != nil {
			t.Fatal(err)
		}
		if status != http.StatusOK || len(out.Items) != 1 || out.Pagination.PageSize != 5 || out.Pagination.TotalItems != 1 {
			t.Fatalf("status = %d, data = %s", status, env.Data)
		}
	})

	t.Run("invalid page_size returns 422", func(t *testing.T) {
		status, env := do(t, s, http.MethodGet, "/api/v1/admin/users?page_size=1000", "")
		if status != http.StatusUnprocessableEntity || env.Errors[0].Field != "page_size" {
			t.Fatalf("status = %d, env = %+v", status, env)
		}
	})

	t.Run("validation errors return 422 with field names", func(t *testing.T) {
		status, env := do(t, s, http.MethodPost, "/api/v1/admin/users", `{"name":"A","email":"nope"}`)
		if status != http.StatusUnprocessableEntity || env.Code != "VALIDATION_ERROR" {
			t.Fatalf("status = %d, env = %+v", status, env)
		}
		if len(env.Errors) != 2 || env.Errors[0].Field != "name" {
			t.Errorf("errors = %+v", env.Errors)
		}
		if len(env.Data) != 0 {
			t.Errorf("error response must omit data, got %s", env.Data)
		}
		if env.requestID == "" {
			t.Error("missing X-Request-ID header")
		}
	})

	t.Run("malformed JSON returns 400", func(t *testing.T) {
		status, env := do(t, s, http.MethodPost, "/api/v1/admin/users", `{"name":`)
		if status != http.StatusBadRequest || env.Code != "BAD_REQUEST" {
			t.Fatalf("status = %d, env = %+v", status, env)
		}
	})

	t.Run("duplicate email returns 409", func(t *testing.T) {
		status, _ := do(t, s, http.MethodPost, "/api/v1/admin/users", `{"name":"Ada 2","email":"ADA@example.com"}`)
		if status != http.StatusConflict {
			t.Fatalf("status = %d", status)
		}
	})

	t.Run("invalid id returns 400, unknown id returns 404", func(t *testing.T) {
		if status, _ := do(t, s, http.MethodGet, "/api/v1/admin/users/abc", ""); status != http.StatusBadRequest {
			t.Errorf("bad id status = %d", status)
		}
		if status, _ := do(t, s, http.MethodGet, "/api/v1/admin/users/7b8e7f3e-8c1f-4d6a-9a43-2f4f1c3c9f00", ""); status != http.StatusNotFound {
			t.Errorf("unknown id status = %d", status)
		}
	})

	t.Run("unknown route returns standard 404 envelope", func(t *testing.T) {
		status, env := do(t, s, http.MethodGet, "/nope", "")
		if status != http.StatusNotFound || env.Code != "NOT_FOUND" {
			t.Fatalf("status = %d, env = %+v", status, env)
		}
	})

	t.Run("delete returns 200 with message and no data", func(t *testing.T) {
		status, env := do(t, s, http.MethodDelete, "/api/v1/admin/users/"+createdID, "")
		if status != http.StatusOK || env.Code != "SUCCESS" || env.Message != "user deleted" || len(env.Data) != 0 {
			t.Fatalf("status = %d, env = %+v", status, env)
		}
	})

	t.Run("healthz is up", func(t *testing.T) {
		if status, env := do(t, s, http.MethodGet, "/healthz", ""); status != http.StatusOK || env.Code != "SUCCESS" {
			t.Fatalf("status = %d, env = %+v", status, env)
		}
	})
}
