package song

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/questionnaire"
)

// Repository is the storage the service needs (GormRepository).
type Repository interface {
	CreateJob(ctx context.Context, j Job) (Job, error)
	GetJob(ctx context.Context, id string) (Job, error)
	UpdateJob(ctx context.Context, j Job) error
	GetSong(ctx context.Context, id string) (Song, error)
	SongByJob(ctx context.Context, jobID string) (Song, error)
	CountGuestJobsSince(ctx context.Context, guestSessionID, ipHash string, since time.Time) (int64, error)
	CountRegenerates(ctx context.Context, rootJobID string) (int64, error)
	// UnlockSong marks the song bought: full version available, owned by ownerUserID, no guest expiry.
	UnlockSong(ctx context.Context, id, ownerUserID string, at time.Time) error
	// FreeAlternate returns an unused spare take of the chain, or ErrAlternateNotFound.
	FreeAlternate(ctx context.Context, rootJobID string) (Alternate, error)
	// CreateJobFromAlternate saves the job and its song and claims the spare atomically
	// (ErrAlternateTaken when it was claimed first).
	CreateJobFromAlternate(ctx context.Context, j Job, s Song, alternateID string) (Job, error)
}

// Questionnaires checks the visitor owns the questionnaire (questionnaire.Service).
type Questionnaires interface {
	Owned(ctx context.Context, v guest.Visitor, id string) (questionnaire.Questionnaire, error)
}

// Enqueuer schedules a job on the worker queue (generation.Enqueuer).
type Enqueuer interface {
	Enqueue(ctx context.Context, jobID string) error
}

// AudioStore reads stored audio (storage.Local, later S3).
type AudioStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// Limits are the guest quotas (PRD §5.6, §12).
type Limits struct {
	GuestDailyQuota int
	MaxRegenerate   int
	// SpareReveal is how long a "Buat Ulang" served from a spare take shows the loading steps
	// before the song appears. Zero uses DefaultSpareReveal.
	SpareReveal time.Duration
}

// DefaultSpareReveal is the simulated generation time of a song served from a spare take.
const DefaultSpareReveal = 90 * time.Second

// CodeEnqueueFailed is the error code of a job that never reached the worker.
const CodeEnqueueFailed = "enqueue_failed"

// Service holds the song job and song rules.
type Service struct {
	repo    Repository
	answers Questionnaires
	queue   Enqueuer
	audio   AudioStore
	limits  Limits
	now     func() time.Time
}

// NewService builds the service.
func NewService(repo Repository, answers Questionnaires, queue Enqueuer, audio AudioStore, limits Limits) *Service {
	if limits.SpareReveal <= 0 {
		limits.SpareReveal = DefaultSpareReveal
	}
	return &Service{repo: repo, answers: answers, queue: queue, audio: audio, limits: limits,
		now: func() time.Time { return time.Now().UTC() }}
}

// CreateJob starts generating a new song from the visitor's questionnaire, within the daily quota.
func (s *Service) CreateJob(ctx context.Context, v guest.Visitor, in CreateSongJobInput) (SongJobOutput, error) {
	q, err := s.answers.Owned(ctx, v, in.QuestionnaireID)
	if err != nil {
		return SongJobOutput{}, err
	}
	used, err := s.repo.CountGuestJobsSince(ctx, v.SessionID, v.IPHash, s.now().Add(-24*time.Hour))
	if err != nil {
		return SongJobOutput{}, apperror.Internal(err)
	}
	if int(used) >= s.limits.GuestDailyQuota {
		return SongJobOutput{}, apperror.TooMany("Kuota lagu gratis hari ini sudah habis. Coba lagi besok atau daftar untuk melanjutkan.").WithReason(apperror.ReasonDailyQuotaExceeded)
	}
	return s.start(ctx, Job{QuestionnaireID: q.ID, GuestSessionID: v.SessionID, UserID: q.UserID}, s.limits.MaxRegenerate)
}

// GetJob returns the visitor's job progress (polled by the loading screen).
func (s *Service) GetJob(ctx context.Context, v guest.Visitor, id string) (SongJobOutput, error) {
	j, err := s.ownedJob(ctx, v, id)
	if err != nil {
		return SongJobOutput{}, err
	}
	j = visibleJob(j, s.now())
	left, err := s.regeneratesLeft(ctx, j)
	if err != nil {
		return SongJobOutput{}, err
	}
	songID := ""
	if j.Status == JobCompleted {
		sg, err := s.repo.SongByJob(ctx, j.ID)
		if err != nil && !errors.Is(err, ErrSongNotFound) {
			return SongJobOutput{}, apperror.Internal(err)
		}
		songID = sg.ID
	}
	return toSongJobOutput(j, songID, left), nil
}

