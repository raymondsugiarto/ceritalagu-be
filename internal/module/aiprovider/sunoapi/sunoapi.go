// Package sunoapi holds the Suno music adapter served through sunoapi.org (an unofficial Suno reseller).
// Flow (docs.sunoapi.org, read 2026-11-15):
//  1. POST /api/v1/generate                         -> {"code":200,"msg":"success","data":{"taskId":"..."}}
//  2. GET  /api/v1/generate/record-info?taskId=...  -> poll until data.status is SUCCESS or a failure state
//  3. GET  data.response.sunoData[i].audio_url      -> the audio files (no API key is sent to that host).
//     Suno makes two takes per request: [0] is the song, [1] is kept as a spare for "Buat Ulang".
//  4. POST /api/v1/generate/get-timestamped-lyrics  {taskId, audioId} -> data.alignedWords per take (best effort:
//     the preview is cut at the end of the first chorus with them; a failure only loses the timestamps)
//
// Auth is "Authorization: Bearer <key>". Errors come back as a "code" in the JSON body (the HTTP
// status may still be 200), so both are checked. callBackUrl is required by the API, but this
// adapter polls instead; the callback target is configurable and defaults to an unreachable host so
// the song data is never posted to a third party.
package sunoapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

const (
	providerName = "sunoapi"
	// DefaultBaseURL is the sunoapi.org API host. Admins can override it per provider.
	DefaultBaseURL = "https://api.sunoapi.org"
	// DefaultModel is the Suno version sunoapi.org recommends.
	DefaultModel = "V6"
	// DefaultCallbackURL satisfies the required callBackUrl without sending results anywhere.
	DefaultCallbackURL = "https://localhost/sunoapi/callback"
	// MaxLyricsChars is the documented lyrics limit (V4_5+ and V6).
	MaxLyricsChars = 5000
	// MaxTitleChars is the documented title limit for every model.
	MaxTitleChars = 80

	generatePath        = "/api/v1/generate"
	recordPath          = "/api/v1/generate/record-info"
	timestampPath       = "/api/v1/generate/get-timestamped-lyrics"
	creditPath          = "/api/v1/generate/credit" //nolint:gosec // URL path, not a credential
	defaultTimeout      = 10 * time.Minute          // covers submit + polling + download
	defaultPollInterval = 10 * time.Second          // docs suggest 30 s; songs usually finish in 1-3 min
	maxBodyBytes        = 1 << 20
	maxAudioBytes       = 100 << 20
	logMaxString        = 8000
	logMaxTotal         = 32000
)

// Models are the Suno versions sunoapi.org accepts (model enum); V6* are current, the rest deprecated.
var Models = []string{"V6", "V6_MINI", "V6_WILD", "V5_5", "V5", "V4_5PLUS", "V4_5ALL", "V4_5", "V4"}

// Register adds the sunoapi.org Suno adapter to r.
func Register(r *aiprovider.Registry) {
	r.RegisterMusic(aiprovider.AdapterSunoAPISuno, NewMusicAdapter(&http.Client{}))
}

