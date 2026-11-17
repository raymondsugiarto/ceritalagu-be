// Package muapi holds the Suno music adapter served through muapi.ai (an unofficial Suno reseller).
// Flow (muapi.ai/playground/suno-create-music/api, read 2026-11-15):
//  1. POST /api/v1/suno-create-music        -> {"request_id": "...", "status": "processing"}
//  2. GET  /api/v1/predictions/{id}/result  -> poll until status is completed | failed | cancelled
//  3. GET  outputs[0]                       -> the audio file (no API key is sent to that host)
//
// Auth is the x-api-key header. Error bodies, HTTP codes and rate limits are not documented, so
// errors are classified by HTTP status and message keywords.
package muapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

const (
	providerName = "muapi"
	// DefaultBaseURL is the muapi API host. Admins can override it per provider.
	DefaultBaseURL = "https://api.muapi.ai"
	// DefaultModel is the Suno model version muapi uses by default.
	DefaultModel = "V6"
	// MaxLyricsChars is the documented prompt (lyrics) limit for V6 models.
	MaxLyricsChars = 5000

	createPath          = "/api/v1/suno-create-music"
	resultPath          = "/api/v1/predictions/%s/result"
	balancePath         = "/api/v1/account/balance"
	defaultTimeout      = 10 * time.Minute // covers submit + polling + download
	defaultPollInterval = 5 * time.Second  // docs: poll every 2-5 s
	maxBodyBytes        = 1 << 20          // JSON replies are small
	maxAudioBytes       = 100 << 20
	logMaxString        = 8000
	logMaxTotal         = 32000
)

// Models are the Suno versions muapi accepts (model enum).
var Models = []string{"V6", "V6_MINI", "V6_WILD", "V5_5", "V5", "V4_5PLUS", "V4_5ALL", "V4_5", "V4", "V3_5"}

// Register adds the muapi Suno adapter to r.
func Register(r *aiprovider.Registry) {
	r.RegisterMusic(aiprovider.AdapterMuapiSuno, NewMusicAdapter(&http.Client{}))
}

// Catalog describes the muapi Suno adapter for the admin provider form.
func Catalog() []aiprovider.AdapterInfo {
	return []aiprovider.AdapterInfo{
		{Type: aiprovider.AdapterMuapiSuno, Category: aiprovider.CategoryMusic, Label: "Suno (muapi.ai)",
			DefaultBaseURL: DefaultBaseURL, DefaultModel: DefaultModel, Models: Models,
			Params: []aiprovider.ParamSpec{
				{Key: "vocal_gender", Label: "Suara vokal", Kind: aiprovider.ParamString, Options: []string{"male", "female"},
					Placeholder: "Ikuti hasil lirik", Help: "Kosong = pakai gender vokal dari langkah lirik."},
				{Key: "duration", Label: "Durasi (detik)", Kind: aiprovider.ParamInt, Min: aiprovider.Bound(10),
					Max: aiprovider.Bound(360), Placeholder: "otomatis"},
				{Key: "negative_tags", Label: "Negative tags", Kind: aiprovider.ParamString,
					Placeholder: "heavy metal, screaming", Help: "Gaya yang harus dihindari, dipisah koma."},
				{Key: "style_weight", Label: "Style weight", Kind: aiprovider.ParamFloat, Min: aiprovider.Bound(0),
					Max: aiprovider.Bound(1), Placeholder: "0–1"},
				{Key: "weirdness_constraint", Label: "Weirdness", Kind: aiprovider.ParamFloat, Min: aiprovider.Bound(0),
					Max: aiprovider.Bound(1), Placeholder: "0–1"},
				{Key: "poll_interval_sec", Label: "Interval polling (detik)", Kind: aiprovider.ParamInt,
					Min: aiprovider.Bound(1), Max: aiprovider.Bound(60), Placeholder: "5"},
			}},
	}
}

