package generation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/prompt"
	"github.com/yourorg/music-api/internal/module/questionnaire"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/storage"
)

// Pipeline-level error codes (provider failures use aiprovider.ErrorKind as the code).
const (
	CodeNoProvider = "no_active_provider"
	CodeInternal   = "internal"
)

// Pipeline generates songs. Safe for concurrent use; one Run per job.
type Pipeline struct {
	jobs     Jobs
	answers  Questionnaires
	prompts  Prompts
	provs    Providers
	registry *aiprovider.Registry
	store    Storage
	audio    Audio
	notifier Notifier
	log      *slog.Logger
	opts     Options

	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
	newID func() string
}

// Deps groups the Pipeline's collaborators.
type Deps struct {
	Jobs           Jobs
	Questionnaires Questionnaires
	Prompts        Prompts
	Providers      Providers
	Registry       *aiprovider.Registry
	Storage        Storage
	Audio          Audio
	Notifier       Notifier // optional; nil skips the song-ready email
	Log            *slog.Logger
}

// NewPipeline builds a pipeline.
func NewPipeline(d Deps, opts Options) *Pipeline {
	return &Pipeline{
		jobs: d.Jobs, answers: d.Questionnaires, prompts: d.Prompts, provs: d.Providers,
		registry: d.Registry, store: d.Storage, audio: d.Audio, notifier: d.Notifier, log: d.Log,
		opts:  opts.withDefaults(),
		now:   func() time.Time { return time.Now().UTC() },
		sleep: sleepCtx,
		newID: uuid.NewString,
	}
}

// Run executes the job. A finished job is a no-op, so a redelivered queue message is harmless.
// On failure the job is saved as failed and the error is returned for the worker to log.
func (p *Pipeline) Run(ctx context.Context, jobID string) error {
	job, err := p.jobs.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job %s: %w", jobID, err)
	}
	if job.Status.IsFinal() {
		return nil
	}
	log := p.log.With("job_id", job.ID)
	started := p.now()
	job.StartedAt, job.Attempt = &started, job.Attempt+1
	if job.Timings == nil {
		job.Timings = map[string]int64{}
	}

	s, alt, err := p.run(ctx, &job)
	if err != nil {
		log.Error("song job failed", "status_at_failure", job.Status, "attempt", job.Attempt,
			"questionnaire_id", job.QuestionnaireID, "error_code", errorCode(err), "err", err)
		return p.fail(ctx, &job, err)
	}

	if err := p.jobs.CreateSong(ctx, s); err != nil {
		return p.fail(ctx, &job, fmt.Errorf("save song: %w", err))
	}
	finished := p.now()
	job.Status, job.FinishedAt = song.JobCompleted, &finished
	job.Timings["total"] = finished.Sub(started).Milliseconds()
	if err := p.jobs.UpdateJob(ctx, job); err != nil {
		return fmt.Errorf("save completed job: %w", err)
	}
	log.Info("song job completed", "song_id", s.ID, "total_ms", job.Timings["total"])
	if alt != nil {
		if err := p.jobs.CreateAlternate(ctx, *alt); err != nil {
			log.Warn("save spare take failed", "alternate_song_id", alt.SongID, "err", err)
		} else {
			log.Info("spare take saved", "alternate_song_id", alt.SongID, "root_job_id", alt.RootJobID)
		}
	}
	p.notify(ctx, s)
	return nil
}

// notify emails the customer that their song is ready. The song is already saved, so a failed
// email is logged and the job stays completed.
func (p *Pipeline) notify(ctx context.Context, s song.Song) {
	if p.notifier == nil {
		return
	}
	if err := p.notifier.SongReady(ctx, s); err != nil {
		p.log.Warn("song-ready email failed", "song_id", s.ID, "err", err)
	}
}

