// Package minimax holds the MiniMax adapters: chat completion for lyrics & style
// (POST /v1/text/chatcompletion_v2) and music generation for audio (POST /v1/music_generation).
// Shapes and limits follow platform.minimax.io docs (read 2026-11-15).
package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

const (
	providerName = "minimax"
	// DefaultBaseURL is the international API host. Admins can override it per provider.
	DefaultBaseURL = "https://api.minimax.io"
	defaultTimeout = 5 * time.Minute
	maxBodyBytes   = 64 << 20 // music responses carry hex audio
	// Log limits: a JSON string longer than logMaxString (the hex audio) is replaced by its length,
	// and a logged body is cut at logMaxTotal. Lyrics replies (~3-5 KB) stay whole.
	logMaxString = 8000
	logMaxTotal  = 32000
)

// Register adds both MiniMax adapters to r, sharing one HTTP client.
func Register(r *aiprovider.Registry) {
	c := &http.Client{}
	r.RegisterLyricsStyle(aiprovider.AdapterMiniMaxText, NewTextAdapter(c))
	r.RegisterMusic(aiprovider.AdapterMiniMaxMusic, NewMusicAdapter(c))
}

// baseResp is MiniMax's status block, present in every response.
type baseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// client does one JSON POST. Retries are the caller's job (see aiprovider.IsRetryable).
type client struct {
	http *http.Client
}

func (c client) post(ctx context.Context, cfg aiprovider.Config, path string, in, out any) error {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("encode minimax request: %w", err)
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build minimax request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	// Debug level: bodies hold the customer's answers and lyrics. The API key is masked.
	url := base + path
	slog.DebugContext(ctx, "ai provider request", "provider", providerName, "method", http.MethodPost,
		"url", url, "api_key", aiprovider.MaskKey(cfg.APIKey), "model", cfg.Model,
		"timeout", timeout.String(), "body", aiprovider.LogBody(body, logMaxString, logMaxTotal))
	started := time.Now()

	resp, err := c.http.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "ai provider request failed", "provider", providerName, "url", url,
			"api_key", aiprovider.MaskKey(cfg.APIKey), "duration_ms", time.Since(started).Milliseconds(), "error", err)
		kind := aiprovider.KindUnavailable
		if errors.Is(err, context.DeadlineExceeded) {
			kind = aiprovider.KindTimeout
		}
		return &aiprovider.Error{Kind: kind, Provider: providerName, Message: "request failed", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	slog.DebugContext(ctx, "ai provider response", "provider", providerName, "url", url,
		"api_key", aiprovider.MaskKey(cfg.APIKey), "status", resp.StatusCode,
		"duration_ms", time.Since(started).Milliseconds(), "bytes", len(raw),
		"body", aiprovider.LogBody(raw, logMaxString, logMaxTotal))
	if err != nil {
		return &aiprovider.Error{Kind: aiprovider.KindUnavailable, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "read response", Err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return &aiprovider.Error{Kind: kindFromHTTP(resp.StatusCode), Provider: providerName, HTTPStatus: resp.StatusCode, Message: snippet(raw)}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "decode response", Err: err}
	}
	return nil
}

// checkBase turns a non-zero base_resp.status_code into an *aiprovider.Error.
func checkBase(b baseResp) error {
	if b.StatusCode == 0 {
		return nil
	}
	return &aiprovider.Error{Kind: kindFromCode(b.StatusCode), Provider: providerName, ProviderCode: b.StatusCode, Message: b.StatusMsg}
}

// kindFromCode maps MiniMax status codes (docs: api-reference/errorcode).
func kindFromCode(code int) aiprovider.ErrorKind {
	switch code {
	case 1002, 1045:
		return aiprovider.KindRateLimited
	case 1001:
		return aiprovider.KindTimeout
	case 1000, 1013, 1024, 2064:
		return aiprovider.KindUnavailable
	case 1004, 2049:
		return aiprovider.KindAuth
	case 1008, 2053, 2056:
		return aiprovider.KindInsufficientBalance
	case 1026, 1027, 1046:
		return aiprovider.KindContentFlagged
	case 1039, 2013, 2014, 2022, 2061, 2068, 2069:
		return aiprovider.KindInvalidRequest
	default:
		return aiprovider.KindUnknown
	}
}

func kindFromHTTP(status int) aiprovider.ErrorKind {
	switch {
	case status == http.StatusTooManyRequests:
		return aiprovider.KindRateLimited
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return aiprovider.KindAuth
	case status >= 500:
		return aiprovider.KindUnavailable
	default:
		return aiprovider.KindInvalidRequest
	}
}

func snippet(b []byte) string {
	const limit = 300
	s := strings.TrimSpace(string(b))
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}

// intParam reads a numeric param (JSON numbers decode as float64) with a fallback.
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

// Catalog describes the MiniMax adapters for the admin provider form.
func Catalog() []aiprovider.AdapterInfo {
	return []aiprovider.AdapterInfo{
		{Type: aiprovider.AdapterMiniMaxText, Category: aiprovider.CategoryLyricsStyle, Label: "MiniMax Text",
			DefaultBaseURL: DefaultBaseURL, DefaultModel: DefaultTextModel, Models: []string{DefaultTextModel},
			Params: []aiprovider.ParamSpec{
				{Key: "temperature", Label: "Temperature", Kind: aiprovider.ParamFloat, Min: aiprovider.Bound(0.01),
					Max: aiprovider.Bound(1), Placeholder: "default model"},
				{Key: "max_completion_tokens", Label: "Maks. token output", Kind: aiprovider.ParamInt,
					Min: aiprovider.Bound(256), Max: aiprovider.Bound(32768), Placeholder: strconv.Itoa(defaultMaxCompletionToken)},
			}},
		{Type: aiprovider.AdapterMiniMaxMusic, Category: aiprovider.CategoryMusic, Label: "MiniMax Music",
			DefaultBaseURL: DefaultBaseURL, DefaultModel: DefaultMusicModel, Models: []string{DefaultMusicModel},
			Params: []aiprovider.ParamSpec{
				{Key: "format", Label: "Format audio", Kind: aiprovider.ParamString, Options: []string{"mp3", "wav"}, Placeholder: "mp3"},
				{Key: "sample_rate", Label: "Sample rate", Kind: aiprovider.ParamInt,
					Options: []string{"16000", "24000", "32000", "44100"}, Placeholder: "44100"},
				{Key: "bitrate", Label: "Bitrate", Kind: aiprovider.ParamInt,
					Options: []string{"32000", "64000", "128000", "256000"}, Placeholder: "256000"},
			}},
	}
}