type createRequest struct {
	Style               string   `json:"style"`
	Prompt              string   `json:"prompt,omitempty"` // the exact lyrics when instrumental is false
	Title               string   `json:"title,omitempty"`
	Model               string   `json:"model"`
	CustomMode          bool     `json:"custom_mode"`
	Instrumental        bool     `json:"instrumental"`
	VocalGender         string   `json:"vocal_gender,omitempty"`
	NegativeTags        string   `json:"negative_tags,omitempty"`
	Duration            int      `json:"duration,omitempty"`
	StyleWeight         *float64 `json:"style_weight,omitempty"`
	WeirdnessConstraint *float64 `json:"weirdness_constraint,omitempty"`
}

type createResponse struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

type resultResponse struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Outputs []string `json:"outputs"`
	Error   *string  `json:"error"`
}

// MusicAdapter generates a song with Suno through muapi: submit, poll, download.
//
// Params (ai_provider.params), all optional:
//   - negative_tags (string): styles to avoid
//   - duration (int, 10-360 s): only V5_5 / V6 / V6_MINI / V6_WILD; unset lets the model choose
//   - style_weight, weirdness_constraint (0-1)
//   - vocal_gender (male|female): overrides the gender from the lyrics step
//   - poll_interval_sec (int, default 5)
//
// The provider timeout (timeout_sec) bounds the whole flow; default 10 minutes.
type MusicAdapter struct {
	http *http.Client
	// PollInterval is the default wait between result polls (tests shorten it).
	PollInterval time.Duration
}

// NewMusicAdapter returns the music adapter.
func NewMusicAdapter(h *http.Client) *MusicAdapter {
	return &MusicAdapter{http: h, PollInterval: defaultPollInterval}
}

var _ aiprovider.MusicProvider = (*MusicAdapter)(nil)

// GenerateMusic submits a custom-mode, vocal Suno job and returns the first output track.
// DurationSec is left 0: muapi reports no duration, so the pipeline measures it.
func (a *MusicAdapter) GenerateMusic(ctx context.Context, req aiprovider.MusicRequest, cfg aiprovider.Config) (aiprovider.MusicResult, error) {
	in, err := buildRequest(req, cfg)
	if err != nil {
		return aiprovider.MusicResult{}, err
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var created createResponse
	if err := a.do(ctx, cfg, http.MethodPost, createPath, in, &created); err != nil {
		return aiprovider.MusicResult{}, err
	}
	if created.RequestID == "" {
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "no request_id in submit response"}
	}

	audioURL, err := a.poll(ctx, cfg, created.RequestID)
	if err != nil {
		return aiprovider.MusicResult{}, err
	}
	audio, format, err := a.download(ctx, audioURL)
	if err != nil {
		return aiprovider.MusicResult{}, err
	}
	return aiprovider.MusicResult{Audio: audio, Format: format}, nil
}