// Catalog describes the adapter for the admin provider form.
func Catalog() []aiprovider.AdapterInfo {
	return []aiprovider.AdapterInfo{
		{Type: aiprovider.AdapterSunoAPISuno, Category: aiprovider.CategoryMusic, Label: "Suno (sunoapi.org)",
			Help: "API key dari sunoapi.org/api-key. Kredit prabayar; tombol Test menampilkan sisa kredit. " +
				"File hasil disimpan sunoapi.org 14 hari, jadi audio langsung diunduh ke storage kita.",
			DefaultBaseURL: DefaultBaseURL, DefaultModel: DefaultModel, Models: Models,
			Params: []aiprovider.ParamSpec{
				{Key: "vocal_gender", Label: "Suara vokal", Kind: aiprovider.ParamString, Options: []string{"male", "female"},
					Placeholder: "Ikuti hasil lirik", Help: "Kosong = pakai gender vokal dari langkah lirik."},
				{Key: "duration", Label: "Durasi (detik)", Kind: aiprovider.ParamInt, Min: aiprovider.Bound(10),
					Max: aiprovider.Bound(360), Placeholder: "otomatis", Help: "Hanya V5_5 dan V6*."},
				{Key: "negative_tags", Label: "Negative tags", Kind: aiprovider.ParamString,
					Placeholder: "heavy metal, screaming", Help: "Gaya yang harus dihindari, dipisah koma."},
				{Key: "style_weight", Label: "Style weight", Kind: aiprovider.ParamFloat, Min: aiprovider.Bound(0),
					Max: aiprovider.Bound(1), Placeholder: "0–1"},
				{Key: "weirdness_constraint", Label: "Weirdness", Kind: aiprovider.ParamFloat, Min: aiprovider.Bound(0),
					Max: aiprovider.Bound(1), Placeholder: "0–1"},
				{Key: "audio_weight", Label: "Audio weight", Kind: aiprovider.ParamFloat, Min: aiprovider.Bound(0),
					Max: aiprovider.Bound(1), Placeholder: "0–1"},
				{Key: "poll_interval_sec", Label: "Interval polling (detik)", Kind: aiprovider.ParamInt,
					Min: aiprovider.Bound(2), Max: aiprovider.Bound(60), Placeholder: "10"},
				{Key: "callback_url", Label: "Callback URL", Kind: aiprovider.ParamString,
					Placeholder: DefaultCallbackURL,
					Help:        "Wajib di API sunoapi.org, tapi hasil diambil lewat polling. Kosongkan kecuali butuh."},
			}},
	}
}

type generateRequest struct {
	Prompt              string   `json:"prompt"` // the exact lyrics in custom mode
	Style               string   `json:"style"`
	Title               string   `json:"title,omitempty"`
	CustomMode          bool     `json:"customMode"`
	Instrumental        bool     `json:"instrumental"`
	Model               string   `json:"model"`
	CallBackURL         string   `json:"callBackUrl"`
	NegativeTags        string   `json:"negativeTags,omitempty"`
	VocalGender         string   `json:"vocalGender,omitempty"` // m | f
	Duration            int      `json:"duration,omitempty"`
	StyleWeight         *float64 `json:"styleWeight,omitempty"`
	WeirdnessConstraint *float64 `json:"weirdnessConstraint,omitempty"`
	AudioWeight         *float64 `json:"audioWeight,omitempty"`
}

// envelope is every sunoapi.org reply: a body code (200 = ok), a message and the payload.
type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type generateData struct {
	TaskID string `json:"taskId"`
}

type recordData struct {
	TaskID       string  `json:"taskId"`
	Status       string  `json:"status"`
	ErrorCode    *int    `json:"errorCode"`
	ErrorMessage *string `json:"errorMessage"`
	Response     *struct {
		SunoData []track `json:"sunoData"`
	} `json:"response"`
}

// track accepts both snake_case (reference docs) and camelCase (quickstart) field names.
type track struct {
	ID             string  `json:"id"` // audioId
	AudioURL       string  `json:"audio_url"`
	AudioURLCamel  string  `json:"audioUrl"`
	StreamURL      string  `json:"stream_audio_url"`
	StreamURLCamel string  `json:"streamAudioUrl"`
	Duration       float64 `json:"duration"`
}

func (t track) audioURL() string {
	for _, u := range []string{t.AudioURL, t.AudioURLCamel, t.StreamURL, t.StreamURLCamel} {
		if u = strings.TrimSpace(u); u != "" {
			return u
		}
	}
	return ""
}

// MusicAdapter generates a song with Suno through sunoapi.org: submit, poll, download.
//
// Params (ai_provider.params), all optional:
//   - vocal_gender (male|female): overrides the gender from the lyrics step
//   - duration (int, 10-360 s): only V5_5 / V6*; unset lets the model choose
//   - negative_tags (string), style_weight / weirdness_constraint / audio_weight (0-1)
//   - poll_interval_sec (int, default 10)
//   - callback_url (string, default DefaultCallbackURL)
//
// The provider timeout (timeout_sec) bounds the whole flow; default 10 minutes.
type MusicAdapter struct {
	http *http.Client
	// PollInterval is the default wait between polls (tests shorten it).
	PollInterval time.Duration
}

// NewMusicAdapter returns the music adapter.
func NewMusicAdapter(h *http.Client) *MusicAdapter {
	return &MusicAdapter{http: h, PollInterval: defaultPollInterval}
}

var _ aiprovider.MusicProvider = (*MusicAdapter)(nil)

