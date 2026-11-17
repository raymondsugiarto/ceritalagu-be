package song

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/pagination"
	"github.com/yourorg/music-api/internal/response"
)

// Admin job monitor (PRD §6.4): every song_job with its customer, providers, step timings and error.

// JobFilter groups job statuses for the admin list.
type JobFilter string

// Job filters. "active" is every job a worker still has to finish.
const (
	FilterAll       JobFilter = "all"
	FilterActive    JobFilter = "active"
	FilterCompleted JobFilter = "completed"
	FilterFailed    JobFilter = "failed"
)

// activeStatuses are the non-final statuses.
var activeStatuses = []string{string(JobQueued), string(JobGeneratingLyrics), string(JobGeneratingMusic), string(JobProcessingAudio)}

// AdminJob is one row of the admin job list (read model, raw SELECT).
type AdminJob struct {
	Job
	UserEmail string
	SongID    string
	SongTitle string
}

// AdminJobParams is the repository query of the admin list.
type AdminJobParams struct {
	Statuses []string // empty = all
	Search   string   // job id, user email or song title
	Offset   int
	Limit    int
}

// ListJobsInput is the query of GET /admin/song-jobs: ?filter=active|completed|failed|all&search=&page=&page_size=
type ListJobsInput struct {
	pagination.Input
	Filter JobFilter `query:"filter" validate:"omitempty,oneof=all active completed failed"`
	Search string    `query:"search" validate:"omitempty,max=100"`
}