// TestConnection checks the key with GET /api/v1/account/balance, which generates nothing.
func (a *MusicAdapter) TestConnection(ctx context.Context, cfg aiprovider.Config) error {
	timeout := cfg.Timeout
	if timeout <= 0 || timeout > time.Minute {
		timeout = time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var out struct {
		Balance float64 `json:"balance"`
	}
	if err := a.do(ctx, cfg, http.MethodGet, balancePath, nil, &out); err != nil {
		return err
	}
	slog.InfoContext(ctx, "muapi balance", "provider", providerName, "balance_usd", out.Balance)
	return nil
}

func buildRequest(req aiprovider.MusicRequest, cfg aiprovider.Config) (createRequest, error) {
	invalid := func(msg string) error {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidRequest, Provider: providerName, Message: msg}
	}
	lyrics := strings.TrimSpace(req.Lyrics)
	switch n := utf8.RuneCountInString(lyrics); {
	case n == 0:
		return createRequest{}, invalid("lyrics are required")
	case n > MaxLyricsChars:
		return createRequest{}, invalid(fmt.Sprintf("lyrics have %d characters, max %d", n, MaxLyricsChars))
	}
	style := strings.TrimSpace(req.StylePrompt)
	if style == "" {
		return createRequest{}, invalid("style prompt is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	in := createRequest{
		Style: style, Prompt: lyrics, Title: strings.TrimSpace(req.Title), Model: model,
		CustomMode: true, Instrumental: false, // muapi defaults instrumental to true; we always want vocals
		VocalGender:  gender(stringParam(cfg.Params, "vocal_gender", req.VocalGender)),
		NegativeTags: stringParam(cfg.Params, "negative_tags", ""),
		Duration:     intParam(cfg.Params, "duration", 0),
	}
	if in.Duration != 0 && (in.Duration < 10 || in.Duration > 360) {
		return createRequest{}, invalid(fmt.Sprintf("duration %d s is out of range (10-360)", in.Duration))
	}
	if v, ok := floatParam(cfg.Params, "style_weight"); ok {
		in.StyleWeight = &v
	}
	if v, ok := floatParam(cfg.Params, "weirdness_constraint"); ok {
		in.WeirdnessConstraint = &v
	}
	return in, nil
}

// poll waits for the job to finish and returns the first output URL.
func (a *MusicAdapter) poll(ctx context.Context, cfg aiprovider.Config, id string) (string, error) {
	interval := a.PollInterval
	if s := intParam(cfg.Params, "poll_interval_sec", 0); s > 0 {
		interval = time.Duration(s) * time.Second
	}
	p := fmt.Sprintf(resultPath, url.PathEscape(id))
	for {
		var res resultResponse
		if err := a.do(ctx, cfg, http.MethodGet, p, nil, &res); err != nil {
			return "", err
		}
		switch strings.ToLower(res.Status) {
		case "completed":
			for _, u := range res.Outputs {
				if strings.TrimSpace(u) != "" {
					return u, nil
				}
			}
			return "", &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "completed without outputs (request " + id + ")"}
		case "failed", "cancelled":
			msg := res.Status
			if res.Error != nil && *res.Error != "" {
				msg += ": " + *res.Error
			}
			return "", &aiprovider.Error{Kind: failureKind(msg), Provider: providerName, Message: msg + " (request " + id + ")"}
		}
		select {
		case <-ctx.Done():
			return "", &aiprovider.Error{Kind: aiprovider.KindTimeout, Provider: providerName,
				Message: "job not finished in time (request " + id + ", last status " + res.Status + ")", Err: ctx.Err()}
		case <-time.After(interval):
		}
	}
}

// download fetches the output file. The URL points at muapi's storage/CDN, so no API key is sent.
func (a *MusicAdapter) download(ctx context.Context, rawURL string) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, "", &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "output is not a URL: " + snippet([]byte(rawURL))}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build muapi download: %w", err)
	}
	slog.DebugContext(ctx, "ai provider download", "provider", providerName, "url", rawURL)
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, "", transportError(err, "download audio")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", &aiprovider.Error{Kind: aiprovider.KindUnavailable, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "download audio"}
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, maxAudioBytes))
	if err != nil {
		return nil, "", transportError(err, "read audio")
	}
	if len(audio) == 0 {
		return nil, "", &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "empty audio file"}
	}
	return audio, formatOf(resp.Header.Get("Content-Type"), u.Path), nil
}