// GenerateMusic submits a custom-mode, vocal Suno job. Suno makes two takes per request: the first
// is the result, the second (best effort) is returned as Alternate, a spare for "Buat Ulang".
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

	var created generateData
	if err := a.do(ctx, cfg, http.MethodPost, generatePath, in, &created); err != nil {
		return aiprovider.MusicResult{}, err
	}
	if created.TaskID == "" {
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "no taskId in generate response"}
	}

	tracks, err := a.poll(ctx, cfg, created.TaskID)
	if err != nil {
		return aiprovider.MusicResult{}, err
	}
	t := tracks[0]
	audio, format, err := a.download(ctx, t.audioURL())
	if err != nil {
		return aiprovider.MusicResult{}, err
	}
	res := aiprovider.MusicResult{Audio: audio, Format: format, DurationSec: int(math.Round(t.Duration)),
		Words: a.words(ctx, cfg, created.TaskID, t.ID)}
	if len(tracks) > 1 {
		res.Alternate = a.alternate(ctx, cfg, created.TaskID, tracks[1])
	}
	return res, nil
}

// alternate downloads the second take. A failure only loses the spare, so it is logged and nil returned.
func (a *MusicAdapter) alternate(ctx context.Context, cfg aiprovider.Config, taskID string, t track) *aiprovider.MusicTrack {
	audio, format, err := a.download(ctx, t.audioURL())
	if err != nil {
		slog.WarnContext(ctx, "sunoapi second track download failed", "provider", providerName, "task_id", taskID, "audio_id", t.ID, "err", err)
		return nil
	}
	return &aiprovider.MusicTrack{Audio: audio, Format: format, DurationSec: int(math.Round(t.Duration)),
		Words: a.words(ctx, cfg, taskID, t.ID)}
}

type timestampRequest struct {
	TaskID  string `json:"taskId"`
	AudioID string `json:"audioId"`
}

type timestampData struct {
	AlignedWords []struct {
		Word    string  `json:"word"`
		Success bool    `json:"success"`
		StartS  float64 `json:"startS"`
		EndS    float64 `json:"endS"`
	} `json:"alignedWords"`
}

// words fetches the word timestamps of the track. Failures are logged and return nil: the song is
// still good, only the preview cut falls back to an estimate.
func (a *MusicAdapter) words(ctx context.Context, cfg aiprovider.Config, taskID, audioID string) []aiprovider.AlignedWord {
	if audioID == "" {
		slog.WarnContext(ctx, "sunoapi track has no id, skipping lyric timestamps", "provider", providerName, "task_id", taskID)
		return nil
	}
	var out timestampData
	if err := a.do(ctx, cfg, http.MethodPost, timestampPath, timestampRequest{TaskID: taskID, AudioID: audioID}, &out); err != nil {
		slog.WarnContext(ctx, "sunoapi lyric timestamps failed", "provider", providerName, "task_id", taskID, "err", err)
		return nil
	}
	words := make([]aiprovider.AlignedWord, 0, len(out.AlignedWords))
	for _, w := range out.AlignedWords {
		if !w.Success || w.EndS <= 0 {
			continue
		}
		words = append(words, aiprovider.AlignedWord{Text: w.Word, StartSec: w.StartS, EndSec: w.EndS})
	}
	return words
}

