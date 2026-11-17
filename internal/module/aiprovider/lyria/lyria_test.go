package lyria_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/aiprovider/lyria"
)

// fakeAPI serves one canned reply and records the request it got.
type fakeAPI struct {
	status int
	reply  string
	method string
	path   string
	key    string
	body   map[string]any
}

func (f *fakeAPI) start(t *testing.T) aiprovider.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.method, f.path, f.key = r.Method, r.URL.Path, r.Header.Get("x-goog-api-key")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &f.body)
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.reply)
	}))
	t.Cleanup(srv.Close)
	return aiprovider.Config{BaseURL: srv.URL, APIKey: "test-key"}
}

var song = aiprovider.MusicRequest{Lyrics: "[Verse]\nhai\n[Chorus]\nsayang", StylePrompt: "pop ballad, female vocal"}

func TestGenerateMusic(t *testing.T) {
	t.Parallel()
	// "SUQzYWJj" = base64("ID3abc"); the lyrics text part comes first to check part scanning.
	api := &fakeAPI{status: 200, reply: `{"candidates":[{"content":{"parts":[{"text":"[Verse] hai"},` +
		`{"inlineData":{"mimeType":"audio/mpeg","data":"SUQzYWJj"}}]},"finishReason":"STOP"}]}`}
	cfg := api.start(t)

	got, err := lyria.NewMusicAdapter(http.DefaultClient).GenerateMusic(context.Background(), song, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Audio) != "ID3abc" || got.Format != "mp3" || got.DurationSec != 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if api.method != http.MethodPost || api.path != "/v1beta/models/lyria-3.5:generateContent" || api.key != "test-key" {
		t.Fatalf("wrong request: %s %s key=%s", api.method, api.path, api.key)
	}
	gc := api.body["generationConfig"].(map[string]any)
	if _, ok := gc["responseFormat"]; ok {
		t.Fatalf("mp3 must not send responseFormat: %v", gc)
	}
	text := api.body["contents"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "pop ballad") || !strings.Contains(text, "Lyrics:\n[Verse]\nhai") {
		t.Fatalf("prompt missing style or lyrics: %q", text)
	}
}

func TestGenerateMusicWAV(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{status: 200, reply: `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/wav","data":"UklGRg=="}}]}}]}`}
	cfg := api.start(t)
	cfg.Model = lyria.ClipModel
	cfg.Params = map[string]any{"format": "wav"}

	got, err := lyria.NewMusicAdapter(http.DefaultClient).GenerateMusic(context.Background(), song, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != "wav" || string(got.Audio) != "RIFF" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if api.path != "/v1beta/models/lyria-3-clip-preview:generateContent" {
		t.Fatalf("wrong path %s", api.path)
	}
	rf := api.body["generationConfig"].(map[string]any)["responseFormat"].(map[string]any)["audio"].(map[string]any)
	if rf["mimeType"] != "AUDIO_WAV" {
		t.Fatalf("wrong responseFormat: %v", rf)
	}
}

func TestGenerateMusicErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		reply  string
		kind   aiprovider.ErrorKind
	}{
		{"bad key", 400, `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`, aiprovider.KindAuth},
		{"quota", 429, `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`, aiprovider.KindRateLimited},
		{"billing", 400, `{"error":{"code":400,"message":"Billing is not enabled","status":"FAILED_PRECONDITION"}}`, aiprovider.KindInsufficientBalance},
		{"overloaded", 503, `{"error":{"code":503,"message":"overloaded","status":"UNAVAILABLE"}}`, aiprovider.KindUnavailable},
		{"prompt blocked", 200, `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`, aiprovider.KindContentFlagged},
		{"output blocked", 200, `{"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}`, aiprovider.KindContentFlagged},
		{"no audio", 200, `{"candidates":[{"content":{"parts":[{"text":"lyrics only"}]},"finishReason":"STOP"}]}`, aiprovider.KindInvalidOutput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := &fakeAPI{status: tc.status, reply: tc.reply}
			_, err := lyria.NewMusicAdapter(http.DefaultClient).GenerateMusic(context.Background(), song, api.start(t))
			if got := aiprovider.KindOf(err); got != tc.kind {
				t.Fatalf("kind = %s, want %s (err %v)", got, tc.kind, err)
			}
		})
	}
}

func TestGenerateMusicValidates(t *testing.T) {
	t.Parallel()
	a := lyria.NewMusicAdapter(http.DefaultClient)
	if _, err := a.GenerateMusic(context.Background(), aiprovider.MusicRequest{StylePrompt: "pop"}, aiprovider.Config{}); aiprovider.KindOf(err) != aiprovider.KindInvalidRequest {
		t.Fatalf("empty lyrics: %v", err)
	}
	cfg := aiprovider.Config{Params: map[string]any{"format": "flac"}}
	if _, err := a.GenerateMusic(context.Background(), song, cfg); aiprovider.KindOf(err) != aiprovider.KindInvalidRequest {
		t.Fatalf("bad format: %v", err)
	}
}

func TestTestConnection(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{status: 200, reply: `{"name":"models/lyria-3.5"}`}
	if err := lyria.NewMusicAdapter(http.DefaultClient).TestConnection(context.Background(), api.start(t)); err != nil {
		t.Fatal(err)
	}
	if api.method != http.MethodGet || api.path != "/v1beta/models/lyria-3.5" || api.key != "test-key" {
		t.Fatalf("wrong request: %s %s", api.method, api.path)
	}

	bad := &fakeAPI{status: 403, reply: `{"error":{"code":403,"message":"denied","status":"PERMISSION_DENIED"}}`}
	err := lyria.NewMusicAdapter(http.DefaultClient).TestConnection(context.Background(), bad.start(t))
	if aiprovider.KindOf(err) != aiprovider.KindAuth {
		t.Fatalf("want auth error, got %v", err)
	}
}
