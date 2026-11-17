package prompt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/audit"
	"github.com/yourorg/music-api/internal/module/playground"
	"github.com/yourorg/music-api/internal/module/questionnaire"
)

type memStore struct {
	rows    map[string]Template
	max     int
	hints   Hints
	actions []string
}

func newMem() *memStore { return &memStore{rows: map[string]Template{}} }

func (m *memStore) List(context.Context) ([]Template, error) {
	var out []Template
	for _, t := range m.rows {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out, nil
}

func (m *memStore) Get(_ context.Context, id string) (Template, error) {
	t, ok := m.rows[id]
	if !ok {
		return Template{}, ErrNotFound
	}
	return t, nil
}

func (m *memStore) Create(_ context.Context, t Template) (Template, error) {
	m.max++
	t.Version, t.ID = m.max, fmt.Sprintf("v%d", m.max)
	m.rows[t.ID] = t
	return t, nil
}

func (m *memStore) Activate(_ context.Context, id string) error {
	if _, ok := m.rows[id]; !ok {
		return ErrNotFound
	}
	for k, t := range m.rows {
		t.IsActive = k == id
		m.rows[k] = t
	}
	return nil
}

func (m *memStore) DeactivateAll(context.Context) error {
	for k, t := range m.rows {
		t.IsActive = false
		m.rows[k] = t
	}
	return nil
}

func (m *memStore) Archive(_ context.Context, id string) error {
	delete(m.rows, id)
	return nil
}

func (m *memStore) Hints(context.Context, questionnaire.Answers) (Hints, error) { return m.hints, nil }

func (m *memStore) Record(_ context.Context, e audit.Entry) error {
	m.actions = append(m.actions, e.Action)
	return nil
}

type fakeTester struct {
	got      aiprovider.LyricsStyleRequest
	res      aiprovider.LyricsStyleResult
	err      error
	gotMusic aiprovider.MusicRequest
	music    aiprovider.MusicResult
	musicErr error
}

func (f *fakeTester) TestMusic(_ context.Context, req aiprovider.MusicRequest) (aiprovider.MusicResult, TesterInfo, error) {
	f.gotMusic = req
	return f.music, TesterInfo{Provider: "MiniMax", Model: "music-3.0"}, f.musicErr
}

func (f *fakeTester) TestLyrics(_ context.Context, req aiprovider.LyricsStyleRequest, _ questionnaire.Voice) (aiprovider.LyricsStyleResult, TesterInfo, error) {
	f.got = req
	return f.res, TesterInfo{Provider: "MiniMax", Model: "M2"}, f.err
}

func newSvc() (*Service, *memStore, *fakeTester) {
	m, ft := newMem(), &fakeTester{}
	return NewService(m, ft, m), m, ft
}

func validInput() CreateVersionInput {
	return CreateVersionInput{Name: "warm", SystemPrompt: "Write in {{language}}.", UserPrompt: "For {{name}}"}
}

func TestCreateVersionsAndActivate(t *testing.T) {
	t.Parallel()
	svc, m, _ := newSvc()
	ctx := context.Background()

	v1, err := svc.Create(ctx, "admin1", validInput())
	if err != nil {
		t.Fatal(err)
	}
	in := validInput()
	in.Activate = true
	v2, err := svc.Create(ctx, "admin1", in)
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || v2.Version != 2 || !v2.IsActive {
		t.Fatalf("versions: v1=%+v v2=%+v", v1, v2)
	}

	list, _ := svc.List(ctx)
	if list.DefaultInUse || list.Default.IsActive || len(list.Versions) != 2 || list.Versions[0].Version != 2 {
		t.Fatalf("list after activate: %+v", list)
	}

	// Rollback to v1.
	if _, err := svc.Activate(ctx, "admin1", v1.ID); err != nil {
		t.Fatal(err)
	}
	if !m.rows[v1.ID].IsActive || m.rows[v2.ID].IsActive {
		t.Fatal("rollback did not switch the active version")
	}

	if err := svc.UseDefault(ctx, "admin1"); err != nil {
		t.Fatal(err)
	}
	if list, _ = svc.List(ctx); !list.DefaultInUse || !list.Default.IsActive {
		t.Fatal("default should be in use")
	}
	want := "prompt_template.create,prompt_template.create,prompt_template.activate,prompt_template.activate,prompt_template.use_default"
	if got := strings.Join(m.actions, ","); got != want {
		t.Fatalf("audit = %s", got)
	}
}

func TestCreateRejectsUnknownPlaceholderAndEmpty(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	in := validInput()
	in.UserPrompt = "Hi {{nme}} and {{ name }}"
	in.SystemPrompt = "  "
	_, err := svc.Create(context.Background(), "a", in)
	var ae *apperror.Error
	if !errors.As(err, &ae) || len(ae.Details) != 2 {
		t.Fatalf("want 2 validation details, got %v", err)
	}
	if !strings.Contains(ae.Details[1].Message, "{{nme}}, {{ name }}") {
		t.Fatalf("unexpected message %q", ae.Details[1].Message)
	}
}

func TestArchiveActiveIsRefused(t *testing.T) {
	t.Parallel()
	svc, m, _ := newSvc()
	in := validInput()
	in.Activate = true
	v, _ := svc.Create(context.Background(), "a", in)
	if err := svc.Archive(context.Background(), "a", v.ID); err == nil {
		t.Fatal("archiving the active version must fail")
	}
	_ = svc.UseDefault(context.Background(), "a")
	if err := svc.Archive(context.Background(), "a", v.ID); err != nil {
		t.Fatal(err)
	}
	if len(m.rows) != 0 {
		t.Fatal("version not archived")
	}
}

func TestPlaygroundRendersAndRuns(t *testing.T) {
	t.Parallel()
	svc, m, ft := newSvc()
	m.hints = Hints{GenreName: "Pop", Genre: "bright pop"}
	ft.res = aiprovider.LyricsStyleResult{Title: "Untuk Nadia", Lyrics: "[Chorus] ...", StylePrompt: "pop", Raw: "{}"}

	out, err := svc.Playground(context.Background(), PlaygroundInput{
		SystemPrompt: "Lang {{language}} {{oops}}", UserPrompt: "{{name}} likes {{genre}} ({{genre_hint}})",
		Answers: SampleAnswers{RecipientName: "Nadia", Genre: "pop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ft.got.UserPrompt != "Nadia likes Pop (bright pop)" || ft.got.SystemPrompt != "Lang Bahasa Indonesia {{oops}}" {
		t.Fatalf("rendered request: %+v", ft.got)
	}
	if !out.OK || out.Result == nil || out.Result.Title != "Untuk Nadia" || out.Provider != "MiniMax" {
		t.Fatalf("output: %+v", out)
	}
	if len(out.Rendered.UnknownPlaceholders) != 1 || out.Rendered.UnknownPlaceholders[0] != "oops" {
		t.Fatalf("unknown placeholders: %v", out.Rendered.UnknownPlaceholders)
	}
}

func TestPlaygroundProviderErrorIsNotHTTPError(t *testing.T) {
	t.Parallel()
	svc, _, ft := newSvc()
	ft.res = aiprovider.LyricsStyleResult{Raw: "not json"}
	ft.err = &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: "minimax", Message: "no JSON object in reply"}
	out, err := svc.Playground(context.Background(), PlaygroundInput{SystemPrompt: "s", UserPrompt: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if out.OK || out.ErrorKind != "invalid_output" || out.Raw != "not json" {
		t.Fatalf("output: %+v", out)
	}

	ft.err = fmt.Errorf("%w: none", ErrNoProvider)
	out, _ = svc.Playground(context.Background(), PlaygroundInput{SystemPrompt: "s", UserPrompt: "u"})
	if out.ErrorKind != "no_active_provider" {
		t.Fatalf("output: %+v", out)
	}
}

func TestDefaultUsesOnlyKnownPlaceholders(t *testing.T) {
	t.Parallel()
	if bad := UnknownPlaceholders(Default.SystemPrompt + Default.UserPrompt); len(bad) != 0 {
		t.Fatalf("default template uses unknown placeholders %v", bad)
	}
}

func TestPlaygroundMusic(t *testing.T) {
	svc, _, ft := newSvc()
	ft.music = aiprovider.MusicResult{Audio: []byte("ID3abc"), DurationSec: 95}
	out, err := svc.PlaygroundMusic(context.Background(), "admin-1", MusicPlaygroundInput{Lyrics: "[Verse]\nla", StylePrompt: "pop"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.Format != "mp3" || out.AudioDurationSec != 95 || out.SizeBytes != 6 || out.AudioBase64 != "SUQzYWJj" {
		t.Fatalf("unexpected output %+v", out)
	}
	if ft.gotMusic.Lyrics != "[Verse]\nla" || ft.gotMusic.StylePrompt != "pop" || out.Model != "music-3.0" {
		t.Fatalf("request not passed through: %+v / %+v", ft.gotMusic, out)
	}
}

func TestPlaygroundMusicNoProvider(t *testing.T) {
	svc, _, ft := newSvc()
	ft.musicErr = fmt.Errorf("%w: none", ErrNoMusicProvider)
	out, err := svc.PlaygroundMusic(context.Background(), "admin-1", MusicPlaygroundInput{Lyrics: "la"})
	if err != nil {
		t.Fatal(err)
	}
	if out.OK || out.ErrorKind != "no_active_provider" || out.AudioBase64 != "" {
		t.Fatalf("unexpected output %+v", out)
	}
}

type fakeSaver struct {
	got     playground.SaveInput
	adminID string
	err     error
}

func (f *fakeSaver) Save(_ context.Context, adminID string, in playground.SaveInput) (string, error) {
	f.got, f.adminID = in, adminID
	if f.err != nil {
		return "", f.err
	}
	return "pg-1", nil
}

func TestPlaygroundMusicSavesSong(t *testing.T) {
	svc, _, ft := newSvc()
	saver := &fakeSaver{}
	svc.WithPlaygroundSaver(saver)
	ft.music = aiprovider.MusicResult{Audio: []byte("ID3abc"), DurationSec: 95}
	out, err := svc.PlaygroundMusic(context.Background(), "admin-1", MusicPlaygroundInput{Title: "Untuk Ibu", Lyrics: "la", StylePrompt: "pop"})
	if err != nil {
		t.Fatal(err)
	}
	if out.SavedSongID != "pg-1" || out.SaveError != "" {
		t.Fatalf("unexpected output %+v", out)
	}
	if saver.adminID != "admin-1" || saver.got.Title != "Untuk Ibu" || saver.got.Format != "mp3" || string(saver.got.Audio) != "ID3abc" {
		t.Fatalf("saver got %+v (%s)", saver.got, saver.adminID)
	}
}

func TestPlaygroundMusicSaveFailureStillReturnsAudio(t *testing.T) {
	svc, _, ft := newSvc()
	svc.WithPlaygroundSaver(&fakeSaver{err: errors.New("bucket down")})
	ft.music = aiprovider.MusicResult{Audio: []byte("ID3abc")}
	out, err := svc.PlaygroundMusic(context.Background(), "admin-1", MusicPlaygroundInput{Lyrics: "la"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.AudioBase64 == "" || out.SavedSongID != "" || out.SaveError == "" {
		t.Fatalf("unexpected output %+v", out)
	}
}