// TestConnection checks the key with GET /api/v1/generate/credit, which generates nothing.
func (a *MusicAdapter) TestConnection(ctx context.Context, cfg aiprovider.Config) error {
	timeout := cfg.Timeout
	if timeout <= 0 || timeout > time.Minute {
		timeout = time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var credits float64
	if err := a.do(ctx, cfg, http.MethodGet, creditPath, nil, &credits); err != nil {
		return err
	}
	slog.InfoContext(ctx, "sunoapi credits", "provider", providerName, "credits", credits)
	if credits <= 0 {
		return &aiprovider.Error{Kind: aiprovider.KindInsufficientBalance, Provider: providerName, Message: "key is valid but has 0 credits"}
	}
	return nil
}

func buildRequest(req aiprovider.MusicRequest, cfg aiprovider.Config) (generateRequest, error) {
	invalid := func(msg string) error {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidRequest, Provider: providerName, Message: msg}
	}
	lyrics := strings.TrimSpace(req.Lyrics)
	switch n := utf8.RuneCountInString(lyrics); {
	case n == 0:
		return generateRequest{}, invalid("lyrics are required")
	case n > MaxLyricsChars:
		return generateRequest{}, invalid(fmt.Sprintf("lyrics have %d characters, max %d", n, MaxLyricsChars))
	}
	style := strings.TrimSpace(req.StylePrompt)
	if style == "" {
		return generateRequest{}, invalid("style prompt is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	in := generateRequest{
		Prompt: lyrics, Style: truncate(style, styleLimit(model)), Title: truncate(strings.TrimSpace(req.Title), MaxTitleChars),
		CustomMode: true, Instrumental: false, Model: model,
		CallBackURL:  stringParam(cfg.Params, "callback_url", DefaultCallbackURL),
		NegativeTags: stringParam(cfg.Params, "negative_tags", ""),
		VocalGender:  gender(stringParam(cfg.Params, "vocal_gender", req.VocalGender)),
		Duration:     intParam(cfg.Params, "duration", 0),
	}
	if in.Duration != 0 && (in.Duration < 10 || in.Duration > 360) {
		return generateRequest{}, invalid(fmt.Sprintf("duration %d s is out of range (10-360)", in.Duration))
	}
	if v, ok := floatParam(cfg.Params, "style_weight"); ok {
		in.StyleWeight = &v
	}
	if v, ok := floatParam(cfg.Params, "weirdness_constraint"); ok {
		in.WeirdnessConstraint = &v
	}
	if v, ok := floatParam(cfg.Params, "audio_weight"); ok {
		in.AudioWeight = &v
	}
	return in, nil
}

// styleLimit is the documented style length: 200 characters for V4, 1000 for newer models.
func styleLimit(model string) int {
	if strings.EqualFold(model, "V4") {
		return 200
	}
	return 1000
}

// poll waits for SUCCESS and returns the tracks that have an audio URL (at least one).
func (a *MusicAdapter) poll(ctx context.Context, cfg aiprovider.Config, taskID string) ([]track, error) {
	interval := a.PollInterval
	if s := intParam(cfg.Params, "poll_interval_sec", 0); s > 0 {
		interval = time.Duration(s) * time.Second
	}
	p := recordPath + "?taskId=" + url.QueryEscape(taskID)
	for {
		var rec recordData
		if err := a.do(ctx, cfg, http.MethodGet, p, nil, &rec); err != nil {
			return nil, err
		}
		status := strings.ToUpper(strings.TrimSpace(rec.Status))
		switch status {
		case "SUCCESS":
			if ts := rec.withAudio(); len(ts) > 0 {
				return ts, nil
			}
			return nil, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "SUCCESS without audio (task " + taskID + ")"}
		case "CREATE_TASK_FAILED", "GENERATE_AUDIO_FAILED", "SENSITIVE_WORD_ERROR", "CALLBACK_EXCEPTION", "FAILED":
			// CALLBACK_EXCEPTION only means our callback was unreachable; if audio exists, use it.
			if status == "CALLBACK_EXCEPTION" {
				if ts := rec.withAudio(); len(ts) > 0 {
					return ts, nil
				}
			}
			msg := status
			if rec.ErrorMessage != nil && *rec.ErrorMessage != "" {
				msg += ": " + *rec.ErrorMessage
			}
			if rec.ErrorCode != nil {
				msg += fmt.Sprintf(" (code %d)", *rec.ErrorCode)
			}
			return nil, &aiprovider.Error{Kind: failureKind(status, msg), Provider: providerName, Message: msg + " (task " + taskID + ")"}
		}
		// PENDING, TEXT_SUCCESS, FIRST_SUCCESS: keep waiting for the full tracks.
		select {
		case <-ctx.Done():
			return nil, &aiprovider.Error{Kind: aiprovider.KindTimeout, Provider: providerName,
				Message: "job not finished in time (task " + taskID + ", last status " + rec.Status + ")", Err: ctx.Err()}
		case <-time.After(interval):
		}
	}
}

// withAudio returns the finished tracks that have an audio URL, in the order sunoapi.org lists them.
func (r recordData) withAudio() []track {
	if r.Response == nil {
		return nil
	}
	var out []track
	for _, t := range r.Response.SunoData {
		if t.audioURL() != "" {
			out = append(out, t)
		}
	}
	return out
}

