package song

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/pagination"
	"github.com/yourorg/music-api/internal/response"
)

// Admin song library: every successfully generated song (table song), with its customer,
// lyrics and both audio files.

// SongAccess filters the admin song list by purchase state.
type SongAccess string

// Song access filters.
const (
	AccessAll      SongAccess = "all"
	AccessUnlocked SongAccess = "unlocked"
	AccessLocked   SongAccess = "locked"
)

// AdminSong is one row of the admin song list (read model, raw SELECT).
type AdminSong struct {
	Song
	UserEmail string
	// Alternate is the spare take made by the same music request (Suno returns two); nil when none.
	Alternate *Alternate
}

// AdminSongParams is the repository query of the admin song list.
type AdminSongParams struct {
	Unlocked *bool  // nil = all
	Search   string // song id, title or user email
	Offset   int
	Limit    int
}

// ListSongsInput is the query of GET /admin/songs: ?access=all|unlocked|locked&search=&page=&page_size=
type ListSongsInput struct {
	pagination.Input
	Access SongAccess `query:"access" validate:"omitempty,oneof=all unlocked locked"`
	Search string     `query:"search" validate:"omitempty,max=100"`
}

// AdminSongOutput is a song as shown in the admin library. Lyrics are always included (admin).
type AdminSongOutput struct {
	ID                 string     `json:"id"`
	JobID              string     `json:"job_id"`
	QuestionnaireID    string     `json:"questionnaire_id"`
	Title              string     `json:"title"`
	Lyrics             string     `json:"lyrics"`
	StylePrompt        string     `json:"style_prompt"`
	Genre              string     `json:"genre"`
	VocalGender        string     `json:"vocal_gender"`
	Language           string     `json:"language"`
	Mood               string     `json:"mood"`
	DurationSec        int        `json:"duration_sec"`
	PreviewDurationSec int        `json:"preview_duration_sec"`
	UserID             string     `json:"user_id,omitempty"`
	UserEmail          string     `json:"user_email,omitempty"`
	GuestSessionID     string     `json:"guest_session_id,omitempty"`
	IsUnlocked         bool       `json:"is_unlocked"`
	UnlockedAt         *time.Time `json:"unlocked_at"`
	ExpiresAt          *time.Time `json:"expires_at"`
	PreviewURL         string     `json:"preview_url"`
	AudioURL           string     `json:"audio_url"`
	// Alternate is the second take of the same request; null when the provider made none.
	Alternate *AdminAlternateOutput `json:"alternate"`
	CreatedAt time.Time             `json:"created_at"`
}

// AdminAlternateOutput is the spare take shown next to a song in the admin library.
type AdminAlternateOutput struct {
	ID                 string `json:"id"`
	DurationSec        int    `json:"duration_sec"`
	PreviewDurationSec int    `json:"preview_duration_sec"`
	// UsedByJobID is the "Buat Ulang" job that turned the take into a song; empty while still spare.
	UsedByJobID string `json:"used_by_job_id,omitempty"`
	// UsedSongID is the song it became (song_alternate.song_id), set only when used.
	UsedSongID string `json:"used_song_id,omitempty"`
	PreviewURL string `json:"preview_url"`
	AudioURL   string `json:"audio_url"`
}

// SongStatsOutput counts songs for the library's summary cards.
type SongStatsOutput struct {
	Total    int64 `json:"total"`
	Unlocked int64 `json:"unlocked"`
	Guest    int64 `json:"guest"`
}

// AdminSongRepository is the read side the admin library needs (GormRepository).
type AdminSongRepository interface {
	ListAdminSongs(ctx context.Context, p AdminSongParams) ([]AdminSong, int64, error)
	CountSongs(ctx context.Context) (SongStatsOutput, error)
	GetSong(ctx context.Context, id string) (Song, error)
	SongAnswers(ctx context.Context, id string) (AdminSongAnswersOutput, error)
	// SongAlternate returns the spare take made by song id's music request, or ErrAlternateNotFound.
	SongAlternate(ctx context.Context, id string) (Alternate, error)
}

