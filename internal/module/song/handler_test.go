package song_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/app"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/song"
)

// TestHTTPFlow drives the real app: cookie -> create job -> poll -> song -> preview audio.
func TestHTTPFlow(t *testing.T) {
	t.Setenv("CONFIG_DIR", "example")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	sid, qID := uuid.NewString(), uuid.NewString()
	repo := newMemRepo()
	svc := song.NewService(repo, ownedAnswers{qID: sid}, &fakeQueue{}, fakeAudio{}, song.Limits{GuestDailyQuota: 3, MaxRegenerate: 2})
	srv := app.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), app.NewHealth(),
		guest.NewMiddleware("salt", false), song.NewHandler(svc))

	call := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", guest.CookieName+"="+sid)
		resp, err := srv.Fiber().Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var env map[string]any
		raw, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(raw, &env)
		return resp.StatusCode, env
	}

	code, env := call(http.MethodPost, "/api/v1/song-jobs", `{"questionnaire_id":"`+qID+`"}`)
	if code != http.StatusAccepted {
		t.Fatalf("create: %d %v", code, env)
	}
	jobID := env["data"].(map[string]any)["id"].(string)

	if code, env = call(http.MethodPost, "/api/v1/song-jobs", `{"questionnaire_id":"nope"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad id: %d %v", code, env)
	}

	s := repo.finish(jobID, false)
	if code, env = call(http.MethodGet, "/api/v1/song-jobs/"+jobID, ""); code != http.StatusOK ||
		env["data"].(map[string]any)["song_id"] != s.ID {
		t.Fatalf("poll: %d %v", code, env)
	}
	if code, env = call(http.MethodGet, "/api/v1/songs/"+s.ID, ""); code != http.StatusOK ||
		env["data"].(map[string]any)["title"] != "Untuk Nadia" {
		t.Fatalf("song: %d %v", code, env)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/songs/"+s.ID+"/preview", nil)
	req.Header.Set("Cookie", guest.CookieName+"="+sid)
	audioResp, err := srv.Fiber().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = audioResp.Body.Close() }()
	body, _ := io.ReadAll(audioResp.Body)
	if audioResp.StatusCode != http.StatusOK || audioResp.Header.Get("Content-Type") != "audio/mpeg" || string(body) != "MP3:songs/x/preview.mp3" {
		t.Fatalf("preview: %d %s %q", audioResp.StatusCode, audioResp.Header.Get("Content-Type"), body)
	}
}
