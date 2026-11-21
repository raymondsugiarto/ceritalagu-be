package mail_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/mail"
)

// capturedRequest is what the fake Brevo server received.
type capturedRequest struct {
	Header http.Header
	Body   map[string]any
}

func TestBrevoSendsJSONWithAPIKey(t *testing.T) {
	t.Parallel()
	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/smtp/email" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.Body)
		got.Header = r.Header.Clone()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<1@brevo>"}`))
	}))
	defer srv.Close()

	sender := newBrevo(t, srv.URL, false)
	err := sender.Send(context.Background(), mail.Message{
		To:      "customer@example.com",
		Subject: "Lagu kamu sudah siap",
		Text:    "Halo!",
		HTML:    "<p>Halo!</p>",
		Files:   []mail.Attachment{{Name: "lagu.mp3", Content: []byte("MP3DATA")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got.Header.Get("api-key") != "test-key" {
		t.Errorf("api-key header = %q", got.Header.Get("api-key"))
	}
	if ct := got.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	if got.Body["subject"] != "Lagu kamu sudah siap" {
		t.Errorf("subject = %v", got.Body["subject"])
	}
	if got.Body["textContent"] != "Halo!" {
		t.Errorf("textContent = %v", got.Body["textContent"])
	}
	if got.Body["htmlContent"] != "<p>Halo!</p>" {
		t.Errorf("htmlContent = %v", got.Body["htmlContent"])
	}
	sender2, ok := got.Body["sender"].(map[string]any)
	if !ok {
		t.Fatalf("sender = %v", got.Body["sender"])
	}
	if sender2["email"] != "no-reply@example.com" || sender2["name"] != "Lagu Untukmu" {
		t.Errorf("sender = %v", sender2)
	}
	to, ok := got.Body["to"].([]any)
	if !ok || len(to) != 1 {
		t.Fatalf("to = %v", got.Body["to"])
	}
	if to[0].(map[string]any)["email"] != "customer@example.com" {
		t.Errorf("to = %v", to)
	}
	att, ok := got.Body["attachment"].([]any)
	if !ok || len(att) != 1 {
		t.Fatalf("attachment = %v", got.Body["attachment"])
	}
	a := att[0].(map[string]any)
	if a["name"] != "lagu.mp3" {
		t.Errorf("attachment name = %v", a["name"])
	}
	if a["content"] != base64.StdEncoding.EncodeToString([]byte("MP3DATA")) {
		t.Errorf("attachment content = %v", a["content"])
	}
}

func TestBrevoSandboxHeader(t *testing.T) {
	t.Parallel()
	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.Body)
		got.Header = r.Header.Clone()
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sender := newBrevo(t, srv.URL, true)
	if err := sender.Send(context.Background(), mail.Message{To: "a@example.com", Subject: "s", Text: "t"}); err != nil {
		t.Fatal(err)
	}
	if got.Header.Get("X-Sib-Sandbox") != "drop" {
		t.Errorf("sandbox header = %q", got.Header.Get("X-Sib-Sandbox"))
	}
}

func TestBrevoRetriesOnServerError(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sender := newBrevo(t, srv.URL, false)
	// The backoff waits between attempts, so shorten it to keep the test fast.
	sender.RetryBaseDelay = time.Millisecond
	if err := sender.Send(context.Background(), mail.Message{To: "a@example.com", Subject: "s", Text: "t"}); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestBrevoDoesNotRetryOnBadRequest(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"invalid_parameter","message":"bad"}`))
	}))
	defer srv.Close()

	sender := newBrevo(t, srv.URL, false)
	err := sender.Send(context.Background(), mail.Message{To: "a@example.com", Subject: "s", Text: "t"})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "invalid_parameter") {
		t.Errorf("error = %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 400)", calls)
	}
}

func TestNewBrevoNeedsAPIKeyAndSender(t *testing.T) {
	t.Parallel()
	if _, err := mail.NewBrevo(config.BrevoConfig{}); err == nil {
		t.Fatal("missing api key must fail")
	}
	if _, err := mail.NewBrevo(config.BrevoConfig{APIKey: "k"}); err == nil {
		t.Fatal("missing sender email must fail")
	}
	sender, err := mail.NewBrevo(config.BrevoConfig{APIKey: "k", SenderEmail: "a@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if sender == nil {
		t.Fatal("nil sender")
	}
}

// newBrevo builds a sender pointed at the fake server.
func newBrevo(t *testing.T, baseURL string, sandbox bool) *mail.BrevoSender {
	t.Helper()
	sender, err := mail.NewBrevo(config.BrevoConfig{
		APIKey: "test-key", BaseURL: baseURL, SenderName: "Lagu Untukmu", SenderEmail: "no-reply@example.com",
		Timeout: 5 * time.Second, MaxAttempts: 3, Sandbox: sandbox,
	})
	if err != nil {
		t.Fatal(err)
	}
	return sender
}

// discardLogger returns a logger that drops everything.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBrevoSetsReplyTo(t *testing.T) {
	t.Parallel()
	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	err := newBrevo(t, srv.URL, false).Send(context.Background(), mail.Message{
		To: "hello@ceritalagu.com", Subject: "Kontak", Text: "Halo",
		ReplyTo: "rina@example.com", ReplyToName: "Rina",
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, ok := got.Body["replyTo"].(map[string]any)
	if !ok || rt["email"] != "rina@example.com" || rt["name"] != "Rina" {
		t.Errorf("replyTo = %#v", got.Body["replyTo"])
	}
}

func TestBrevoSendsTemplateWithParams(t *testing.T) {
	t.Parallel()
	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	err := newBrevo(t, srv.URL, false).Send(context.Background(), mail.Message{
		To:         "customer@example.com",
		Subject:    "fallback subject",
		Text:       "fallback text",
		HTML:       "<p>fallback</p>",
		TemplateID: 7,
		Params:     map[string]any{"SONG_TITLE": "Untuk Nadia"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Body["templateId"] != float64(7) {
		t.Errorf("templateId = %v", got.Body["templateId"])
	}
	params, _ := got.Body["params"].(map[string]any)
	if params["SONG_TITLE"] != "Untuk Nadia" {
		t.Errorf("params = %v", got.Body["params"])
	}
	for _, k := range []string{"subject", "htmlContent", "textContent"} {
		if _, ok := got.Body[k]; ok {
			t.Errorf("%s must be left to the template, got %v", k, got.Body[k])
		}
	}
}
