package prompt

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/audit"
	"github.com/yourorg/music-api/internal/module/playground"
	"github.com/yourorg/music-api/internal/module/questionnaire"
)

// Render replaces exact {{name}} only, so "{{ name }}" counts as unknown.
var placeholderPattern = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

// Store is the storage the service needs (GormRepository).
type Store interface {
	List(ctx context.Context) ([]Template, error)
	Get(ctx context.Context, id string) (Template, error)
	Create(ctx context.Context, t Template) (Template, error)
	Activate(ctx context.Context, id string) error
	DeactivateAll(ctx context.Context) error
	Archive(ctx context.Context, id string) error
	Hints(ctx context.Context, a questionnaire.Answers) (Hints, error)
}

// TesterInfo names the provider a playground run used.
type TesterInfo struct {
	Provider string
	Model    string
}

// LyricsTester calls the active providers once for the playground (generation.LyricsTester):
// TestLyrics on the lyrics & style provider, TestMusic on the music provider.
type LyricsTester interface {
	TestLyrics(ctx context.Context, req aiprovider.LyricsStyleRequest, voice questionnaire.Voice) (aiprovider.LyricsStyleResult, TesterInfo, error)
	TestMusic(ctx context.Context, req aiprovider.MusicRequest) (aiprovider.MusicResult, TesterInfo, error)
}

// Auditor records back-office changes (audit.GormRecorder).
type Auditor interface {
	Record(ctx context.Context, e audit.Entry) error
}

// Service manages prompt versions and the playground.
type Service struct {
	store  Store
	tester LyricsTester
	audit  Auditor
	saver  PlaygroundSaver
	now    func() time.Time
}

// PlaygroundSaver keeps every successful playground song (playground.Service): audio at
// playground/<id>.<format>, so the admin can later pick it for the landing page.
type PlaygroundSaver interface {
	Save(ctx context.Context, adminID string, in playground.SaveInput) (string, error)
}

// NewService builds the service.
func NewService(store Store, tester LyricsTester, auditor Auditor) *Service {
	return &Service{store: store, tester: tester, audit: auditor, now: time.Now}
}

// WithPlaygroundSaver makes PlaygroundMusic save each generated song. Without it nothing is stored.
func (s *Service) WithPlaygroundSaver(p PlaygroundSaver) *Service {
	s.saver = p
	return s
}

// List returns every version, the default and the placeholders.
func (s *Service) List(ctx context.Context) (ListOutput, error) {
	ts, err := s.store.List(ctx)
	if err != nil {
		return ListOutput{}, apperror.Internal(err)
	}
	out := ListOutput{Versions: make([]TemplateOutput, len(ts)), Default: toOutput(Default), DefaultInUse: true,
		Placeholders: Placeholders}
	for i, t := range ts {
		out.Versions[i] = toOutput(t)
		if t.IsActive {
			out.DefaultInUse = false
		}
	}
	out.Default.IsActive = out.DefaultInUse
	return out, nil
}

// Create saves a new version. Unknown placeholders are rejected: they would reach the AI as-is.
func (s *Service) Create(ctx context.Context, actorID string, in CreateVersionInput) (TemplateOutput, error) {
	t := Template{Name: strings.TrimSpace(in.Name), SystemPrompt: strings.TrimSpace(in.SystemPrompt),
		UserPrompt: strings.TrimSpace(in.UserPrompt), Note: strings.TrimSpace(in.Note), CreatedByAdminID: actorID}
	var details []apperror.ErrorDetail
	check := func(field, v string) {
		if v == "" {
			details = append(details, apperror.ErrorDetail{Field: field, Message: "is required"})
		} else if len(v) > maxPromptLen {
			details = append(details, apperror.ErrorDetail{Field: field, Message: "terlalu panjang (maks. 20000 karakter)"})
		} else if bad := UnknownPlaceholders(v); len(bad) > 0 {
			details = append(details, apperror.ErrorDetail{Field: field,
				Message: "placeholder tidak dikenal: {{" + strings.Join(bad, "}}, {{") + "}}"})
		}
	}
	check("name", t.Name)
	check("system_prompt", t.SystemPrompt)
	check("user_prompt", t.UserPrompt)
	if len(details) > 0 {
		return TemplateOutput{}, apperror.Validation(details)
	}

	t, err := s.store.Create(ctx, t)
	if err != nil {
		return TemplateOutput{}, apperror.Internal(err)
	}
	s.record(ctx, actorID, "create", t.ID, map[string]any{"version": t.Version, "name": t.Name, "note": t.Note})
	if in.Activate {
		if err := s.store.Activate(ctx, t.ID); err != nil {
			return TemplateOutput{}, mapErr(err)
		}
		t.IsActive = true
		s.record(ctx, actorID, "activate", t.ID, map[string]any{"version": t.Version})
	}
	return toOutput(t), nil
}