// Regenerate ("Buat Ulang") starts a new job from the same answers, at most MaxRegenerate times
// per original song. It does not count against the daily quota. When the chain has a spare take
// (the music AI's second track), that becomes the new song without another AI call; the job still
// shows the usual loading steps for SpareReveal.
func (s *Service) Regenerate(ctx context.Context, v guest.Visitor, jobID string) (SongJobOutput, error) {
	j, err := s.ownedJob(ctx, v, jobID)
	if err != nil {
		return SongJobOutput{}, err
	}
	j = visibleJob(j, s.now())
	if !j.Status.IsFinal() {
		return SongJobOutput{}, apperror.Conflict("Lagu ini masih diproses.").WithReason(apperror.ReasonJobInProgress)
	}
	left, err := s.regeneratesLeft(ctx, j)
	if err != nil {
		return SongJobOutput{}, err
	}
	if left <= 0 {
		return SongJobOutput{}, apperror.TooMany("Batas buat ulang untuk lagu ini sudah habis.").WithReason(apperror.ReasonRegenerateLimitReached)
	}
	next := Job{
		QuestionnaireID: j.QuestionnaireID, GuestSessionID: j.GuestSessionID, UserID: j.UserID,
		RegenerateOfJobID: rootOf(j),
	}
	if out, ok := s.fromSpare(ctx, next, left-1); ok {
		return out, nil
	}
	return s.start(ctx, next, left-1)
}

// fromSpare turns a free spare take of the chain into the regenerate job's song. It reports false
// when there is none or anything fails, and the caller then generates a new song as usual.
func (s *Service) fromSpare(ctx context.Context, j Job, regeneratesLeft int) (SongJobOutput, bool) {
	alt, err := s.repo.FreeAlternate(ctx, j.RegenerateOfJobID)
	if errors.Is(err, ErrAlternateNotFound) {
		return SongJobOutput{}, false
	}
	log := slog.With("root_job_id", j.RegenerateOfJobID, "alternate_id", alt.ID)
	if err != nil {
		log.WarnContext(ctx, "spare take lookup failed, generating a new song", "err", err)
		return SongJobOutput{}, false
	}
	src, err := s.repo.GetJob(ctx, alt.JobID)
	if err != nil {
		log.WarnContext(ctx, "spare take source job missing, generating a new song", "err", err)
		return SongJobOutput{}, false
	}
	srcSong, err := s.repo.SongByJob(ctx, alt.JobID)
	if err != nil {
		log.WarnContext(ctx, "spare take source song missing, generating a new song", "err", err)
		return SongJobOutput{}, false
	}

	now := s.now()
	reveal := now.Add(s.limits.SpareReveal)
	j.Status, j.StartedAt, j.FinishedAt, j.RevealAt = JobCompleted, &now, &reveal, &reveal
	j.PromptTemplateID, j.LyricsStyleOutput = src.PromptTemplateID, src.LyricsStyleOutput
	j.LyricsProviderSnapshot, j.MusicProviderSnapshot = src.LyricsProviderSnapshot, src.MusicProviderSnapshot
	j.Timings = map[string]int64{"spare_reveal": s.limits.SpareReveal.Milliseconds()}

	sg := Song{
		ID: alt.SongID, QuestionnaireID: j.QuestionnaireID, OwnerUserID: j.UserID, GuestSessionID: j.GuestSessionID,
		Title: srcSong.Title, Lyrics: srcSong.Lyrics, StylePrompt: srcSong.StylePrompt, VocalGender: srcSong.VocalGender,
		Language: srcSong.Language, Mood: srcSong.Mood, GenreCode: srcSong.GenreCode,
		FullAudioKey: alt.FullAudioKey, PreviewAudioKey: alt.PreviewAudioKey,
		DurationSec: alt.DurationSec, PreviewDurationSec: alt.PreviewDurationSec,
		CreatedAt: now, UpdatedAt: now,
	}
	if sg.OwnerUserID == "" {
		sg.ExpiresAt = srcSong.ExpiresAt
	}
	saved, err := s.repo.CreateJobFromAlternate(ctx, j, sg, alt.ID)
	if err != nil {
		if !errors.Is(err, ErrAlternateTaken) {
			log.WarnContext(ctx, "saving song from spare take failed, generating a new song", "err", err)
		}
		return SongJobOutput{}, false
	}
	log.InfoContext(ctx, "regenerate served from spare take", "job_id", saved.ID, "song_id", sg.ID, "reveal_at", reveal)
	return toSongJobOutput(visibleJob(saved, now), "", regeneratesLeft), true
}

// GetSong returns the visitor's song. Full lyrics only when unlocked.
func (s *Service) GetSong(ctx context.Context, v guest.Visitor, id string) (SongOutput, error) {
	sg, err := s.ownedSong(ctx, v, id)
	if err != nil {
		return SongOutput{}, err
	}
	return toSongOutput(sg), nil
}