// AdminSongService lists songs and serves their audio to admins.
type AdminSongService struct {
	repo  AdminSongRepository
	audio AudioStore
}

// NewAdminSongService builds the service.
func NewAdminSongService(repo AdminSongRepository, audio AudioStore) *AdminSongService {
	return &AdminSongService{repo: repo, audio: audio}
}

// ListSongs returns one page of songs, newest first.
func (s *AdminSongService) ListSongs(ctx context.Context, in ListSongsInput) (pagination.ListOutput[AdminSongOutput], error) {
	in.Normalize()
	var unlocked *bool
	switch in.Access {
	case AccessUnlocked:
		t := true
		unlocked = &t
	case AccessLocked:
		f := false
		unlocked = &f
	}
	rows, total, err := s.repo.ListAdminSongs(ctx, AdminSongParams{
		Unlocked: unlocked, Search: strings.TrimSpace(in.Search), Offset: in.Offset(), Limit: in.PageSize,
	})
	if err != nil {
		return pagination.ListOutput[AdminSongOutput]{}, apperror.Internal(err)
	}
	out := make([]AdminSongOutput, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAdminSongOutput(r))
	}
	return pagination.NewList(out, in.Input, total), nil
}

// Stats counts all, unlocked and guest-owned songs.
func (s *AdminSongService) Stats(ctx context.Context) (SongStatsOutput, error) {
	out, err := s.repo.CountSongs(ctx)
	if err != nil {
		return SongStatsOutput{}, apperror.Internal(err)
	}
	return out, nil
}

// Audio returns the preview, or the full song when full is true, of any song. Admins skip the
// purchase check.
func (s *AdminSongService) Audio(ctx context.Context, id string, full bool) ([]byte, Song, error) {
	sg, err := s.repo.GetSong(ctx, id)
	if errors.Is(err, ErrSongNotFound) {
		return nil, Song{}, apperror.NotFound("Lagu tidak ditemukan.")
	}
	if err != nil {
		return nil, Song{}, apperror.Internal(err)
	}
	key := sg.PreviewAudioKey
	if full {
		key = sg.FullAudioKey
	}
	if key == "" {
		return nil, Song{}, apperror.NotFound("File audio tidak tersedia.")
	}
	data, err := s.audio.Get(ctx, key)
	if err != nil {
		return nil, Song{}, apperror.Internal(err)
	}
	return data, sg, nil
}

// AlternateAudio returns the preview, or the full audio when full is true, of the spare take made
// by song id's music request.
func (s *AdminSongService) AlternateAudio(ctx context.Context, id string, full bool) ([]byte, Song, error) {
	sg, err := s.repo.GetSong(ctx, id)
	if errors.Is(err, ErrSongNotFound) {
		return nil, Song{}, apperror.NotFound("Lagu tidak ditemukan.")
	}
	if err != nil {
		return nil, Song{}, apperror.Internal(err)
	}
	alt, err := s.repo.SongAlternate(ctx, id)
	if errors.Is(err, ErrAlternateNotFound) {
		return nil, Song{}, apperror.NotFound("Lagu ini tidak punya take ke-2.")
	}
	if err != nil {
		return nil, Song{}, apperror.Internal(err)
	}
	key := alt.PreviewAudioKey
	if full {
		key = alt.FullAudioKey
	}
	if key == "" {
		return nil, Song{}, apperror.NotFound("File audio tidak tersedia.")
	}
	data, err := s.audio.Get(ctx, key)
	if err != nil {
		return nil, Song{}, apperror.Internal(err)
	}
	return data, sg, nil
}

func toAdminSongOutput(r AdminSong) AdminSongOutput {
	base := "/api/v1/admin/songs/" + r.ID + "/audio"
	return AdminSongOutput{
		ID: r.ID, JobID: r.JobID, QuestionnaireID: r.QuestionnaireID, Title: r.Title, Lyrics: r.Lyrics,
		StylePrompt: r.StylePrompt, Genre: r.GenreCode, VocalGender: r.VocalGender, Language: r.Language,
		Mood: r.Mood, DurationSec: r.DurationSec, PreviewDurationSec: r.PreviewDurationSec,
		UserID: r.OwnerUserID, UserEmail: r.UserEmail, GuestSessionID: r.GuestSessionID,
		IsUnlocked: r.IsUnlocked, UnlockedAt: r.UnlockedAt, ExpiresAt: r.ExpiresAt,
		PreviewURL: base, AudioURL: base + "?full=1", Alternate: toAdminAlternateOutput(r.ID, r.Alternate),
		CreatedAt: r.CreatedAt,
	}
}