func (a *MusicAdapter) do(ctx context.Context, cfg aiprovider.Config, method, p string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return fmt.Errorf("encode muapi request: %w", err)
		}
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	endpoint := base + p
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build muapi request: %w", err)
	}
	req.Header.Set("x-api-key", cfg.APIKey)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Debug level: bodies hold the customer's lyrics. The API key is masked.
	slog.DebugContext(ctx, "ai provider request", "provider", providerName, "method", method,
		"url", endpoint, "api_key", aiprovider.MaskKey(cfg.APIKey), "model", cfg.Model,
		"body", aiprovider.LogBody(body, logMaxString, logMaxTotal))
	started := time.Now()

	resp, err := a.http.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "ai provider request failed", "provider", providerName, "url", endpoint,
			"api_key", aiprovider.MaskKey(cfg.APIKey), "duration_ms", time.Since(started).Milliseconds(), "error", err)
		return transportError(err, "request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	slog.DebugContext(ctx, "ai provider response", "provider", providerName, "url", endpoint,
		"api_key", aiprovider.MaskKey(cfg.APIKey), "status", resp.StatusCode,
		"duration_ms", time.Since(started).Milliseconds(), "bytes", len(raw),
		"cost_usd", resp.Header.Get("X-MuAPI-Cost-USD"), "balance_usd", resp.Header.Get("X-Account-Balance"),
		"body", aiprovider.LogBody(raw, logMaxString, logMaxTotal))
	if err != nil {
		return &aiprovider.Error{Kind: aiprovider.KindUnavailable, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "read response", Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := errorMessage(raw)
		return &aiprovider.Error{Kind: kindFromHTTP(resp.StatusCode, msg), Provider: providerName, HTTPStatus: resp.StatusCode, Message: msg}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "decode response", Err: err}
	}
	return nil
}

func transportError(err error, msg string) error {
	kind := aiprovider.KindUnavailable
	if errors.Is(err, context.DeadlineExceeded) {
		kind = aiprovider.KindTimeout
	}
	return &aiprovider.Error{Kind: kind, Provider: providerName, Message: msg, Err: err}
}

// errorMessage pulls a message out of an undocumented error body ({"detail"|"error"|"message": ...}).
func errorMessage(raw []byte) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		for _, k := range []string{"detail", "error", "message"} {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
	}
	return snippet(raw)
}

func kindFromHTTP(status int, msg string) aiprovider.ErrorKind {
	lower := strings.ToLower(msg)
	switch {
	case status == http.StatusPaymentRequired || strings.Contains(lower, "insufficient") ||
		strings.Contains(lower, "balance") || strings.Contains(lower, "credit"):
		return aiprovider.KindInsufficientBalance
	case status == http.StatusTooManyRequests:
		return aiprovider.KindRateLimited
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return aiprovider.KindAuth
	case status == http.StatusGatewayTimeout || status == http.StatusRequestTimeout:
		return aiprovider.KindTimeout
	case status >= 500:
		return aiprovider.KindUnavailable
	default:
		return aiprovider.KindInvalidRequest
	}
}

// failureKind classifies a failed job. Failed jobs are refunded by muapi, so unknown failures retry.
func failureKind(msg string) aiprovider.ErrorKind {
	lower := strings.ToLower(msg)
	for _, w := range []string{"nsfw", "moderat", "sensitive", "policy", "copyright", "artist", "inappropriate"} {
		if strings.Contains(lower, w) {
			return aiprovider.KindContentFlagged
		}
	}
	if strings.HasPrefix(lower, "cancelled") {
		return aiprovider.KindUnknown
	}
	return aiprovider.KindUnavailable
}

func formatOf(contentType, urlPath string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "wav"):
		return "wav"
	case strings.Contains(ct, "mpeg"), strings.Contains(ct, "mp3"):
		return "mp3"
	}
	switch strings.ToLower(path.Ext(urlPath)) {
	case ".wav":
		return "wav"
	default:
		return "mp3"
	}
}

func gender(g string) string {
	switch g = strings.ToLower(strings.TrimSpace(g)); g {
	case "male", "female":
		return g
	default:
		return "" // let the model choose
	}
}

func intParam(params map[string]any, key string, fallback int) int {
	switch v := params[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return fallback
	}
}

func floatParam(params map[string]any, key string) (float64, bool) {
	switch v := params[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
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