// Preview returns the 60 s preview audio (a separate file; the full song is never served here).
func (s *Service) Preview(ctx context.Context, v guest.Visitor, id string) ([]byte, error) {
	sg, err := s.ownedSong(ctx, v, id)
	if err != nil {
		return nil, err
	}
	data, err := s.audio.Get(ctx, sg.PreviewAudioKey)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return data, nil
}

// FullAudio returns the full song, only once it is unlocked (bought).
func (s *Service) FullAudio(ctx context.Context, v guest.Visitor, id string) ([]byte, Song, error) {
	sg, err := s.ownedSong(ctx, v, id)
	if err != nil {
		return nil, Song{}, err
	}
	if !sg.IsUnlocked {
		return nil, Song{}, apperror.Forbidden("Lagu lengkap tersedia setelah pembayaran.").WithReason(apperror.ReasonFullSongLocked)
	}
	data, err := s.audio.Get(ctx, sg.FullAudioKey)
	if err != nil {
		return nil, Song{}, apperror.Internal(err)
	}
	return data, sg, nil
}

// Owned returns the visitor's song entity (payment checkout), or a 404 error.
func (s *Service) Owned(ctx context.Context, v guest.Visitor, id string) (Song, error) {
	return s.ownedSong(ctx, v, id)
}

// Unlock gives access to the full song after payment. Idempotent.
func (s *Service) Unlock(ctx context.Context, id, ownerUserID string, at time.Time) error {
	return s.repo.UnlockSong(ctx, id, ownerUserID, at)
}

func (s *Service) start(ctx context.Context, j Job, regeneratesLeft int) (SongJobOutput, error) {
	j.Status = JobQueued
	j, err := s.repo.CreateJob(ctx, j)
	if err != nil {
		return SongJobOutput{}, apperror.Internal(err)
	}
	if err := s.queue.Enqueue(ctx, j.ID); err != nil {
		finished := s.now()
		j.Status, j.FinishedAt, j.ErrorCode, j.ErrorMessage = JobFailed, &finished, CodeEnqueueFailed, err.Error()
		if saveErr := s.repo.UpdateJob(ctx, j); saveErr != nil {
			return SongJobOutput{}, apperror.Internal(errors.Join(err, saveErr))
		}
		return SongJobOutput{}, apperror.Unavailable("Layanan sedang sibuk. Silakan coba lagi.", err).WithReason(apperror.ReasonGenerationUnavailable)
	}
	return toSongJobOutput(j, "", regeneratesLeft), nil
}

func (s *Service) regeneratesLeft(ctx context.Context, j Job) (int, error) {
	n, err := s.repo.CountRegenerates(ctx, rootOf(j))
	if err != nil {
		return 0, apperror.Internal(err)
	}
	return max(s.limits.MaxRegenerate-int(n), 0), nil
}

func (s *Service) ownedJob(ctx context.Context, v guest.Visitor, id string) (Job, error) {
	j, err := s.repo.GetJob(ctx, id)
	if errors.Is(err, ErrJobNotFound) || (err == nil && j.GuestSessionID != v.SessionID) {
		return Job{}, apperror.NotFound("song job not found")
	}
	if err != nil {
		return Job{}, apperror.Internal(err)
	}
	return j, nil
}

func (s *Service) ownedSong(ctx context.Context, v guest.Visitor, id string) (Song, error) {
	sg, err := s.repo.GetSong(ctx, id)
	if errors.Is(err, ErrSongNotFound) || (err == nil && sg.GuestSessionID != v.SessionID) {
		return Song{}, apperror.NotFound("song not found")
	}
	if err != nil {
		return Song{}, apperror.Internal(err)
	}
	return sg, nil
}

// visibleJob is the job as customers see it. A job served from a spare take is completed at once,
// but until RevealAt it walks through the usual steps, in proportion to the time passed.
func visibleJob(j Job, now time.Time) Job {
	if j.RevealAt == nil || j.Status != JobCompleted || !now.Before(*j.RevealAt) {
		return j
	}
	total := j.RevealAt.Sub(j.CreatedAt)
	frac := 0.0
	if total > 0 {
		frac = float64(now.Sub(j.CreatedAt)) / float64(total)
	}
	switch {
	case frac < 0.04:
		j.Status = JobQueued
	case frac < 0.22:
		j.Status = JobGeneratingLyrics
	case frac < 0.88:
		j.Status = JobGeneratingMusic
	default:
		j.Status = JobProcessingAudio
	}
	j.FinishedAt = nil
	return j
}

// rootOf is the original job of a regenerate chain.
func rootOf(j Job) string {
	if j.RegenerateOfJobID != "" {
		return j.RegenerateOfJobID
	}
	return j.ID
}
