package song

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository-level sentinel errors.
var (
	ErrJobNotFound  = errors.New("song job not found")
	ErrSongNotFound = errors.New("song not found")
)

// jobModel maps table song_job (docs/database-standards.md: dumb store, rules live here).
type jobModel struct {
	ID                     string           `gorm:"column:id;primaryKey"`
	QuestionnaireID        string           `gorm:"column:questionnaire_id"`
	GuestSessionID         string           `gorm:"column:guest_session_id"`
	UserID                 string           `gorm:"column:user_id"`
	RegenerateOfJobID      string           `gorm:"column:regenerate_of_job_id"`
	Status                 string           `gorm:"column:status"`
	Attempt                int              `gorm:"column:attempt"`
	LyricsProviderSnapshot map[string]any   `gorm:"column:lyrics_provider_snapshot;serializer:json"`
	MusicProviderSnapshot  map[string]any   `gorm:"column:music_provider_snapshot;serializer:json"`
	PromptTemplateID       string           `gorm:"column:prompt_template_id"`
	LyricsStyleOutput      map[string]any   `gorm:"column:lyrics_style_output;serializer:json"`
	ErrorCode              string           `gorm:"column:error_code"`
	ErrorMessage           string           `gorm:"column:error_message"`
	Timings                map[string]int64 `gorm:"column:timings;serializer:json"`
	StartedAt              *time.Time       `gorm:"column:started_at"`
	FinishedAt             *time.Time       `gorm:"column:finished_at"`
	CreatedAt              time.Time        `gorm:"column:created_at"`
	UpdatedAt              time.Time        `gorm:"column:updated_at"`
	DeletedAt              gorm.DeletedAt   `gorm:"column:deleted_at"`
}

func (jobModel) TableName() string { return "song_job" }