func (p *Pipeline) run(ctx context.Context, job *song.Job) (song.Song, *song.Alternate, error) {
	q, err := p.answers.Get(ctx, job.QuestionnaireID)
	if err != nil {
		return song.Song{}, nil, fmt.Errorf("load questionnaire: %w", err)
	}

	// 1. Lyrics & style.
	if err := p.setStatus(ctx, job, song.JobGeneratingLyrics); err != nil {
		return song.Song{}, nil, err
	}
	p.log.Info("song job step", "job_id", job.ID, "step", "lyrics_style")
	ls, err := p.lyricsStyle(ctx, job, q.Answers)
	if err != nil {
		return song.Song{}, nil, err
	}

	// 2. Music.
	if err := p.setStatus(ctx, job, song.JobGeneratingMusic); err != nil {
		return song.Song{}, nil, err
	}
	p.log.Info("song job step", "job_id", job.ID, "step", "music")
	music, err := p.music(ctx, job, ls)
	if err != nil {
		return song.Song{}, nil, err
	}

	// 3. Store full audio + preview.
	if err := p.setStatus(ctx, job, song.JobProcessingAudio); err != nil {
		return song.Song{}, nil, err
	}
	p.log.Info("song job step", "job_id", job.ID, "step", "processing_audio")
	return p.finish(ctx, job, q, ls, music)
}

func (p *Pipeline) lyricsStyle(ctx context.Context, job *song.Job, a questionnaire.Answers) (aiprovider.LyricsStyleResult, error) {
	t0 := p.now()
	tpl, err := p.prompts.Active(ctx)
	if err != nil {
		return aiprovider.LyricsStyleResult{}, fmt.Errorf("load prompt template: %w", err)
	}
	hints, err := p.prompts.Hints(ctx, a)
	if err != nil {
		return aiprovider.LyricsStyleResult{}, fmt.Errorf("load prompt hints: %w", err)
	}
	prov, err := p.provs.Active(ctx, aiprovider.CategoryLyricsStyle)
	if err != nil {
		return aiprovider.LyricsStyleResult{}, noProvider(aiprovider.CategoryLyricsStyle, err)
	}
	adapter, err := p.registry.LyricsStyle(prov.AdapterType)
	if err != nil {
		return aiprovider.LyricsStyleResult{}, noProvider(aiprovider.CategoryLyricsStyle, err)
	}
	job.PromptTemplateID, job.LyricsProviderSnapshot = tpl.ID, prov.Snapshot()

	system, user := prompt.Render(tpl, a, hints)
	var res aiprovider.LyricsStyleResult
	err = p.retry(ctx, prov.Config.MaxRetries, "lyrics_style", func() error {
		var callErr error
		res, callErr = adapter.GenerateLyricsStyle(ctx, aiprovider.LyricsStyleRequest{SystemPrompt: system, UserPrompt: user}, prov.Config)
		if callErr != nil {
			return callErr
		}
		// The model sometimes drifts into another language or script mid-song; reject and retry.
		return aiprovider.CheckLyricsLanguage(string(prov.AdapterType), res.Lyrics, lyricsLang(a.Language))
	})
	if err != nil {
		return aiprovider.LyricsStyleResult{}, fmt.Errorf("generate lyrics & style: %w", err)
	}
	res = applyVoice(res, a.Voice)
	job.LyricsStyleOutput = map[string]any{
		"title": res.Title, "lyrics": res.Lyrics, "style_prompt": res.StylePrompt,
		"vocal_gender": res.VocalGender, "language": res.Language, "mood": res.Mood,
	}
	job.Timings["lyrics_style"] = p.now().Sub(t0).Milliseconds()
	return res, nil
}

func (p *Pipeline) music(ctx context.Context, job *song.Job, ls aiprovider.LyricsStyleResult) (aiprovider.MusicResult, error) {
	t0 := p.now()
	prov, err := p.provs.Active(ctx, aiprovider.CategoryMusic)
	if err != nil {
		return aiprovider.MusicResult{}, noProvider(aiprovider.CategoryMusic, err)
	}
	adapter, err := p.registry.Music(prov.AdapterType)
	if err != nil {
		return aiprovider.MusicResult{}, noProvider(aiprovider.CategoryMusic, err)
	}
	job.MusicProviderSnapshot = prov.Snapshot()

	var res aiprovider.MusicResult
	err = p.retry(ctx, prov.Config.MaxRetries, "music", func() error {
		var callErr error
		res, callErr = adapter.GenerateMusic(ctx, aiprovider.MusicRequest{Lyrics: ls.Lyrics, StylePrompt: ls.StylePrompt,
			Title: ls.Title, VocalGender: ls.VocalGender}, prov.Config)
		return callErr
	})
	if err != nil {
		return aiprovider.MusicResult{}, fmt.Errorf("generate music: %w", err)
	}
	job.Timings["music"] = p.now().Sub(t0).Milliseconds()
	return res, nil
}

