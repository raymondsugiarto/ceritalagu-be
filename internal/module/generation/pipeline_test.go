package generation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/prompt"
	"github.com/yourorg/music-api/internal/module/questionnaire"
	"github.com/yourorg/music-api/internal/module/song"
)

// ---- fakes ----

type fakeJobs struct {
	mu       sync.Mutex
	job      song.Job
	statuses []song.JobStatus
	songs    []song.Song
	alts     []song.Alternate
}

func (f *fakeJobs) CreateAlternate(_ context.Context, a song.Alternate) error {
	f.alts = append(f.alts, a)
	return nil
}

func (f *fakeJobs) GetJob(_ context.Context, _ string) (song.Job, error) { return f.job, nil }
func (f *fakeJobs) UpdateJob(_ context.Context, j song.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.job = j
	f.statuses = append(f.statuses, j.Status)
	return nil
}
func (f *fakeJobs) CreateSong(_ context.Context, s song.Song) error {
	f.songs = append(f.songs, s)
	return nil
}

type fakeAnswers struct{ q questionnaire.Questionnaire }

func (f fakeAnswers) Get(context.Context, string) (questionnaire.Questionnaire, error) {
	return f.q, nil
}

type fakePrompts struct{}

func (fakePrompts) Active(context.Context) (prompt.Template, error) { return prompt.Default, nil }
func (fakePrompts) Hints(context.Context, questionnaire.Answers) (prompt.Hints, error) {
	return prompt.Hints{}, nil
}

type fakeProviders struct{ missing aiprovider.Category }

func (f fakeProviders) Active(_ context.Context, c aiprovider.Category) (ResolvedProvider, error) {
	if c == f.missing {
		return ResolvedProvider{}, errors.New("none active")
	}
	t := aiprovider.AdapterType("fake_text")
	if c == aiprovider.CategoryMusic {
		t = "fake_music"
	}
	return ResolvedProvider{ID: "p-" + string(c), AdapterType: t, Config: aiprovider.Config{APIKey: "secret", MaxRetries: 2}}, nil
}

type fakeLyrics struct {
	errs  []error // returned in order before succeeding
	calls int
	got   aiprovider.LyricsStyleRequest
}

func (f *fakeLyrics) GenerateLyricsStyle(_ context.Context, req aiprovider.LyricsStyleRequest, _ aiprovider.Config) (aiprovider.LyricsStyleResult, error) {
	f.calls++
	f.got = req
	if f.calls <= len(f.errs) {
		return aiprovider.LyricsStyleResult{}, f.errs[f.calls-1]
	}
	return aiprovider.LyricsStyleResult{Title: "Untuk Nadia", Lyrics: "[Verse]\na b c d\n[Chorus]\nNadia e f g\n[Verse 2]\nh i j k\n[Chorus]\nNadia l m n", StylePrompt: "pop ballad, piano", VocalGender: "male", Language: "id"}, nil
}
func (f *fakeLyrics) TestConnection(context.Context, aiprovider.Config) error { return nil }

type fakeMusic struct {
	err error
	got aiprovider.MusicRequest
	alt *aiprovider.MusicTrack
}

func (f *fakeMusic) GenerateMusic(_ context.Context, req aiprovider.MusicRequest, _ aiprovider.Config) (aiprovider.MusicResult, error) {
	f.got = req
	if f.err != nil {
		return aiprovider.MusicResult{}, f.err
	}
	return aiprovider.MusicResult{Audio: []byte("FULL"), Format: "mp3", DurationSec: 150, Alternate: f.alt}, nil
}
func (f *fakeMusic) TestConnection(context.Context, aiprovider.Config) error { return nil }

type fakeStorage struct{ files map[string][]byte }

func (f *fakeStorage) Put(_ context.Context, key string, data []byte, _ string) error {
	f.files[key] = data
	return nil
}

type fakeAudio struct{}

func (fakeAudio) Preview(_ context.Context, full []byte, _ float64, _ int) ([]byte, error) {
	return append([]byte("PREVIEW-"), full[:1]...), nil
}
func (fakeAudio) Duration(context.Context, []byte) (int, error) { return 150, nil }

type fakeNotifier struct {
	songs []song.Song
	err   error
}

func (f *fakeNotifier) SongReady(_ context.Context, s song.Song) error {
	f.songs = append(f.songs, s)
	return f.err
}

// ---- harness ----

type harness struct {
	p      *Pipeline
	jobs   *fakeJobs
	lyrics *fakeLyrics
	music  *fakeMusic
	store  *fakeStorage
	sleeps []time.Duration
}

