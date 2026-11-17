package minimax_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/aiprovider/minimax"
)

// fakeAPI serves one canned reply and records the request it got.
type fakeAPI struct {
	status int
	reply  string
	path   string
	auth   string
	body   map[string]any
}

func (f *fakeAPI) start(t *testing.T) aiprovider.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.path, f.auth = r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &f.body)
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.reply)
	}))
	t.Cleanup(srv.Close)
	return aiprovider.Config{BaseURL: srv.URL, APIKey: "test-key"}
}

const lyricsJSON = `{\"title\":\"Untuk Nadia\",\"lyrics\":\"[Verse]\\nhai\\n[Chorus]\\nsayang\",\"style_prompt\":\"pop ballad\",\"vocal_gender\":\"female\",\"language\":\"id\",\"mood\":\"romantic\"}`

func TestTextAdapterGenerate(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{status: 200, reply: `{"choices":[{"finish_reason":"stop","message":{"content":"` + lyricsJSON + `","reasoning_content":"thinking"}}],"base_resp":{"status_code":0,"status_msg":"success"}}`}
	cfg := api.start(t)
	cfg.Params = map[string]any{"temperature": 0.9, "max_completion_tokens": float64(4000)}

	got, err := minimax.NewTextAdapter(http.DefaultClient).GenerateLyricsStyle(context.Background(),
		aiprovider.LyricsStyleRequest{SystemPrompt: "sys", UserPrompt: "usr"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Untuk Nadia" || !strings.Contains(got.Lyrics, "[Chorus]") {
		t.Fatalf("unexpected result: %+v", got)
	}
	if api.path != "/v1/text/chatcompletion_v2" || api.auth != "Bearer test-key" {
		t.Fatalf("wrong request: path=%s auth=%s", api.path, api.auth)
	}
	if api.body["model"] != minimax.DefaultTextModel || api.body["temperature"] != 0.9 || api.body["max_completion_tokens"] != float64(4000) {
		t.Fatalf("wrong body: %v", api.body)
	}
	msgs, _ := api.body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("want system+user messages, got %v", msgs)
	}
}

func TestTextAdapterErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		status    int
		reply     string
		kind      aiprovider.ErrorKind
		retryable bool
	}{
		{"rate limited", 200, `{"base_resp":{"status_code":1002,"status_msg":"rate limit"}}`, aiprovider.KindRateLimited, true},
		{"bad key", 200, `{"base_resp":{"status_code":1004,"status_msg":"auth failed"}}`, aiprovider.KindAuth, false},
		{"no balance", 200, `{"base_resp":{"status_code":1008,"status_msg":"insufficient balance"}}`, aiprovider.KindInsufficientBalance, false},
		{"sensitive", 200, `{"choices":[{"message":{"content":"x"}}],"output_sensitive":true,"base_resp":{"status_code":0}}`, aiprovider.KindContentFlagged, false},
		{"http 503", 503, `upstream down`, aiprovider.KindUnavailable, true},
		{"not json", 200, `<html>`, aiprovider.KindInvalidOutput, true},
		{"reply without JSON", 200, `{"choices":[{"finish_reason":"stop","message":{"content":"maaf"}}],"base_resp":{"status_code":0}}`, aiprovider.KindInvalidOutput, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := &fakeAPI{status: tt.status, reply: tt.reply}
			_, err := minimax.NewTextAdapter(http.DefaultClient).GenerateLyricsStyle(context.Background(), aiprovider.LyricsStyleRequest{}, api.start(t))
			if got := aiprovider.KindOf(err); got != tt.kind {
				t.Fatalf("kind = %s, want %s (err %v)", got, tt.kind, err)
			}
			if aiprovider.IsRetryable(err) != tt.retryable {
				t.Fatalf("retryable = %t, want %t", !tt.retryable, tt.retryable)
			}
		})
	}
}

func TestMusicAdapterGenerate(t *testing.T) {
	t.Parallel()
	audio := []byte("ID3fake-mp3-bytes")
	api := &fakeAPI{status: 200, reply: `{"data":{"audio":"` + hex.EncodeToString(audio) + `","status":2},"extra_info":{"music_duration":151600},"base_resp":{"status_code":0,"status_msg":"success"}}`}
	cfg := api.start(t)
	cfg.Model = "music-2.6"
	cfg.Params = map[string]any{"bitrate": float64(128000)}

	got, err := minimax.NewMusicAdapter(http.DefaultClient).GenerateMusic(context.Background(),
		aiprovider.MusicRequest{Lyrics: "[Verse]\nhai", StylePrompt: "pop"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Audio) != string(audio) || got.Format != "mp3" || got.DurationSec != 152 {
		t.Fatalf("unexpected result: %+v", got)
	}
	setting, _ := api.body["audio_setting"].(map[string]any)
	if api.path != "/v1/music_generation" || api.body["model"] != "music-2.6" || api.body["output_format"] != "hex" ||
		setting["bitrate"] != float64(128000) || setting["sample_rate"] != float64(44100) {
		t.Fatalf("wrong request: %s %v", api.path, api.body)
	}
}

func TestMusicAdapterValidation(t *testing.T) {
	t.Parallel()
	a := minimax.NewMusicAdapter(http.DefaultClient)
	tests := []struct {
		name string
		req  aiprovider.MusicRequest
	}{
		{"empty lyrics", aiprovider.MusicRequest{StylePrompt: "pop"}},
		{"lyrics too long", aiprovider.MusicRequest{Lyrics: strings.Repeat("a", minimax.MaxLyricsChars+1)}},
		{"prompt too long", aiprovider.MusicRequest{Lyrics: "[Verse] a", StylePrompt: strings.Repeat("a", minimax.MaxPromptChars+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// No server: validation must fail before any HTTP call.
			_, err := a.GenerateMusic(context.Background(), tt.req, aiprovider.Config{BaseURL: "http://127.0.0.1:1"})
			if aiprovider.KindOf(err) != aiprovider.KindInvalidRequest {
				t.Fatalf("want invalid_request, got %v", err)
			}
		})
	}
}

func TestMusicAdapterNotCompleted(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{status: 200, reply: `{"data":{"audio":"","status":1},"base_resp":{"status_code":0}}`}
	_, err := minimax.NewMusicAdapter(http.DefaultClient).GenerateMusic(context.Background(),
		aiprovider.MusicRequest{Lyrics: "[Verse] a"}, api.start(t))
	if !aiprovider.IsRetryable(err) {
		t.Fatalf("in-progress status must be retryable, got %v", err)
	}
}

func TestMusicTestConnection(t *testing.T) {
	t.Parallel()
	ok := &fakeAPI{status: 200, reply: `{"base_resp":{"status_code":2013,"status_msg":"invalid params"}}`}
	if err := minimax.NewMusicAdapter(http.DefaultClient).TestConnection(context.Background(), ok.start(t)); err != nil {
		t.Fatalf("2013 means the key works, got %v", err)
	}
	bad := &fakeAPI{status: 200, reply: `{"base_resp":{"status_code":2049,"status_msg":"invalid api key"}}`}
	if err := minimax.NewMusicAdapter(http.DefaultClient).TestConnection(context.Background(), bad.start(t)); aiprovider.KindOf(err) != aiprovider.KindAuth {
		t.Fatalf("want auth error, got %v", err)
	}
}