func (p *Pipeline) finish(ctx context.Context, job *song.Job, q questionnaire.Questionnaire, ls aiprovider.LyricsStyleResult, m aiprovider.MusicResult) (song.Song, *song.Alternate, error) {
	t0 := p.now()
	id := p.newID()
	format := m.Format
	if format == "" {
		format = "mp3"
	}
	fullKey, previewKey := songKeys(job, id, format)

	if err := p.store.Put(ctx, fullKey, m.Audio, contentType(format)); err != nil {
		return song.Song{}, nil, fmt.Errorf("store full audio: %w", err)
	}
	duration := m.DurationSec
	if duration <= 0 {
		var err error
		if duration, err = p.audio.Duration(ctx, m.Audio); err != nil {
			return song.Song{}, nil, fmt.Errorf("measure duration: %w", err)
		}
	}
	cut, method := previewCut(ls.Lyrics, m.Words, duration, p.opts.PreviewSeconds)
	p.log.Info("preview cut", "job_id", job.ID, "method", method, "cut_sec", cut, "duration_sec", duration, "words", len(m.Words))
	preview, err := p.audio.Preview(ctx, m.Audio, cut, p.opts.FadeSeconds)
	if err != nil {
		return song.Song{}, nil, fmt.Errorf("cut preview: %w", err)
	}
	if err := p.store.Put(ctx, previewKey, preview, "audio/mpeg"); err != nil {
		return song.Song{}, nil, fmt.Errorf("store preview: %w", err)
	}

	now := p.now()
	s := song.Song{
		ID: id, JobID: job.ID, QuestionnaireID: q.ID,
		OwnerUserID: job.UserID, GuestSessionID: job.GuestSessionID,
		Title: ls.Title, Lyrics: ls.Lyrics, StylePrompt: ls.StylePrompt,
		VocalGender: ls.VocalGender, Language: ls.Language, Mood: ls.Mood, GenreCode: q.Answers.Genre,
		FullAudioKey: fullKey, PreviewAudioKey: previewKey,
		DurationSec: duration, PreviewDurationSec: min(int(math.Round(cut)), duration),
		CreatedAt: now, UpdatedAt: now,
	}
	if s.OwnerUserID == "" {
		expires := now.Add(p.opts.GuestRetention)
		s.ExpiresAt = &expires
	}
	alt := p.alternate(ctx, job, ls.Lyrics, m.Alternate)
	job.Timings["processing_audio"] = p.now().Sub(t0).Milliseconds()
	return s, alt, nil
}

// alternate stores the provider's second take (full audio + preview) under a song ID minted now,
// ready for a later "Buat Ulang". It is best effort: any failure is logged and nil returned.
func (p *Pipeline) alternate(ctx context.Context, job *song.Job, lyrics string, t *aiprovider.MusicTrack) *song.Alternate {
	if t == nil || len(t.Audio) == 0 {
		return nil
	}
	log := p.log.With("job_id", job.ID)
	id := p.newID()
	format := t.Format
	if format == "" {
		format = "mp3"
	}
	fullKey, previewKey := songKeys(job, id, format)
	if err := p.store.Put(ctx, fullKey, t.Audio, contentType(format)); err != nil {
		log.Warn("store spare take failed", "err", err)
		return nil
	}
	duration := t.DurationSec
	if duration <= 0 {
		var err error
		if duration, err = p.audio.Duration(ctx, t.Audio); err != nil {
			log.Warn("measure spare take failed", "err", err)
			return nil
		}
	}
	cut, method := previewCut(lyrics, t.Words, duration, p.opts.PreviewSeconds)
	preview, err := p.audio.Preview(ctx, t.Audio, cut, p.opts.FadeSeconds)
	if err != nil {
		log.Warn("cut spare preview failed", "err", err)
		return nil
	}
	if err := p.store.Put(ctx, previewKey, preview, "audio/mpeg"); err != nil {
		log.Warn("store spare preview failed", "err", err)
		return nil
	}
	log.Info("spare take stored", "method", method, "cut_sec", cut, "duration_sec", duration)
	root := job.RegenerateOfJobID
	if root == "" {
		root = job.ID
	}
	return &song.Alternate{
		JobID: job.ID, RootJobID: root, SongID: id, FullAudioKey: fullKey, PreviewAudioKey: previewKey,
		DurationSec: duration, PreviewDurationSec: min(int(math.Round(cut)), duration),
	}
}

