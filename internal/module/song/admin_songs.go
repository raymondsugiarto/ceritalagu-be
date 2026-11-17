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
	CreatedAt          time.Time  `json:"created_at"`
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

func toAdminSongOutput(r AdminSong) AdminSongOutput {
	base := "/api/v1/admin/songs/" + r.ID + "/audio"
	return AdminSongOutput{
		ID: r.ID, JobID: r.JobID, QuestionnaireID: r.QuestionnaireID, Title: r.Title, Lyrics: r.Lyrics,
		StylePrompt: r.StylePrompt, Genre: r.GenreCode, VocalGender: r.VocalGender, Language: r.Language,
		Mood: r.Mood, DurationSec: r.DurationSec, PreviewDurationSec: r.PreviewDurationSec,
		UserID: r.OwnerUserID, UserEmail: r.UserEmail, GuestSessionID: r.GuestSessionID,
		IsUnlocked: r.IsUnlocked, UnlockedAt: r.UnlockedAt, ExpiresAt: r.ExpiresAt,
		PreviewURL: base, AudioURL: base + "?full=1", CreatedAt: r.CreatedAt,
	}
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
func (h *AdminSongHandler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/admin/songs", h.auth)
	g.Get("/", h.list)
	g.Get("/stats", h.stats)
	g.Get("/:id/audio", h.audio)
	g.Get("/:id/answers", h.answers)
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
	c.Set(fiber.HeaderContentType, "audio/mpeg")
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	if c.Query("download") != "" {
		suffix := " (preview)"
		if full {
			suffix = ""
		}
		c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+downloadName(sg.Title)+suffix+`.mp3"`)
	}
	return c.Send(data)
}

// ---- GormRepository (read-only raw SELECTs, database-standards §5.1) ----

type adminSongRow struct {
	Song      songModel `gorm:"embedded"`
	UserEmail string    `gorm:"column:user_email"`
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
	q := "SELECT s.*, COALESCE(u.email, '') AS user_email" + from + cond + " ORDER BY s.created_at DESC LIMIT ? OFFSET ?"
	if err := r.db.WithContext(ctx).Raw(q, append(args, p.Limit, p.Offset)...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list admin songs: %w", err)
	}
	out := make([]AdminSong, 0, len(rows))
	for _, m := range rows {
		out = append(out, AdminSong{Song: m.Song.toSong(), UserEmail: m.UserEmail})
	}
	return out, total, nil
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