// songModel maps table song.
type songModel struct {
	ID                 string         `gorm:"column:id;primaryKey"`
	SongJobID          string         `gorm:"column:song_job_id"`
	QuestionnaireID    string         `gorm:"column:questionnaire_id"`
	OwnerUserID        string         `gorm:"column:owner_user_id"`
	GuestSessionID     string         `gorm:"column:guest_session_id"`
	Title              string         `gorm:"column:title"`
	Lyrics             string         `gorm:"column:lyrics"`
	StylePrompt        string         `gorm:"column:style_prompt"`
	VocalGender        string         `gorm:"column:vocal_gender"`
	Language           string         `gorm:"column:language"`
	Mood               string         `gorm:"column:mood"`
	GenreCode          string         `gorm:"column:genre_code"`
	FullAudioKey       string         `gorm:"column:full_audio_key"`
	PreviewAudioKey    string         `gorm:"column:preview_audio_key"`
	DurationSec        int            `gorm:"column:duration_sec"`
	PreviewDurationSec int            `gorm:"column:preview_duration_sec"`
	IsUnlocked         bool           `gorm:"column:is_unlocked"`
	UnlockedAt         *time.Time     `gorm:"column:unlocked_at"`
	ExpiresAt          *time.Time     `gorm:"column:expires_at"`
	CreatedAt          time.Time      `gorm:"column:created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (songModel) TableName() string { return "song" }

// GormRepository stores jobs and songs in PostgreSQL. Soft-deleted rows are skipped by GORM.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// CreateJob inserts a job, minting its ID if empty, and returns it.
func (r *GormRepository) CreateJob(ctx context.Context, j Job) (Job, error) {
	if j.ID == "" {
		j.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	j.CreatedAt, j.UpdatedAt = now, now
	if err := r.db.WithContext(ctx).Create(toJobModel(j)).Error; err != nil {
		return Job{}, fmt.Errorf("insert song_job: %w", err)
	}
	return j, nil
}

// GetJob returns ErrJobNotFound if the job does not exist.
func (r *GormRepository) GetJob(ctx context.Context, id string) (Job, error) {
	var m jobModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("select song_job %s: %w", id, err)
	}
	return m.toJob(), nil
}

// UpdateJob saves every field of j.
func (r *GormRepository) UpdateJob(ctx context.Context, j Job) error {
	j.UpdatedAt = time.Now().UTC()
	res := r.db.WithContext(ctx).Save(toJobModel(j))
	if res.Error != nil {
		return fmt.Errorf("update song_job %s: %w", j.ID, res.Error)
	}
	return nil
}

// CreateSong inserts a song, minting its ID if empty.
func (r *GormRepository) CreateSong(ctx context.Context, s Song) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if err := r.db.WithContext(ctx).Create(toSongModel(s)).Error; err != nil {
		return fmt.Errorf("insert song: %w", err)
	}
	return nil
}

// GetSong returns ErrSongNotFound if the song does not exist.
func (r *GormRepository) GetSong(ctx context.Context, id string) (Song, error) {
	var m songModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Song{}, ErrSongNotFound
	}
	if err != nil {
		return Song{}, fmt.Errorf("select song %s: %w", id, err)
	}
	return m.toSong(), nil
}

func toJobModel(j Job) *jobModel {
	return &jobModel{
		ID: j.ID, QuestionnaireID: j.QuestionnaireID, GuestSessionID: j.GuestSessionID, UserID: j.UserID,
		RegenerateOfJobID: j.RegenerateOfJobID, Status: string(j.Status), Attempt: j.Attempt,
		LyricsProviderSnapshot: j.LyricsProviderSnapshot, MusicProviderSnapshot: j.MusicProviderSnapshot,
		PromptTemplateID: j.PromptTemplateID, LyricsStyleOutput: j.LyricsStyleOutput,
		ErrorCode: j.ErrorCode, ErrorMessage: j.ErrorMessage, Timings: j.Timings,
		StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
	}
}

func (m jobModel) toJob() Job {
	return Job{
		ID: m.ID, QuestionnaireID: m.QuestionnaireID, GuestSessionID: m.GuestSessionID, UserID: m.UserID,
		RegenerateOfJobID: m.RegenerateOfJobID, Status: JobStatus(m.Status), Attempt: m.Attempt,
		LyricsProviderSnapshot: m.LyricsProviderSnapshot, MusicProviderSnapshot: m.MusicProviderSnapshot,
		PromptTemplateID: m.PromptTemplateID, LyricsStyleOutput: m.LyricsStyleOutput,
		ErrorCode: m.ErrorCode, ErrorMessage: m.ErrorMessage, Timings: m.Timings,
		StartedAt: m.StartedAt, FinishedAt: m.FinishedAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func toSongModel(s Song) *songModel {
	return &songModel{
		ID: s.ID, SongJobID: s.JobID, QuestionnaireID: s.QuestionnaireID, OwnerUserID: s.OwnerUserID,
		GuestSessionID: s.GuestSessionID, Title: s.Title, Lyrics: s.Lyrics, StylePrompt: s.StylePrompt,
		VocalGender: s.VocalGender, Language: s.Language, Mood: s.Mood, GenreCode: s.GenreCode,
		FullAudioKey: s.FullAudioKey, PreviewAudioKey: s.PreviewAudioKey, DurationSec: s.DurationSec,
		PreviewDurationSec: s.PreviewDurationSec, IsUnlocked: s.IsUnlocked, UnlockedAt: s.UnlockedAt,
		ExpiresAt: s.ExpiresAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func (m songModel) toSong() Song {
	return Song{
		ID: m.ID, JobID: m.SongJobID, QuestionnaireID: m.QuestionnaireID, OwnerUserID: m.OwnerUserID,
		GuestSessionID: m.GuestSessionID, Title: m.Title, Lyrics: m.Lyrics, StylePrompt: m.StylePrompt,
		VocalGender: m.VocalGender, Language: m.Language, Mood: m.Mood, GenreCode: m.GenreCode,
		FullAudioKey: m.FullAudioKey, PreviewAudioKey: m.PreviewAudioKey, DurationSec: m.DurationSec,
		PreviewDurationSec: m.PreviewDurationSec, IsUnlocked: m.IsUnlocked, UnlockedAt: m.UnlockedAt,
		ExpiresAt: m.ExpiresAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// UnlockSong sets is_unlocked (unlocked_at only the first time), the owner if it was empty, and clears
// the guest expiry. It returns ErrSongNotFound for an unknown song.
func (r *GormRepository) UnlockSong(ctx context.Context, id, ownerUserID string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&songModel{}).Where("id = ?", id).Updates(map[string]any{
		"is_unlocked":   true,
		"unlocked_at":   gorm.Expr("COALESCE(unlocked_at, ?)", at),
		"owner_user_id": gorm.Expr("CASE WHEN COALESCE(owner_user_id, '') = '' THEN ? ELSE owner_user_id END", ownerUserID),
		"expires_at":    nil,
		"updated_at":    at,
	})
	if res.Error != nil {
		return fmt.Errorf("unlock song %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrSongNotFound
	}
	return nil
}

// SongByJob returns the song produced by jobID, or ErrSongNotFound.
func (r *GormRepository) SongByJob(ctx context.Context, jobID string) (Song, error) {
	var m songModel
	err := r.db.WithContext(ctx).Where("song_job_id = ?", jobID).Order("created_at DESC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Song{}, ErrSongNotFound
	}
	if err != nil {
		return Song{}, fmt.Errorf("select song of job %s: %w", jobID, err)
	}
	return m.toSong(), nil
}

// CountGuestJobsSince counts new songs (not regenerations) started since `since` by this guest
// session or by any session from the same IP hash. Read-only raw SELECT (database-standards §5.1).
func (r *GormRepository) CountGuestJobsSince(ctx context.Context, guestSessionID, ipHash string, since time.Time) (int64, error) {
	const q = `
SELECT COUNT(*) FROM song_job j
WHERE j.deleted_at IS NULL
  AND j.created_at >= ?
  AND COALESCE(j.regenerate_of_job_id, '') = ''
  AND (j.guest_session_id = ?
       OR j.guest_session_id IN (SELECT g.id FROM guest_session g WHERE g.ip_hash = ? AND g.deleted_at IS NULL))`
	var n int64
	if err := r.db.WithContext(ctx).Raw(q, since, guestSessionID, ipHash).Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("count guest jobs: %w", err)
	}
	return n, nil
}

// CountRegenerates counts jobs that regenerate rootJobID.
func (r *GormRepository) CountRegenerates(ctx context.Context, rootJobID string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&jobModel{}).Where("regenerate_of_job_id = ?", rootJobID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count regenerates of %s: %w", rootJobID, err)
	}
	return n, nil
}