func (p *Pipeline) setStatus(ctx context.Context, job *song.Job, s song.JobStatus) error {
	job.Status = s
	if err := p.jobs.UpdateJob(ctx, *job); err != nil {
		return fmt.Errorf("save job status %s: %w", s, err)
	}
	return nil
}

func (p *Pipeline) fail(ctx context.Context, job *song.Job, err error) error {
	finished := p.now()
	job.Status, job.FinishedAt = song.JobFailed, &finished
	job.ErrorCode, job.ErrorMessage = errorCode(err), err.Error()
	if saveErr := p.jobs.UpdateJob(ctx, *job); saveErr != nil {
		p.log.Error("save failed job", "job_id", job.ID, "err", saveErr)
	}
	return &FailedError{JobID: job.ID, Err: err}
}

// retry calls fn up to 1+maxRetries times, backing off exponentially on retryable provider errors.
func (p *Pipeline) retry(ctx context.Context, maxRetries int, step string, fn func() error) error {
	delay := p.opts.RetryBaseDelay
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil || attempt >= maxRetries || !aiprovider.IsRetryable(err) {
			return err
		}
		p.log.Warn("provider call failed, retrying", "step", step, "attempt", attempt+1, "delay", delay, "err", err)
		if err := p.sleep(ctx, delay); err != nil {
			return err
		}
		delay *= 2
	}
}

// lyricsLang maps the customer's language to the code CheckLyricsLanguage expects (Indonesian by default,
// matching the prompt).
func lyricsLang(l questionnaire.Language) string {
	if l == questionnaire.LanguageEN {
		return "en"
	}
	return "id"
}

// applyVoice enforces the customer's voice choice; "surprise" keeps the AI's pick.
func applyVoice(r aiprovider.LyricsStyleResult, v questionnaire.Voice) aiprovider.LyricsStyleResult {
	switch v {
	case questionnaire.VoiceMale:
		r.VocalGender = "male"
	case questionnaire.VoiceFemale:
		r.VocalGender = "female"
	}
	if r.VocalGender != "" && !strings.Contains(strings.ToLower(r.StylePrompt), r.VocalGender+" vocal") {
		r.StylePrompt = strings.TrimRight(r.StylePrompt, " ,.") + ", " + r.VocalGender + " vocal"
	}
	return r
}

type pipelineError struct {
	code string
	err  error
}

func (e *pipelineError) Error() string { return e.err.Error() }
func (e *pipelineError) Unwrap() error { return e.err }

func noProvider(c aiprovider.Category, err error) error {
	return &pipelineError{code: CodeNoProvider, err: fmt.Errorf("no usable %s provider: %w", c, err)}
}

func errorCode(err error) string {
	var pe *pipelineError
	if errors.As(err, &pe) {
		return pe.code
	}
	if k := aiprovider.KindOf(err); k != aiprovider.KindUnknown {
		return string(k)
	}
	return CodeInternal
}

func contentType(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	default:
		return "audio/mpeg"
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("retry wait: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}

// FailedError means the job ran and was saved as failed. Running it again would not help,
// so the worker does not redeliver it; the user can press "Buat Ulang" instead.
type FailedError struct {
	JobID string
	Err   error
}

func (e *FailedError) Error() string { return fmt.Sprintf("song job %s failed: %v", e.JobID, e.Err) }
func (e *FailedError) Unwrap() error { return e.Err }

// songKeys puts a song under its user's folder (users/<user_id>/songs/<song_id>/...). A job without a
// user (older guest flow) falls back to guests/<guest_session_id>/songs/<song_id>/...
func songKeys(job *song.Job, songID, format string) (full, preview string) {
	key := func(file string) string { return storage.GuestSongKey(job.GuestSessionID, songID, file) }
	if job.UserID != "" {
		key = func(file string) string { return storage.UserSongKey(job.UserID, songID, file) }
	}
	return key("full." + format), key("preview.mp3")
}