// AdminJobOutput is a job as shown in the admin monitor.
type AdminJobOutput struct {
	ID                string           `json:"id"`
	Status            JobStatus        `json:"status"`
	StatusMessage     string           `json:"status_message"`
	Attempt           int              `json:"attempt"`
	QuestionnaireID   string           `json:"questionnaire_id"`
	UserID            string           `json:"user_id,omitempty"`
	UserEmail         string           `json:"user_email,omitempty"`
	GuestSessionID    string           `json:"guest_session_id,omitempty"`
	RegenerateOfJobID string           `json:"regenerate_of_job_id,omitempty"`
	LyricsProvider    string           `json:"lyrics_provider,omitempty"`
	MusicProvider     string           `json:"music_provider,omitempty"`
	ErrorCode         string           `json:"error_code,omitempty"`
	ErrorMessage      string           `json:"error_message,omitempty"` // internal detail, admin only
	Timings           map[string]int64 `json:"timings"`
	SongID            string           `json:"song_id,omitempty"`
	SongTitle         string           `json:"song_title,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	StartedAt         *time.Time       `json:"started_at"`
	FinishedAt        *time.Time       `json:"finished_at"`
	// ElapsedMs is finished-started, or now-started while the job runs (now-created while queued).
	ElapsedMs int64 `json:"elapsed_ms"`
}

// JobStatsOutput counts jobs per status (all time) for the monitor's summary cards.
type JobStatsOutput struct {
	Counts map[JobStatus]int64 `json:"counts"`
	Active int64               `json:"active"`
}

// AdminRepository is the read side the admin monitor needs (GormRepository).
type AdminRepository interface {
	ListAdminJobs(ctx context.Context, p AdminJobParams) ([]AdminJob, int64, error)
	CountJobsByStatus(ctx context.Context) (map[string]int64, error)
}

// AdminService lists jobs for the admin.
type AdminService struct {
	repo AdminRepository
	now  func() time.Time
}

// NewAdminService builds the service.
func NewAdminService(repo AdminRepository) *AdminService {
	return &AdminService{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// ListJobs returns one page of jobs, newest first.
func (s *AdminService) ListJobs(ctx context.Context, in ListJobsInput) (pagination.ListOutput[AdminJobOutput], error) {
	in.Normalize()
	var statuses []string
	switch in.Filter {
	case FilterActive:
		statuses = activeStatuses
	case FilterCompleted:
		statuses = []string{string(JobCompleted)}
	case FilterFailed:
		statuses = []string{string(JobFailed)}
	}
	rows, total, err := s.repo.ListAdminJobs(ctx, AdminJobParams{
		Statuses: statuses, Search: strings.TrimSpace(in.Search), Offset: in.Offset(), Limit: in.PageSize,
	})
	if err != nil {
		return pagination.ListOutput[AdminJobOutput]{}, apperror.Internal(err)
	}
	now := s.now()
	out := make([]AdminJobOutput, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAdminJobOutput(r, now))
	}
	return pagination.NewList(out, in.Input, total), nil
}

// Stats counts jobs per status.
func (s *AdminService) Stats(ctx context.Context) (JobStatsOutput, error) {
	m, err := s.repo.CountJobsByStatus(ctx)
	if err != nil {
		return JobStatsOutput{}, apperror.Internal(err)
	}
	out := JobStatsOutput{Counts: map[JobStatus]int64{}}
	for st, n := range m {
		out.Counts[JobStatus(st)] = n
		if !JobStatus(st).IsFinal() {
			out.Active += n
		}
	}
	return out, nil
}

func toAdminJobOutput(r AdminJob, now time.Time) AdminJobOutput {
	timings := r.Timings
	if timings == nil {
		timings = map[string]int64{}
	}
	out := AdminJobOutput{
		ID: r.ID, Status: r.Status, StatusMessage: statusMessages[r.Status], Attempt: r.Attempt,
		QuestionnaireID: r.QuestionnaireID, UserID: r.UserID, UserEmail: r.UserEmail, GuestSessionID: r.GuestSessionID,
		RegenerateOfJobID: r.RegenerateOfJobID, LyricsProvider: snapshotName(r.LyricsProviderSnapshot),
		MusicProvider: snapshotName(r.MusicProviderSnapshot), ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage,
		Timings: timings, SongID: r.SongID, SongTitle: r.SongTitle,
		CreatedAt: r.CreatedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
	from := r.CreatedAt
	if r.StartedAt != nil {
		from = *r.StartedAt
	}
	to := now
	if r.FinishedAt != nil {
		to = *r.FinishedAt
	}
	if to.After(from) {
		out.ElapsedMs = to.Sub(from).Milliseconds()
	}
	return out
}

// snapshotName is "name (adapter_type)" from a provider snapshot.
func snapshotName(s map[string]any) string {
	name, _ := s["name"].(string)
	adapter, _ := s["adapter_type"].(string)
	switch {
	case name != "" && adapter != "":
		return fmt.Sprintf("%s (%s)", name, adapter)
	case name != "":
		return name
	default:
		return adapter
	}
}

// AdminHandler serves /admin/song-jobs.
type AdminHandler struct {
	svc  *AdminService
	auth fiber.Handler
}

// NewAdminHandler builds the handler. auth guards every route.
func NewAdminHandler(svc *AdminService, auth fiber.Handler) *AdminHandler {
	return &AdminHandler{svc: svc, auth: auth}
}

// RegisterRoutes mounts the admin monitor under /api/v1. It implements app.Module.
//
//	GET /api/v1/admin/song-jobs         jobs (?filter=active|completed|failed|all&search=&page=&page_size=)
//	GET /api/v1/admin/song-jobs/stats   job counts per status
func (h *AdminHandler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/admin/song-jobs", h.auth)
	g.Get("/", h.list)
	g.Get("/stats", h.stats)
}

func (h *AdminHandler) list(c fiber.Ctx) error {
	var in ListJobsInput
	if err := c.Bind().Query(&in); err != nil {
		return err
	}
	out, err := h.svc.ListJobs(c.Context(), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *AdminHandler) stats(c fiber.Ctx) error {
	out, err := h.svc.Stats(c.Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// ---- GormRepository (read-only raw SELECTs, database-standards §5.1) ----

type adminJobRow struct {
	Job       jobModel `gorm:"embedded"` // named: an unexported embedded type cannot be set by GORM
	UserEmail string   `gorm:"column:user_email"`
	SongID    string   `gorm:"column:song_id"`
	SongTitle string   `gorm:"column:song_title"`
}

// ListAdminJobs returns one page of jobs joined with their user and song, newest first.
func (r *GormRepository) ListAdminJobs(ctx context.Context, p AdminJobParams) ([]AdminJob, int64, error) {
	where := []string{"j.deleted_at IS NULL"}
	var args []any
	if len(p.Statuses) > 0 {
		where = append(where, "j.status IN ?")
		args = append(args, p.Statuses)
	}
	if p.Search != "" {
		like := "%" + strings.ToLower(p.Search) + "%"
		where = append(where, "(j.id = ? OR LOWER(COALESCE(u.email, '')) LIKE ? OR LOWER(COALESCE(s.title, '')) LIKE ?)")
		args = append(args, p.Search, like, like)
	}
	const from = `
FROM song_job j
LEFT JOIN "user" u ON u.id = j.user_id
LEFT JOIN LATERAL (SELECT s.id, s.title FROM song s WHERE s.song_job_id = j.id AND s.deleted_at IS NULL
                   ORDER BY s.created_at DESC LIMIT 1) s ON TRUE
WHERE `
	cond := strings.Join(where, " AND ")

	var total int64
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*)"+from+cond, args...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count admin jobs: %w", err)
	}
	var rows []adminJobRow
	q := "SELECT j.*, COALESCE(u.email, '') AS user_email, COALESCE(s.id, '') AS song_id, COALESCE(s.title, '') AS song_title" +
		from + cond + " ORDER BY j.created_at DESC LIMIT ? OFFSET ?"
	if err := r.db.WithContext(ctx).Raw(q, append(args, p.Limit, p.Offset)...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list admin jobs: %w", err)
	}
	out := make([]AdminJob, 0, len(rows))
	for _, m := range rows {
		out = append(out, AdminJob{Job: m.Job.toJob(), UserEmail: m.UserEmail, SongID: m.SongID, SongTitle: m.SongTitle})
	}
	return out, total, nil
}

// CountJobsByStatus counts live jobs per status.
func (r *GormRepository) CountJobsByStatus(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		Status string
		N      int64
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT COALESCE(status, '') AS status, COUNT(*) AS n FROM song_job
WHERE deleted_at IS NULL GROUP BY status`).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count jobs by status: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Status] = r.N
	}
	return out, nil
}

var _ AdminRepository = (*GormRepository)(nil)