func newHarness(t *testing.T, voice questionnaire.Voice, userID string, provs fakeProviders) *harness {
	t.Helper()
	h := &harness{
		jobs:   &fakeJobs{job: song.Job{ID: "job-1", QuestionnaireID: "q-1", UserID: userID, GuestSessionID: "g-1", Status: song.JobQueued}},
		lyrics: &fakeLyrics{},
		music:  &fakeMusic{},
		store:  &fakeStorage{files: map[string][]byte{}},
	}
	reg := aiprovider.NewRegistry()
	reg.RegisterLyricsStyle("fake_text", h.lyrics)
	reg.RegisterMusic("fake_music", h.music)
	q := questionnaire.Questionnaire{ID: "q-1", Answers: questionnaire.Answers{RecipientName: "Nadia", Genre: "pop", Voice: voice, Language: questionnaire.LanguageID}}

	h.p = NewPipeline(Deps{
		Jobs: h.jobs, Questionnaires: fakeAnswers{q: q}, Prompts: fakePrompts{}, Providers: provs,
		Registry: reg, Storage: h.store, Audio: fakeAudio{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, Options{})
	clock := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	h.p.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	h.p.newID = func() string { return "song-1" }
	h.p.sleep = func(_ context.Context, d time.Duration) error { h.sleeps = append(h.sleeps, d); return nil }
	return h
}

// ---- tests ----

func TestRunHappyPathGuest(t *testing.T) {
	t.Parallel()
	h := newHarness(t, questionnaire.VoiceFemale, "", fakeProviders{})
	if err := h.p.Run(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}

	want := []song.JobStatus{song.JobGeneratingLyrics, song.JobGeneratingMusic, song.JobProcessingAudio, song.JobCompleted}
	if strings.Join(statusStrings(h.jobs.statuses), ",") != strings.Join(statusStrings(want), ",") {
		t.Fatalf("statuses = %v, want %v", h.jobs.statuses, want)
	}
	if len(h.jobs.songs) != 1 {
		t.Fatalf("want 1 song, got %d", len(h.jobs.songs))
	}
	s := h.jobs.songs[0]
	if s.FullAudioKey != "guests/g-1/songs/song-1/full.mp3" || s.PreviewAudioKey != "guests/g-1/songs/song-1/preview.mp3" {
		t.Fatalf("keys: %s %s", s.FullAudioKey, s.PreviewAudioKey)
	}
	if string(h.store.files[s.FullAudioKey]) != "FULL" || !strings.HasPrefix(string(h.store.files[s.PreviewAudioKey]), "PREVIEW") {
		t.Fatalf("stored files: %v", h.store.files)
	}
	if s.ExpiresAt == nil || s.PreviewDurationSec != 77 || s.DurationSec != 150 {
		t.Fatalf("guest song: expires=%v preview=%d duration=%d", s.ExpiresAt, s.PreviewDurationSec, s.DurationSec)
	}
	// Customer chose female: overrides the AI's "male" and lands in the style prompt.
	if s.VocalGender != "female" || h.music.got.StylePrompt != "pop ballad, piano, female vocal" {
		t.Fatalf("voice not applied: %q / %q", s.VocalGender, h.music.got.StylePrompt)
	}
	if !strings.Contains(h.lyrics.got.UserPrompt, "Nadia") {
		t.Fatal("prompt not rendered with answers")
	}
	snap := h.jobs.job.LyricsProviderSnapshot
	if snap["id"] != "p-lyrics_style" || strings.Contains(strings.ToLower(toString(snap)), "secret") {
		t.Fatalf("snapshot must identify the provider without the key: %v", snap)
	}
	if h.jobs.job.Timings["total"] <= 0 {
		t.Fatal("total timing not recorded")
	}
}

func TestRunRegisteredUserHasNoExpiry(t *testing.T) {
	t.Parallel()
	h := newHarness(t, questionnaire.VoiceSurprise, "user-1", fakeProviders{})
	if err := h.p.Run(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	s := h.jobs.songs[0]
	if s.FullAudioKey != "users/user-1/songs/song-1/full.mp3" || s.PreviewAudioKey != "users/user-1/songs/song-1/preview.mp3" {
		t.Fatalf("user song keys: %s %s", s.FullAudioKey, s.PreviewAudioKey)
	}
	if s.ExpiresAt != nil || s.OwnerUserID != "user-1" || s.VocalGender != "male" {
		t.Fatalf("unexpected song: %+v", s)
	}
}

func TestRunRetriesRetryableErrors(t *testing.T) {
	t.Parallel()
	h := newHarness(t, questionnaire.VoiceMale, "", fakeProviders{})
	h.lyrics.errs = []error{
		&aiprovider.Error{Kind: aiprovider.KindRateLimited},
		&aiprovider.Error{Kind: aiprovider.KindInvalidOutput},
	}
	if err := h.p.Run(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	if h.lyrics.calls != 3 || len(h.sleeps) != 2 || h.sleeps[1] != 2*h.sleeps[0] {
		t.Fatalf("calls=%d sleeps=%v", h.lyrics.calls, h.sleeps)
	}
}

func TestRunFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		setup    func(h *harness)
		provs    fakeProviders
		wantCode string
	}{
		{"auth error is not retried", func(h *harness) {
			h.lyrics.errs = []error{&aiprovider.Error{Kind: aiprovider.KindAuth}}
		}, fakeProviders{}, string(aiprovider.KindAuth)},
		{"retries exhausted", func(h *harness) {
			e := &aiprovider.Error{Kind: aiprovider.KindUnavailable}
			h.lyrics.errs = []error{e, e, e}
		}, fakeProviders{}, string(aiprovider.KindUnavailable)},
		{"content flagged by music", func(h *harness) {
			h.music.err = &aiprovider.Error{Kind: aiprovider.KindContentFlagged}
		}, fakeProviders{}, string(aiprovider.KindContentFlagged)},
		{"no music provider", func(*harness) {}, fakeProviders{missing: aiprovider.CategoryMusic}, CodeNoProvider},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, questionnaire.VoiceMale, "", tt.provs)
			tt.setup(h)
			if err := h.p.Run(context.Background(), "job-1"); err == nil {
				t.Fatal("want error")
			}
			j := h.jobs.job
			if j.Status != song.JobFailed || j.ErrorCode != tt.wantCode || j.FinishedAt == nil {
				t.Fatalf("job = status %s code %q, want failed %q", j.Status, j.ErrorCode, tt.wantCode)
			}
			if len(h.jobs.songs) != 0 {
				t.Fatal("no song may be saved on failure")
			}
			if song.UserMessage(j.ErrorCode) == "" {
				t.Fatal("every code needs a user message")
			}
		})
	}
}