// Activate makes a saved version the one new jobs use (also how an admin rolls back).
func (s *Service) Activate(ctx context.Context, actorID, id string) (TemplateOutput, error) {
	t, err := s.store.Get(ctx, id)
	if err != nil {
		return TemplateOutput{}, mapErr(err)
	}
	if err := s.store.Activate(ctx, id); err != nil {
		return TemplateOutput{}, mapErr(err)
	}
	t.IsActive = true
	s.record(ctx, actorID, "activate", id, map[string]any{"version": t.Version})
	return toOutput(t), nil
}

// UseDefault turns every saved version off; new jobs use the built-in Default.
func (s *Service) UseDefault(ctx context.Context, actorID string) error {
	if err := s.store.DeactivateAll(ctx); err != nil {
		return apperror.Internal(err)
	}
	s.record(ctx, actorID, "use_default", "", nil)
	return nil
}

// Archive hides an inactive version. The active one must be replaced first.
func (s *Service) Archive(ctx context.Context, actorID, id string) error {
	t, err := s.store.Get(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if t.IsActive {
		return apperror.Conflict("versi aktif tidak bisa diarsipkan; aktifkan versi lain dulu")
	}
	if err := s.store.Archive(ctx, id); err != nil {
		return mapErr(err)
	}
	s.record(ctx, actorID, "archive", id, map[string]any{"version": t.Version})
	return nil
}

// Preview renders a draft with sample answers, without calling the AI.
func (s *Service) Preview(ctx context.Context, in PlaygroundInput) (RenderOutput, error) {
	a := in.Answers.toAnswers()
	hints, err := s.store.Hints(ctx, a)
	if err != nil {
		return RenderOutput{}, apperror.Internal(err)
	}
	system, user := Render(Template{SystemPrompt: in.SystemPrompt, UserPrompt: in.UserPrompt}, a, hints)
	unknown := UnknownPlaceholders(in.SystemPrompt + "\n" + in.UserPrompt)
	if unknown == nil {
		unknown = []string{}
	}
	return RenderOutput{SystemPrompt: system, UserPrompt: user, UnknownPlaceholders: unknown}, nil
}

// Playground renders a draft and runs it once on the active lyrics & style provider.
func (s *Service) Playground(ctx context.Context, in PlaygroundInput) (PlaygroundOutput, error) {
	rendered, err := s.Preview(ctx, in)
	if err != nil {
		return PlaygroundOutput{}, err
	}
	out := PlaygroundOutput{Rendered: rendered}
	t0 := s.now()
	res, info, err := s.tester.TestLyrics(ctx,
		aiprovider.LyricsStyleRequest{SystemPrompt: rendered.SystemPrompt, UserPrompt: rendered.UserPrompt},
		in.Answers.toAnswers().Voice)
	out.DurationMS = s.now().Sub(t0).Milliseconds()
	out.Provider, out.Model = info.Provider, info.Model
	if err != nil {
		out.ErrorKind, out.Error = playgroundError(err)
		var pe *aiprovider.Error
		if errors.As(err, &pe) && pe.Kind == aiprovider.KindInvalidOutput {
			out.Raw = res.Raw
		}
		return out, nil
	}
	out.OK, out.Raw = true, res.Raw
	out.Result = &LyricsOutput{Title: res.Title, Lyrics: res.Lyrics, StylePrompt: res.StylePrompt,
		VocalGender: res.VocalGender, Language: res.Language, Mood: res.Mood}
	return out, nil
}

// PlaygroundMusic runs step 2 of the playground: lyrics + style prompt (usually from step 1, maybe
// edited) once on the active music provider. Nothing is stored; the audio goes back base64-encoded.
func (s *Service) PlaygroundMusic(ctx context.Context, adminID string, in MusicPlaygroundInput) (MusicPlaygroundOutput, error) {
	var out MusicPlaygroundOutput
	t0 := s.now()
	res, info, err := s.tester.TestMusic(ctx, aiprovider.MusicRequest{Lyrics: in.Lyrics, StylePrompt: in.StylePrompt})
	out.DurationMS = s.now().Sub(t0).Milliseconds()
	out.Provider, out.Model = info.Provider, info.Model
	if err != nil {
		out.ErrorKind, out.Error = playgroundError(err)
		return out, nil
	}
	format := res.Format
	if format == "" {
		format = "mp3"
	}
	out.OK, out.Format, out.AudioDurationSec, out.SizeBytes = true, format, res.DurationSec, len(res.Audio)
	out.AudioBase64 = base64.StdEncoding.EncodeToString(res.Audio)
	if s.saver != nil {
		id, err := s.saver.Save(ctx, adminID, playground.SaveInput{
			Title: in.Title, Lyrics: in.Lyrics, StylePrompt: in.StylePrompt, Audio: res.Audio, Format: format,
			DurationSec: res.DurationSec, Provider: info.Provider, Model: info.Model,
		})
		if err != nil {
			// The admin still hears the song; they are told it was not kept.
			slog.ErrorContext(ctx, "save playground song failed", "error", err)
			out.SaveError = "Lagu berhasil dibuat tetapi gagal disimpan ke storage: " + err.Error()
		}
		out.SavedSongID = id
	}
	return out, nil
}

// UnknownPlaceholders returns the {{names}} in text that Render does not fill, in order, once each.
func UnknownPlaceholders(text string) []string {
	var out []string
	for _, m := range placeholderPattern.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if !slices.Contains(Placeholders, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func playgroundError(err error) (kind, msg string) {
	if errors.Is(err, ErrNoProvider) {
		return "no_active_provider", "Belum ada provider Lyrics & Style yang aktif. Aktifkan di halaman AI Providers."
	}
	if errors.Is(err, ErrNoMusicProvider) {
		return "no_active_provider", "Belum ada provider Music yang aktif. Aktifkan di halaman AI Providers."
	}
	var pe *aiprovider.Error
	if errors.As(err, &pe) {
		return string(pe.Kind), pe.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return string(aiprovider.KindTimeout), "provider tidak menjawab tepat waktu"
	}
	return string(aiprovider.KindUnknown), err.Error()
}

// ErrNoProvider is returned by a LyricsTester when no usable lyrics & style provider is active.
var ErrNoProvider = errors.New("no active lyrics & style provider")

// ErrNoMusicProvider is returned by TestMusic when no usable music provider is active.
var ErrNoMusicProvider = errors.New("no active music provider")

func (s *Service) record(ctx context.Context, actorID, action, id string, diff map[string]any) {
	err := s.audit.Record(ctx, audit.Entry{ActorType: "admin", ActorID: actorID, Action: "prompt_template." + action,
		EntityType: "prompt_template", EntityID: id, Diff: diff})
	if err != nil {
		slog.ErrorContext(ctx, "audit log write failed", "action", action, "entity_id", id, "error", err)
	}
}

func mapErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("versi prompt tidak ditemukan")
	}
	return apperror.Internal(err)
}
