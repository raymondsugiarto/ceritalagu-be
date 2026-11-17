// Package lyria holds the Google Lyria music adapter. It calls the Gemini API generateContent
// endpoint (POST /v1beta/models/{model}:generateContent) and reads the base64 audio part.
// Shapes follow ai.google.dev/gemini-api/docs/music-generation (read 2026-11-15):
//   - lyria-3.5 writes a full song (a couple of minutes), MP3 by default or WAV, 44.1 kHz stereo.
//   - lyria-3-clip-preview always returns a 30-second clip.
//   - lyrics go in the text prompt with section tags ([Verse 1], [Chorus], ...).
//
// The docs give no prompt length limit, no duration metadata and no rate limits.
package lyria

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

const (
	providerName = "lyria"
	// DefaultBaseURL is the Gemini API host. Admins can override it per provider.
	DefaultBaseURL = "https://generativelanguage.googleapis.com"
	// DefaultModel writes full-length songs.
	DefaultModel = "lyria-3.5"
	// ClipModel always returns a 30-second clip; cheaper, useful for testing.
	ClipModel = "lyria-3-clip-preview"

	apiVersion     = "/v1beta"
	defaultTimeout = 5 * time.Minute
	maxBodyBytes   = 64 << 20 // base64 audio of a full WAV song fits
	logMaxString   = 8000     // the base64 audio is replaced by its length in logs
	logMaxTotal    = 32000
)

// Register adds the Lyria music adapter to r.
func Register(r *aiprovider.Registry) {
	r.RegisterMusic(aiprovider.AdapterLyriaMusic, NewMusicAdapter(&http.Client{}))
}

// Catalog describes the Lyria adapter for the admin provider form.
func Catalog() []aiprovider.AdapterInfo {
	return []aiprovider.AdapterInfo{
		{Type: aiprovider.AdapterLyriaMusic, Category: aiprovider.CategoryMusic, Label: "Google Lyria",
			DefaultBaseURL: DefaultBaseURL, DefaultModel: DefaultModel, Models: []string{DefaultModel, ClipModel},
			Params: []aiprovider.ParamSpec{
				{Key: "format", Label: "Format audio", Kind: aiprovider.ParamString, Options: []string{"mp3", "wav"},
					Placeholder: "mp3"},
				{Key: "extra_instructions", Label: "Instruksi tambahan", Kind: aiprovider.ParamText,
					Help: "Ditambahkan ke prompt musik setiap job, mis. \"selalu akhiri dengan outro instrumental\"."},
			}},
	}
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type content struct {
	Parts []part `json:"parts"`
}

type audioFormat struct {
	MimeType string `json:"mimeType"`
}

type responseFormat struct {
	Audio audioFormat `json:"audio"`
}

type generationConfig struct {
	ResponseModalities []string        `json:"responseModalities"`
	ResponseFormat     *responseFormat `json:"responseFormat,omitempty"`
}

type generateRequest struct {
	Contents         []content        `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

type generateResponse struct {
	Candidates []struct {
		Content      content `json:"content"`
		FinishReason string  `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// apiError is Google's error envelope: {"error":{"code":400,"message":"...","status":"INVALID_ARGUMENT"}}.
type apiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// MusicAdapter generates a song with Lyria (synchronous, base64 audio in the response).
// Params (ai_provider.params): format (mp3|wav, default mp3), extra_instructions (appended to the prompt).
type MusicAdapter struct {
	http *http.Client
}

// NewMusicAdapter returns the music adapter.
func NewMusicAdapter(h *http.Client) *MusicAdapter { return &MusicAdapter{http: h} }

var _ aiprovider.MusicProvider = (*MusicAdapter)(nil)

// GenerateMusic sends style + lyrics as one prompt and returns the first audio part.
// DurationSec is left 0: Lyria reports no duration, so the pipeline measures it.
func (a *MusicAdapter) GenerateMusic(ctx context.Context, req aiprovider.MusicRequest, cfg aiprovider.Config) (aiprovider.MusicResult, error) {
	if strings.TrimSpace(req.Lyrics) == "" {
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidRequest, Provider: providerName, Message: "lyrics are required"}
	}
	format := stringParam(cfg.Params, "format", "mp3")
	gc := generationConfig{ResponseModalities: []string{"AUDIO", "TEXT"}}
	switch format {
	case "mp3":
	case "wav":
		gc.ResponseFormat = &responseFormat{Audio: audioFormat{MimeType: "AUDIO_WAV"}}
	default:
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidRequest, Provider: providerName,
			Message: fmt.Sprintf("format %q is not supported (mp3|wav)", format)}
	}
	in := generateRequest{
		Contents:         []content{{Parts: []part{{Text: BuildPrompt(req, stringParam(cfg.Params, "extra_instructions", ""))}}}},
		GenerationConfig: gc,
	}

	model := modelOr(cfg.Model)
	var out generateResponse
	if err := a.do(ctx, cfg, http.MethodPost, "/models/"+url.PathEscape(model)+":generateContent", in, &out); err != nil {
		return aiprovider.MusicResult{}, err
	}
	if r := out.PromptFeedback.BlockReason; r != "" {
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindContentFlagged, Provider: providerName, Message: "prompt blocked: " + r}
	}
	if len(out.Candidates) == 0 {
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "no candidates"}
	}
	cand := out.Candidates[0]
	// The docs say not to assume part order: scan every part for the audio.
	for _, p := range cand.Content.Parts {
		if p.InlineData == nil || p.InlineData.Data == "" {
			continue
		}
		audio, err := base64.StdEncoding.DecodeString(p.InlineData.Data)
		if err != nil {
			return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "audio is not valid base64", Err: err}
		}
		return aiprovider.MusicResult{Audio: audio, Format: formatOf(p.InlineData.MimeType, format)}, nil
	}
	if isSafetyFinish(cand.FinishReason) {
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindContentFlagged, Provider: providerName, Message: "output blocked: " + cand.FinishReason}
	}
	return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName,
		Message: fmt.Sprintf("no audio in response (finishReason %q)", cand.FinishReason)}
}