func toAdminAlternateOutput(songID string, a *Alternate) *AdminAlternateOutput {
	if a == nil {
		return nil
	}
	base := "/api/v1/admin/songs/" + songID + "/alternate/audio"
	out := &AdminAlternateOutput{
		ID: a.ID, DurationSec: a.DurationSec, PreviewDurationSec: a.PreviewDurationSec,
		UsedByJobID: a.UsedByJobID, PreviewURL: base, AudioURL: base + "?full=1",
	}
	if a.UsedByJobID != "" {
		out.UsedSongID = a.SongID
	}
	return out
}

// AdminSongHandler serves /admin/songs.
type AdminSongHandler struct {
	svc  *AdminSongService
	auth fiber.Handler
}

// NewAdminSongHandler builds the handler. auth guards every route.
func NewAdminSongHandler(svc *AdminSongService, auth fiber.Handler) *AdminSongHandler {
	return &AdminSongHandler{svc: svc, auth: auth}
}

// RegisterRoutes mounts the admin song library under /api/v1. It implements app.Module.
//
//	GET /api/v1/admin/songs             songs (?access=all|unlocked|locked&search=&page=&page_size=)
//	GET /api/v1/admin/songs/stats       song counts
//	GET /api/v1/admin/songs/:id/audio   preview audio; ?full=1 full song; &download=1 as attachment
//	GET /api/v1/admin/songs/:id/answers questionnaire answers (steps 1-8) behind the song
//	GET /api/v1/admin/songs/:id/alternate/audio  second take of the same request (same query as /audio)
func (h *AdminSongHandler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/admin/songs", h.auth)
	g.Get("/", h.list)
	g.Get("/stats", h.stats)
	g.Get("/:id/audio", h.audio)
	g.Get("/:id/answers", h.answers)
	g.Get("/:id/alternate/audio", h.alternateAudio)
}