func TestRunSkipsFinishedJob(t *testing.T) {
	t.Parallel()
	h := newHarness(t, questionnaire.VoiceMale, "", fakeProviders{})
	h.jobs.job.Status = song.JobCompleted
	if err := h.p.Run(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	if h.lyrics.calls != 0 || len(h.jobs.statuses) != 0 {
		t.Fatal("finished job must not run again")
	}
}

func TestRunNotifiesCustomer(t *testing.T) {
	t.Parallel()
	h := newHarness(t, questionnaire.VoiceMale, "user-1", fakeProviders{})
	notifier := &fakeNotifier{}
	h.p.notifier = notifier
	if err := h.p.Run(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	if len(notifier.songs) != 1 {
		t.Fatalf("notified %d times, want 1", len(notifier.songs))
	}
	if notifier.songs[0].ID != "song-1" || notifier.songs[0].OwnerUserID != "user-1" {
		t.Fatalf("notified song = %+v", notifier.songs[0])
	}
}

func TestRunSurvivesNotificationFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t, questionnaire.VoiceMale, "user-1", fakeProviders{})
	h.p.notifier = &fakeNotifier{err: errors.New("brevo down")}
	if err := h.p.Run(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	if h.jobs.job.Status != song.JobCompleted {
		t.Fatalf("job status = %q, want completed (the song is saved)", h.jobs.job.Status)
	}
}

func statusStrings(s []song.JobStatus) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = string(v)
	}
	return out
}

func toString(m map[string]any) string {
	var b strings.Builder
	for k, v := range m {
		b.WriteString(k)
		if s, ok := v.(string); ok {
			b.WriteString(s)
		}
	}
	return b.String()
}

func TestRunStoresSpareTake(t *testing.T) {
	t.Parallel()
	h := newHarness(t, questionnaire.VoiceSurprise, "user-1", fakeProviders{})
	h.jobs.job.RegenerateOfJobID = "root-job"
	ids := []string{"song-1", "song-2"}
	h.p.newID = func() string { id := ids[0]; ids = ids[1:]; return id }
	h.music.alt = &aiprovider.MusicTrack{Audio: []byte("ALT"), Format: "mp3", DurationSec: 140}
	if err := h.p.Run(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	if len(h.jobs.songs) != 1 || len(h.jobs.alts) != 1 {
		t.Fatalf("want 1 song + 1 spare, got %d + %d", len(h.jobs.songs), len(h.jobs.alts))
	}
	a := h.jobs.alts[0]
	if a.SongID != "song-2" || a.JobID != "job-1" || a.RootJobID != "root-job" || a.DurationSec != 140 {
		t.Fatalf("spare: %+v", a)
	}
	if a.FullAudioKey != "users/user-1/songs/song-2/full.mp3" || string(h.store.files[a.FullAudioKey]) != "ALT" {
		t.Fatalf("spare full audio not stored: %s %v", a.FullAudioKey, h.store.files)
	}
	if !strings.HasPrefix(string(h.store.files[a.PreviewAudioKey]), "PREVIEW") || a.PreviewDurationSec <= 0 {
		t.Fatalf("spare preview not stored: %+v", a)
	}
}

func TestRunWithoutSpareTake(t *testing.T) {
	t.Parallel()
	h := newHarness(t, questionnaire.VoiceSurprise, "", fakeProviders{})
	if err := h.p.Run(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	if len(h.jobs.alts) != 0 {
		t.Fatalf("no second take, want no spare, got %d", len(h.jobs.alts))
	}
}
