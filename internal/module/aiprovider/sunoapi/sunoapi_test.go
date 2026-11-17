package sunoapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/aiprovider/sunoapi"
)

// fakeSuno is a sunoapi.org server: generate -> N pending polls -> final record -> audio download.
type fakeSuno struct {
	t           *testing.T
	submitReply string
	pending     int
	final       string // final record body; "{AUDIO}" is replaced by the audio URL
	credits     string
	srv         *httptest.Server

	mu         sync.Mutex
	polls      int
	submitBody map[string]any
	auth       map[string]string
	audioAuth  string
}

func (f *fakeSuno) start() aiprovider.Config {
	f.t.Helper()
	f.auth = map[string]string{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth[r.URL.Path] = r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/generate":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &f.submitBody)
			_, _ = io.WriteString(w, f.submitReply)
		case r.URL.Path == "/api/v1/generate/record-info" && r.URL.Query().Get("taskId") == "task-1":
			f.polls++
			if f.polls <= f.pending {
				_, _ = io.WriteString(w, `{"code":200,"msg":"success","data":{"taskId":"task-1","status":"FIRST_SUCCESS"}}`)
				return
			}
			_, _ = io.WriteString(w, strings.ReplaceAll(f.final, "{AUDIO}", f.srv.URL+"/files/song.mp3"))
		case r.URL.Path == "/files/song.mp3":
			f.audioAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = io.WriteString(w, "ID3abc")
		case r.URL.Path == "/api/v1/generate/credit":
			if r.Header.Get("Authorization") != "Bearer test-key" {
				_, _ = io.WriteString(w, `{"code":401,"msg":"Unauthorized"}`)
				return
			}
			_, _ = io.WriteString(w, f.credits)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	f.t.Cleanup(f.srv.Close)
	return aiprovider.Config{BaseURL: f.srv.URL, APIKey: "test-key", Model: "V6"}
}

func adapter() *sunoapi.MusicAdapter {
	a := sunoapi.NewMusicAdapter(http.DefaultClient)
	a.PollInterval = time.Millisecond
	return a
}

var song = aiprovider.MusicRequest{Lyrics: "[Verse]\nhalo", StylePrompt: "pop, warm", Title: "Untuk Ibu", VocalGender: "female"}

const okSubmit = `{"code":200,"msg":"success","data":{"taskId":"task-1"}}`

func kindOf(t *testing.T, err error) aiprovider.ErrorKind {
	t.Helper()
	var pe *aiprovider.Error
	if !errors.As(err, &pe) {
		t.Fatalf("want *aiprovider.Error, got %v", err)
	}
	return pe.Kind
}

func TestGenerateMusicSuccess(t *testing.T) {
	t.Parallel()
	f := &fakeSuno{t: t, submitReply: okSubmit, pending: 2,
		final: `{"code":200,"msg":"success","data":{"taskId":"task-1","status":"SUCCESS","response":{"sunoData":[{"id":"a","audio_url":"{AUDIO}","duration":184.6},{"id":"b","audio_url":"{AUDIO}"}]}}}`}
	cfg := f.start()
	cfg.Params = map[string]any{"negative_tags": "metal", "style_weight": 0.6, "duration": float64(120)}

	res, err := adapter().GenerateMusic(context.Background(), song, cfg)
	if err != nil {
		t.Fatalf("GenerateMusic: %v", err)
	}
	if string(res.Audio) != "ID3abc" || res.Format != "mp3" || res.DurationSec != 185 {
		t.Fatalf("unexpected result %+v", res)
	}
	b := f.submitBody
	checks := map[string]any{
		"prompt": "[Verse]\nhalo", "style": "pop, warm", "title": "Untuk Ibu", "customMode": true,
		"instrumental": false, "model": "V6", "vocalGender": "f", "negativeTags": "metal",
		"styleWeight": 0.6, "duration": float64(120), "callBackUrl": sunoapi.DefaultCallbackURL,
	}
	for k, want := range checks {
		if b[k] != want {
			t.Errorf("submit %s = %v, want %v", k, b[k], want)
		}
	}
	if f.auth["/api/v1/generate"] != "Bearer test-key" {
		t.Errorf("auth header = %q", f.auth["/api/v1/generate"])
	}
	if f.audioAuth != "" {
		t.Errorf("API key leaked to the audio download")
	}
	if f.polls != 3 {
		t.Errorf("polls = %d, want 3", f.polls)
	}
}

func TestVocalGenderParamOverrides(t *testing.T) {
	t.Parallel()
	f := &fakeSuno{t: t, submitReply: okSubmit,
		final: `{"code":200,"msg":"success","data":{"status":"SUCCESS","response":{"sunoData":[{"audioUrl":"{AUDIO}"}]}}}`}
	cfg := f.start()
	cfg.Params = map[string]any{"vocal_gender": "male", "callback_url": "https://example.test/cb"}
	if _, err := adapter().GenerateMusic(context.Background(), song, cfg); err != nil {
		t.Fatalf("GenerateMusic: %v", err)
	}
	if f.submitBody["vocalGender"] != "m" || f.submitBody["callBackUrl"] != "https://example.test/cb" {
		t.Fatalf("submit body %v", f.submitBody)
	}
}

func TestBodyErrorCodes(t *testing.T) {
	t.Parallel()
	cases := map[string]aiprovider.ErrorKind{
		`{"code":401,"msg":"Unauthorized"}`:            aiprovider.KindAuth,
		`{"code":429,"msg":"Insufficient credits"}`:    aiprovider.KindInsufficientBalance,
		`{"code":430,"msg":"call frequency too high"}`: aiprovider.KindRateLimited,
		`{"code":413,"msg":"prompt too long"}`:         aiprovider.KindInvalidRequest,
		`{"code":455,"msg":"maintenance"}`:             aiprovider.KindUnavailable,
	}
	for body, want := range cases {
		f := &fakeSuno{t: t, submitReply: body}
		cfg := f.start()
		_, err := adapter().GenerateMusic(context.Background(), song, cfg)
		if got := kindOf(t, err); got != want {
			t.Errorf("%s: kind %s, want %s", body, got, want)
		}
	}
}

func TestTaskFailures(t *testing.T) {
	t.Parallel()
	cases := map[string]aiprovider.ErrorKind{
		`{"code":200,"msg":"success","data":{"status":"SENSITIVE_WORD_ERROR","errorMessage":"bad words"}}`:                 aiprovider.KindContentFlagged,
		`{"code":200,"msg":"success","data":{"status":"GENERATE_AUDIO_FAILED","errorMessage":"internal","errorCode":500}}`: aiprovider.KindUnavailable,
		`{"code":200,"msg":"success","data":{"status":"SUCCESS","response":{"sunoData":[]}}}`:                              aiprovider.KindInvalidOutput,
	}
	for body, want := range cases {
		f := &fakeSuno{t: t, submitReply: okSubmit, final: body}
		cfg := f.start()
		_, err := adapter().GenerateMusic(context.Background(), song, cfg)
		if got := kindOf(t, err); got != want {
			t.Errorf("%s: kind %s, want %s", body, got, want)
		}
	}
}

func TestCallbackExceptionWithAudioSucceeds(t *testing.T) {
	t.Parallel()
	f := &fakeSuno{t: t, submitReply: okSubmit,
		final: `{"code":200,"msg":"success","data":{"status":"CALLBACK_EXCEPTION","response":{"sunoData":[{"audio_url":"{AUDIO}"}]}}}`}
	cfg := f.start()
	if _, err := adapter().GenerateMusic(context.Background(), song, cfg); err != nil {
		t.Fatalf("GenerateMusic: %v", err)
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	f := &fakeSuno{t: t, submitReply: okSubmit, pending: 1 << 30}
	cfg := f.start()
	cfg.Timeout = 30 * time.Millisecond
	_, err := adapter().GenerateMusic(context.Background(), song, cfg)
	if got := kindOf(t, err); got != aiprovider.KindTimeout {
		t.Fatalf("kind %s, want timeout", got)
	}
}

func TestInvalidRequest(t *testing.T) {
	t.Parallel()
	for _, req := range []aiprovider.MusicRequest{
		{Lyrics: "", StylePrompt: "pop"},
		{Lyrics: "la", StylePrompt: ""},
		{Lyrics: strings.Repeat("a", sunoapi.MaxLyricsChars+1), StylePrompt: "pop"},
	} {
		_, err := adapter().GenerateMusic(context.Background(), req, aiprovider.Config{BaseURL: "http://unused.invalid"})
		if got := kindOf(t, err); got != aiprovider.KindInvalidRequest {
			t.Errorf("kind %s, want invalid_request", got)
		}
	}
}

func TestTestConnection(t *testing.T) {
	t.Parallel()
	f := &fakeSuno{t: t, credits: `{"code":200,"msg":"success","data":120}`}
	cfg := f.start()
	if err := adapter().TestConnection(context.Background(), cfg); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	cfg.APIKey = "wrong-key"
	if got := kindOf(t, adapter().TestConnection(context.Background(), cfg)); got != aiprovider.KindAuth {
		t.Fatalf("kind %s, want auth", got)
	}

	empty := &fakeSuno{t: t, credits: `{"code":200,"msg":"success","data":0}`}
	if got := kindOf(t, adapter().TestConnection(context.Background(), empty.start())); got != aiprovider.KindInsufficientBalance {
		t.Fatalf("kind %s, want insufficient_balance", got)
	}
}