// TestConnection checks the key and model with GET /v1beta/models/{model}, which generates nothing.
func (a *MusicAdapter) TestConnection(ctx context.Context, cfg aiprovider.Config) error {
	var out struct {
		Name string `json:"name"`
	}
	return a.do(ctx, cfg, http.MethodGet, "/models/"+url.PathEscape(modelOr(cfg.Model)), nil, &out)
}

// BuildPrompt joins the style prompt and the lyrics the way the Lyria prompt guide suggests:
// instructions first, then the section-tagged lyrics kept apart from them.
func BuildPrompt(req aiprovider.MusicRequest, extra string) string {
	var b strings.Builder
	if s := strings.TrimSpace(req.StylePrompt); s != "" {
		b.WriteString("Create a song in this style: ")
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	if e := strings.TrimSpace(extra); e != "" {
		b.WriteString(e)
		b.WriteString("\n\n")
	}
	b.WriteString("Sing exactly the following lyrics, in order. The tags in square brackets ([Verse], [Chorus], ...) " +
		"mark the song structure and are not sung.\n\nLyrics:\n")
	b.WriteString(strings.TrimSpace(req.Lyrics))
	return b.String()
}

func (a *MusicAdapter) do(ctx context.Context, cfg aiprovider.Config, method, path string, in, out any) error {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return fmt.Errorf("encode lyria request: %w", err)
		}
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	endpoint := base + apiVersion + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build lyria request: %w", err)
	}
	req.Header.Set("x-goog-api-key", cfg.APIKey)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Debug level: bodies hold the customer's lyrics. The API key is masked.
	slog.DebugContext(ctx, "ai provider request", "provider", providerName, "method", method,
		"url", endpoint, "api_key", aiprovider.MaskKey(cfg.APIKey), "model", cfg.Model,
		"timeout", timeout.String(), "body", aiprovider.LogBody(body, logMaxString, logMaxTotal))
	started := time.Now()

	resp, err := a.http.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "ai provider request failed", "provider", providerName, "url", endpoint,
			"api_key", aiprovider.MaskKey(cfg.APIKey), "duration_ms", time.Since(started).Milliseconds(), "error", err)
		kind := aiprovider.KindUnavailable
		if errors.Is(err, context.DeadlineExceeded) {
			kind = aiprovider.KindTimeout
		}
		return &aiprovider.Error{Kind: kind, Provider: providerName, Message: "request failed", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	slog.DebugContext(ctx, "ai provider response", "provider", providerName, "url", endpoint,
		"api_key", aiprovider.MaskKey(cfg.APIKey), "status", resp.StatusCode,
		"duration_ms", time.Since(started).Milliseconds(), "bytes", len(raw),
		"body", aiprovider.LogBody(raw, logMaxString, logMaxTotal))
	if err != nil {
		return &aiprovider.Error{Kind: aiprovider.KindUnavailable, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "read response", Err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return httpError(resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "decode response", Err: err}
	}
	return nil
}

// httpError maps a Google API error (HTTP status + canonical status string) to an *aiprovider.Error.
func httpError(status int, raw []byte) error {
	var e apiError
	_ = json.Unmarshal(raw, &e)
	msg := e.Error.Message
	if msg == "" {
		msg = snippet(raw)
	}
	if e.Error.Status != "" {
		msg = e.Error.Status + ": " + msg
	}
	return &aiprovider.Error{Kind: kindFromHTTP(status, e.Error.Status, e.Error.Message), Provider: providerName,
		HTTPStatus: status, Message: msg}
}

func kindFromHTTP(status int, code, msg string) aiprovider.ErrorKind {
	lower := strings.ToLower(msg)
	switch {
	case status == http.StatusTooManyRequests || code == "RESOURCE_EXHAUSTED":
		return aiprovider.KindRateLimited
	case status == http.StatusUnauthorized || status == http.StatusForbidden || code == "UNAUTHENTICATED" ||
		code == "PERMISSION_DENIED" || strings.Contains(lower, "api key not valid") || strings.Contains(lower, "api_key_invalid"):
		return aiprovider.KindAuth
	case code == "FAILED_PRECONDITION" && strings.Contains(lower, "billing"):
		return aiprovider.KindInsufficientBalance
	case status == http.StatusGatewayTimeout || code == "DEADLINE_EXCEEDED":
		return aiprovider.KindTimeout
	case status >= 500:
		return aiprovider.KindUnavailable
	default:
		return aiprovider.KindInvalidRequest
	}
}

func isSafetyFinish(reason string) bool {
	switch reason {
	case "SAFETY", "PROHIBITED_CONTENT", "BLOCKLIST", "RECITATION", "SPII":
		return true
	default:
		return false
	}
}

func formatOf(mime, fallback string) string {
	switch strings.ToLower(mime) {
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return "wav"
	default:
		return fallback
	}
}

func modelOr(m string) string {
	if m = strings.TrimSpace(m); m != "" {
		return m
	}
	return DefaultModel
}

func stringParam(params map[string]any, key, fallback string) string {
	if v, ok := params[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func snippet(b []byte) string {
	const limit = 300
	s := strings.TrimSpace(string(b))
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}
