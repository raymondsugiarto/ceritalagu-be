package song_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/song"
)

type fixture struct {
	svc   *song.Service
	repo  *memRepo
	queue *fakeQueue
	v     guest.Visitor
	qID   string
}

func newFixture() *fixture {
	f := &fixture{repo: newMemRepo(), queue: &fakeQueue{},
		v: guest.Visitor{SessionID: uuid.NewString(), IPHash: "ip-a"}, qID: uuid.NewString()}
	f.svc = song.NewService(f.repo, ownedAnswers{f.qID: f.v.SessionID}, f.queue, fakeAudio{},
		song.Limits{GuestDailyQuota: 3, MaxRegenerate: 2})
	return f
}

func status(err error) int {
	var ae *apperror.Error
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

func TestCreateJobQueuesAndEnforcesQuota(t *testing.T) {
	t.Parallel()
	f := newFixture()
	ctx := context.Background()
	for i := range 3 {
		out, err := f.svc.CreateJob(ctx, f.v, song.CreateSongJobInput{QuestionnaireID: f.qID})
		if err != nil {
			t.Fatalf("job %d: %v", i+1, err)
		}
		if out.Status != song.JobQueued || out.RegeneratesLeft != 2 || out.StatusMessage == "" {
			t.Fatalf("unexpected output: %+v", out)
		}
	}
	if len(f.queue.ids) != 3 {
		t.Fatalf("enqueued %d, want 3", len(f.queue.ids))
	}
	_, err := f.svc.CreateJob(ctx, f.v, song.CreateSongJobInput{QuestionnaireID: f.qID})
	if status(err) != http.StatusTooManyRequests {
		t.Fatalf("4th job: want 429, got %v", err)
	}

	// A new cookie from the same IP shares the quota.
	other := guest.Visitor{SessionID: uuid.NewString(), IPHash: "ip-a"}
	f.repo.ipOf[f.v.SessionID] = "ip-a"
	svc := song.NewService(f.repo, ownedAnswers{f.qID: other.SessionID}, f.queue, fakeAudio{}, song.Limits{GuestDailyQuota: 3, MaxRegenerate: 2})
	if _, err := svc.CreateJob(ctx, other, song.CreateSongJobInput{QuestionnaireID: f.qID}); status(err) != http.StatusTooManyRequests {
		t.Fatalf("same IP, new cookie: want 429, got %v", err)
	}
}

func TestCreateJobRejectsForeignQuestionnaire(t *testing.T) {
	t.Parallel()
	f := newFixture()
	stranger := guest.Visitor{SessionID: uuid.NewString()}
	if _, err := f.svc.CreateJob(context.Background(), stranger, song.CreateSongJobInput{QuestionnaireID: f.qID}); status(err) != http.StatusNotFound {
		t.Fatalf("want 404, got %v", err)
	}
}

func TestCreateJobEnqueueFailure(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.queue.fail = true
	_, err := f.svc.CreateJob(context.Background(), f.v, song.CreateSongJobInput{QuestionnaireID: f.qID})
	if status(err) != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %v", err)
	}
	for _, j := range f.repo.jobs {
		if j.Status != song.JobFailed || j.ErrorCode != song.CodeEnqueueFailed {
			t.Fatalf("job not marked failed: %+v", j)
		}
	}
}

func TestGetJobAndSong(t *testing.T) {
	t.Parallel()
	f := newFixture()
	ctx := context.Background()
	job, _ := f.svc.CreateJob(ctx, f.v, song.CreateSongJobInput{QuestionnaireID: f.qID})
	s := f.repo.finish(job.ID, false)

	got, err := f.svc.GetJob(ctx, f.v, job.ID)
	if err != nil || got.Status != song.JobCompleted || got.SongID != s.ID {
		t.Fatalf("job = %+v, %v", got, err)
	}

	out, err := f.svc.GetSong(ctx, f.v, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Lyrics != "" || !strings.HasSuffix(out.LyricsPreview, "Nadia") || out.PreviewURL != "/api/v1/songs/"+s.ID+"/preview" {
		t.Fatalf("locked song leaked or preview wrong: %+v", out)
	}

	stranger := guest.Visitor{SessionID: uuid.NewString()}
	if _, err := f.svc.GetSong(ctx, stranger, s.ID); status(err) != http.StatusNotFound {
		t.Fatalf("stranger: want 404, got %v", err)
	}
	if _, err := f.svc.GetJob(ctx, stranger, job.ID); status(err) != http.StatusNotFound {
		t.Fatalf("stranger job: want 404, got %v", err)
	}
}

func TestUnlockedSongHasFullLyrics(t *testing.T) {
	t.Parallel()
	f := newFixture()
	job, _ := f.svc.CreateJob(context.Background(), f.v, song.CreateSongJobInput{QuestionnaireID: f.qID})
	s := f.repo.finish(job.ID, true)
	out, _ := f.svc.GetSong(context.Background(), f.v, s.ID)
	if !strings.Contains(out.Lyrics, "[Bridge]") {
		t.Fatalf("unlocked song must include full lyrics: %+v", out)
	}
}

func TestRegenerateLimits(t *testing.T) {
	t.Parallel()
	f := newFixture()
	ctx := context.Background()
	job, _ := f.svc.CreateJob(ctx, f.v, song.CreateSongJobInput{QuestionnaireID: f.qID})

	if _, err := f.svc.Regenerate(ctx, f.v, job.ID); status(err) != http.StatusConflict {
		t.Fatalf("regenerate while queued: want 409, got %v", err)
	}
	f.repo.finish(job.ID, false)

	r1, err := f.svc.Regenerate(ctx, f.v, job.ID)
	if err != nil || r1.RegenerateOfJobID != job.ID || r1.RegeneratesLeft != 1 {
		t.Fatalf("regen 1 = %+v, %v", r1, err)
	}
	f.repo.finish(r1.ID, false)
	// Regenerating the regenerated song still counts against the original.
	r2, err := f.svc.Regenerate(ctx, f.v, r1.ID)
	if err != nil || r2.RegenerateOfJobID != job.ID || r2.RegeneratesLeft != 0 {
		t.Fatalf("regen 2 = %+v, %v", r2, err)
	}
	f.repo.finish(r2.ID, false)
	if _, err := f.svc.Regenerate(ctx, f.v, job.ID); status(err) != http.StatusTooManyRequests {
		t.Fatalf("regen 3: want 429, got %v", err)
	}
	// Regenerations do not use the daily quota: 2 more new songs still fit.
	for range 2 {
		if _, err := f.svc.CreateJob(ctx, f.v, song.CreateSongJobInput{QuestionnaireID: f.qID}); err != nil {
			t.Fatalf("new song after regenerations: %v", err)
		}
	}
}
