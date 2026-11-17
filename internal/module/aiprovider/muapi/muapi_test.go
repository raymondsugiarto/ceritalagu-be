package muapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/aiprovider/muapi"
)

// fakeMuapi is a muapi server: submit -> N polls of "processing" -> final result -> audio download.
type fakeMuapi struct {
	t           *testing.T
	submitCode  int
	submitReply string
	processing  int    // polls answered "processing" before final
	final       string // final poll body; "{AUDIO}" is replaced by the audio URL
	srv         *httptest.Server

	mu         sync.Mutex
	polls      int
	submitBody map[string]any
	keys       map[string]string // path -> x-api-key seen
}

func (f *fakeMuapi) start() aiprovider.Config {
	f.t.Helper()
	f.keys = map[string]string{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.keys[r.URL.Path] = r.Header.Get("x-api-key")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/suno-create-music":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &f.submitBody)
			w.WriteHeader(f.submitCode)
			_, _ = io.WriteString(w, f.submitReply)
		case r.URL.Path == "/api/v1/predictions/req-1/result":
			f.polls++
			if f.polls <= f.processing {
				_, _ = io.WriteString(w, `{"id":"req-1","status":"processing","outputs":[]}`)
				return
			}
			body := f.final
			if len(body) > 0 {
				body = strings.Replace(body, "{AUDIO}", f.srv.URL+"/files/song.mp3", 1)
			}
			_, _ = io.WriteString(w, body)
		case r.URL.Path == "/files/song.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = io.WriteString(w, "ID3abc")
		case r.URL.Path == "/api/v1/account/balance":
			if r.Header.Get("x-api-key") != "test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"detail":"Invalid API key"}`)
				return
			}
			_, _ = io.WriteString(w, `{"balance":42.5}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	f.t.Cleanup(f.srv.Close)
	return aiprovider.Config{BaseURL: f.srv.URL, APIKey: "test-key"}
}

func adapter() *muapi.MusicAdapter {
	a := muapi.NewMusicAdapter(http.DefaultClient)
	a.PollInterval = time.Millisecond
	return a
}

var song = aiprovider.MusicRequest{Lyrics: "[Verse]\nhai\n[Chorus]\nsayang", StylePrompt: "pop ballad",
	Title: "Untuk Nadia", VocalGender: "female"}

const submitted = `{"request_id":"req-1","status":"processing"}`

func TestGenerateMusic(t *testing.T) {
	t.Parallel()
	f := &fakeMuapi{t: t, submitCode: 200, submitReply: submitted, processing: 2,
		final: `{"id":"req-1","status":"completed","outputs":["{AUDIO}","second-take"],"error":null}`}
	cfg := f.start()
	cfg.Params = map[string]any{"negative_tags": "heavy metal", "style_weight": 0.8}

	got, err := adapter().GenerateMusic(context.Background(), song, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Audio) != "ID3abc" || got.Format != "mp3" || got.DurationSec != 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if f.polls != 3 {
		t.Fatalf("polls = %d, want 3", f.polls)
	}
	b := f.submitBody
	if b["style"] != "pop ballad" || b["prompt"] != song.Lyrics || b["title"] != "Untuk Nadia" ||
		b["model"] != "V6" || b["custom_mode"] != true || b["instrumental"] != false ||
		b["vocal_gender"] != "female" || b["negative_tags"] != "heavy metal" || b["style_weight"] != 0.8 {
		t.Fatalf("wrong submit body: %v", b)
	}
	if _, ok := b["duration"]; ok {
		t.Fatalf("duration must be omitted when unset: %v", b)
	}
	if f.keys["/api/v1/suno-create-music"] != "test-key" || f.keys["/api/v1/predictions/req-1/result"] != "test-key" {
		t.Fatalf("api key not sent: %v", f.keys)
	}
	if f.keys["/files/song.mp3"] != "" {
		t.Fatal("api key must not be sent to the audio download URL")
	}
}

func TestGenerateMusicFailures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		submitCode  int
		submitReply string
		final       string
		kind        aiprovider.ErrorKind
	}{
		{"bad key", 401, `{"detail":"Invalid API key"}`, "", aiprovider.KindAuth},
		{"no credits", 402, `{"detail":"Insufficient balance"}`, "", aiprovider.KindInsufficientBalance},
		{"rate limited", 429, `{"detail":"slow down"}`, "", aiprovider.KindRateLimited},
		{"no request id", 200, `{"status":"processing"}`, "", aiprovider.KindInvalidOutput},
		{"job failed", 200, submitted, `{"id":"req-1","status":"failed","error":"upstream error"}`, aiprovider.KindUnavailable},
		{"job flagged", 200, submitted, `{"id":"req-1","status":"failed","error":"Lyrics contain sensitive words"}`, aiprovider.KindContentFlagged},
		{"no outputs", 200, submitted, `{"id":"req-1","status":"completed","outputs":[]}`, aiprovider.KindInvalidOutput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeMuapi{t: t, submitCode: tc.submitCode, submitReply: tc.submitReply, final: tc.final}
			_, err := adapter().GenerateMusic(context.Background(), song, f.start())
			if got := aiprovider.KindOf(err); got != tc.kind {
				t.Fatalf("kind = %s, want %s (err %v)", got, tc.kind, err)
			}
		})
	}
}

func TestGenerateMusicTimesOut(t *testing.T) {
	t.Parallel()
	f := &fakeMuapi{t: t, submitCode: 200, submitReply: submitted, processing: 1 << 30}
	cfg := f.start()
	cfg.Timeout = 50 * time.Millisecond
	_, err := adapter().GenerateMusic(context.Background(), song, cfg)
	if aiprovider.KindOf(err) != aiprovider.KindTimeout {
		t.Fatalf("want timeout, got %v", err)
	}
}

func TestGenerateMusicValidates(t *testing.T) {
	t.Parallel()
	a := adapter()
	cases := map[string]struct {
		req    aiprovider.MusicRequest
		params map[string]any
	}{
		"no lyrics": {aiprovider.MusicRequest{StylePrompt: "pop"}, nil},
		"no style":  {aiprovider.MusicRequest{Lyrics: "la la"}, nil},
		"duration":  {song, map[string]any{"duration": float64(5)}},
	}
	for name, tc := range cases {
		_, err := a.GenerateMusic(context.Background(), tc.req, aiprovider.Config{Params: tc.params})
		if aiprovider.KindOf(err) != aiprovider.KindInvalidRequest {
			t.Fatalf("%s: want invalid_request, got %v", name, err)
		}
	}
}

func TestTestConnection(t *testing.T) {
	t.Parallel()
	f := &fakeMuapi{t: t}
	cfg := f.start()
	if err := adapter().TestConnection(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "wrong"
	if err := adapter().TestConnection(context.Background(), cfg); aiprovider.KindOf(err) != aiprovider.KindAuth {
		t.Fatalf("want auth error, got %v", err)
	}
}