func (h *AdminSongHandler) list(c fiber.Ctx) error {
	var in ListSongsInput
	if err := c.Bind().Query(&in); err != nil {
		return err
	}
	out, err := h.svc.ListSongs(c.Context(), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *AdminSongHandler) stats(c fiber.Ctx) error {
	out, err := h.svc.Stats(c.Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *AdminSongHandler) audio(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	full := c.Query("full") != ""
	data, sg, err := h.svc.Audio(c.Context(), id, full)
	if err != nil {
		return err
	}
	return sendAdminAudio(c, data, sg.Title, full)
}

func (h *AdminSongHandler) alternateAudio(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	full := c.Query("full") != ""
	data, sg, err := h.svc.AlternateAudio(c.Context(), id, full)
	if err != nil {
		return err
	}
	return sendAdminAudio(c, data, sg.Title+" (take 2)", full)
}

// sendAdminAudio writes an MP3, as an attachment named after title when ?download is set.
func sendAdminAudio(c fiber.Ctx, data []byte, title string, full bool) error {
	c.Set(fiber.HeaderContentType, "audio/mpeg")
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	if c.Query("download") != "" {
		suffix := " (preview)"
		if full {
			suffix = ""
		}
		c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+downloadName(title)+suffix+`.mp3"`)
	}
	return c.Send(data)
}

// ---- GormRepository (read-only raw SELECTs, database-standards §5.1) ----

type adminSongRow struct {
	Song      songModel `gorm:"embedded"`
	UserEmail string    `gorm:"column:user_email"`
	AltID     string    `gorm:"column:alt_id"`
	AltSongID string    `gorm:"column:alt_song_id"`
	AltDur    int       `gorm:"column:alt_duration_sec"`
	AltPrev   int       `gorm:"column:alt_preview_duration_sec"`
	AltUsedBy string    `gorm:"column:alt_used_by_job_id"`
}

// ListAdminSongs returns one page of live songs joined with their owner, newest first.
func (r *GormRepository) ListAdminSongs(ctx context.Context, p AdminSongParams) ([]AdminSong, int64, error) {
	where := []string{"s.deleted_at IS NULL"}
	var args []any
	if p.Unlocked != nil {
		where = append(where, "s.is_unlocked = ?")
		args = append(args, *p.Unlocked)
	}
	if p.Search != "" {
		like := "%" + strings.ToLower(p.Search) + "%"
		where = append(where, "(s.id = ? OR LOWER(COALESCE(s.title, '')) LIKE ? OR LOWER(COALESCE(u.email, '')) LIKE ?)")
		args = append(args, p.Search, like, like)
	}
	const from = `
FROM song s
LEFT JOIN "user" u ON u.id = s.owner_user_id
WHERE `
	cond := strings.Join(where, " AND ")

	var total int64
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*)"+from+cond, args...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count admin songs: %w", err)
	}
	var rows []adminSongRow
	// The spare take made by this song's own request (song_job_id), not the chain's other spares.
	q := `SELECT s.*, COALESCE(u.email, '') AS user_email,
       COALESCE(a.id, '') AS alt_id, COALESCE(a.song_id, '') AS alt_song_id,
       COALESCE(a.duration_sec, 0) AS alt_duration_sec,
       COALESCE(a.preview_duration_sec, 0) AS alt_preview_duration_sec,
       COALESCE(a.used_by_job_id, '') AS alt_used_by_job_id` + strings.Replace(from, "WHERE", `LEFT JOIN LATERAL (
    SELECT id, song_id, duration_sec, preview_duration_sec, used_by_job_id FROM song_alternate
    WHERE song_job_id = s.song_job_id AND deleted_at IS NULL ORDER BY created_at ASC LIMIT 1
) a ON TRUE
WHERE`, 1) + cond + " ORDER BY s.created_at DESC LIMIT ? OFFSET ?"
	if err := r.db.WithContext(ctx).Raw(q, append(args, p.Limit, p.Offset)...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list admin songs: %w", err)
	}
	out := make([]AdminSong, 0, len(rows))
	for _, m := range rows {
		row := AdminSong{Song: m.Song.toSong(), UserEmail: m.UserEmail}
		if m.AltID != "" {
			row.Alternate = &Alternate{
				ID: m.AltID, JobID: row.JobID, SongID: m.AltSongID, DurationSec: m.AltDur,
				PreviewDurationSec: m.AltPrev, UsedByJobID: m.AltUsedBy,
			}
		}
		out = append(out, row)
	}
	return out, total, nil
}

// SongAlternate returns the oldest spare take made by song id's music request.
func (r *GormRepository) SongAlternate(ctx context.Context, id string) (Alternate, error) {
	var rows []alternateModel
	err := r.db.WithContext(ctx).Raw(`SELECT a.* FROM song_alternate a
JOIN song s ON s.song_job_id = a.song_job_id
WHERE s.id = ? AND s.deleted_at IS NULL AND a.deleted_at IS NULL
ORDER BY a.created_at ASC LIMIT 1`, id).Scan(&rows).Error
	if err != nil {
		return Alternate{}, fmt.Errorf("select song_alternate of song %s: %w", id, err)
	}
	if len(rows) == 0 {
		return Alternate{}, ErrAlternateNotFound
	}
	return rows[0].toAlternate(), nil
}

// CountSongs counts live songs.
func (r *GormRepository) CountSongs(ctx context.Context) (SongStatsOutput, error) {
	var out SongStatsOutput
	err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) AS total,
       COUNT(*) FILTER (WHERE is_unlocked) AS unlocked,
       COUNT(*) FILTER (WHERE COALESCE(owner_user_id, '') = '') AS guest
FROM song WHERE deleted_at IS NULL`).Scan(&out).Error
	if err != nil {
		return SongStatsOutput{}, fmt.Errorf("count songs: %w", err)
	}
	return out, nil
}

var _ AdminSongRepository = (*GormRepository)(nil)