// download fetches the output file from sunoapi.org's CDN; no API key is sent.
func (a *MusicAdapter) download(ctx context.Context, rawURL string) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, "", &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "audio is not a URL: " + snippet([]byte(rawURL))}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build sunoapi download: %w", err)
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

// do sends one API call and decodes envelope.data into out. A non-200 body code is an error.
func (a *MusicAdapter) do(ctx context.Context, cfg aiprovider.Config, method, p string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return fmt.Errorf("encode sunoapi request: %w", err)
		}
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	endpoint := base + p
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build sunoapi request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Accept", "application/json")
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
		"body", aiprovider.LogBody(raw, logMaxString, logMaxTotal))
	if err != nil {
		return &aiprovider.Error{Kind: aiprovider.KindUnavailable, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "read response", Err: err}
	}

	var env envelope
	decodeErr := json.Unmarshal(raw, &env)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code, msg := resp.StatusCode, snippet(raw)
		if decodeErr == nil && env.Code != 0 {
			code, msg = env.Code, or(env.Msg, msg)
		}
		return &aiprovider.Error{Kind: kindFromCode(code), Provider: providerName, HTTPStatus: resp.StatusCode, Message: fmt.Sprintf("code %d: %s", code, msg)}
	}
	if decodeErr != nil {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "decode response", Err: decodeErr}
	}
	if env.Code != http.StatusOK {
		return &aiprovider.Error{Kind: kindFromCode(env.Code), Provider: providerName, HTTPStatus: resp.StatusCode, Message: fmt.Sprintf("code %d: %s", env.Code, env.Msg)}
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "response has no data"}
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, HTTPStatus: resp.StatusCode, Message: "decode response data", Err: err}
	}
	return nil
}

// kindFromCode maps sunoapi.org's documented codes (body or HTTP) to error kinds.
func kindFromCode(code int) aiprovider.ErrorKind {
	switch code {
	case 401, 403:
		return aiprovider.KindAuth
	case 429: // "Insufficient credits" in sunoapi.org's table, not rate limiting
		return aiprovider.KindInsufficientBalance
	case 405, 430: // rate limit / call frequency too high
		return aiprovider.KindRateLimited
	case 408, 504:
		return aiprovider.KindTimeout
	case 455: // maintenance
		return aiprovider.KindUnavailable
	case 400, 404, 413, 422:
		return aiprovider.KindInvalidRequest
	}
	if code >= 500 {
		return aiprovider.KindUnavailable
	}
	return aiprovider.KindInvalidRequest
}

// failureKind classifies a failed task. Content rejections never retry; others may.
func failureKind(status, msg string) aiprovider.ErrorKind {
	if status == "SENSITIVE_WORD_ERROR" {
		return aiprovider.KindContentFlagged
	}
	lower := strings.ToLower(msg)
	for _, w := range []string{"sensitive", "moderat", "policy", "copyright", "artist", "inappropriate", "violat"} {
		if strings.Contains(lower, w) {
			return aiprovider.KindContentFlagged
		}
	}
	if strings.Contains(lower, "credit") || strings.Contains(lower, "insufficient") {
		return aiprovider.KindInsufficientBalance
	}
	return aiprovider.KindUnavailable
}

func transportError(err error, msg string) error {
	kind := aiprovider.KindUnavailable
	if errors.Is(err, context.DeadlineExceeded) {
		kind = aiprovider.KindTimeout
	}
	return &aiprovider.Error{Kind: kind, Provider: providerName, Message: msg, Err: err}
}

func formatOf(contentType, urlPath string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "wav"):
		return "wav"
	case strings.Contains(ct, "mpeg"), strings.Contains(ct, "mp3"):
		return "mp3"
	}
	if strings.EqualFold(path.Ext(urlPath), ".wav") {
		return "wav"
	}
	return "mp3"
}

// gender maps male|female to sunoapi.org's m|f; anything else lets the model choose.
func gender(g string) string {
	switch strings.ToLower(strings.TrimSpace(g)) {
	case "male", "m":
		return "m"
	case "female", "f":
		return "f"
	default:
		return ""
	}
}

func truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:limit]))
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
	if v, ok := params[key].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func snippet(b []byte) string {
	const limit = 300
	s := strings.TrimSpace(string(b))
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}
