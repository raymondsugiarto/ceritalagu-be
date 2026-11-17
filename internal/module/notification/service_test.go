package notification_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/notification"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/module/user"
)

// ---- fakes ----

type fakeSender struct {
	sent []mail.Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, m mail.Message) error {
	f.sent = append(f.sent, m)
	return f.err
}

type fakeUsers struct {
	users map[string]user.User
	err   error
}

func (f *fakeUsers) GetByID(_ context.Context, id string) (user.User, error) {
	if f.err != nil {
		return user.User{}, f.err
	}
	u, ok := f.users[id]
	if !ok {
		return user.User{}, errors.New("user not found")
	}
	return u, nil
}

type fakeAudio struct {
	files map[string][]byte
	err   error
}

func (f *fakeAudio) Get(_ context.Context, key string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.files[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return data, nil
}

// ---- harness ----

type fixture struct {
	svc    *notification.Service
	sender *fakeSender
	users  *fakeUsers
	audio  *fakeAudio
}

func newFixture(opts notification.Setting) *fixture {
	f := &fixture{
		sender: &fakeSender{},
		users:  &fakeUsers{users: map[string]user.User{"u-1": {ID: "u-1", Name: "Nadia", Email: "nadia@example.com"}}},
		audio:  &fakeAudio{files: map[string][]byte{"preview.mp3": []byte("PREVIEW"), "full.mp3": []byte("FULL")}},
	}
	f.svc = notification.NewService(f.sender, f.users, f.audio, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return f
}

func songFixture() song.Song {
	return song.Song{
		ID: "song-1", Title: "Untuk Nadia", Lyrics: "[Verse]\nBintang\n[Chorus]\nNadia",
		OwnerUserID: "u-1", FullAudioKey: "full.mp3", PreviewAudioKey: "preview.mp3",
	}
}

// ---- tests ----

func TestSongReadySendsPreviewWithAttachment(t *testing.T) {
	t.Parallel()
	f := newFixture(notification.Setting{Enabled: true, MaxAttachmentBytes: 1024 * 1024})
	if err := f.svc.SongReady(context.Background(), songFixture()); err != nil {
		t.Fatal(err)
	}
	if len(f.sender.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(f.sender.sent))
	}
	msg := f.sender.sent[0]
	if msg.To != "nadia@example.com" {
		t.Errorf("to = %q", msg.To)
	}
	if !strings.Contains(msg.Subject, "Untuk Nadia") {
		t.Errorf("subject = %q", msg.Subject)
	}
	if len(msg.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(msg.Files))
	}
	if string(msg.Files[0].Content) != "PREVIEW" {
		t.Errorf("attached = %q, want the preview", msg.Files[0].Content)
	}
	if !strings.HasSuffix(msg.Files[0].Name, ".mp3") {
		t.Errorf("attachment name = %q", msg.Files[0].Name)
	}
	if !strings.Contains(msg.Text, "Nadia") {
		t.Errorf("body = %q", msg.Text)
	}
	if msg.HTML == "" {
		t.Error("html body missing")
	}
}

func TestSongReadyAttachesFullAudioWhenAllowed(t *testing.T) {
	t.Parallel()
	f := newFixture(notification.Setting{Enabled: true, AttachFullAudio: true, MaxAttachmentBytes: 1024 * 1024})
	if err := f.svc.SongReady(context.Background(), songFixture()); err != nil {
		t.Fatal(err)
	}
	if string(f.sender.sent[0].Files[0].Content) != "FULL" {
		t.Errorf("attached = %q, want the full song", f.sender.sent[0].Files[0].Content)
	}
}

func TestSongReadySkipsOversizedFile(t *testing.T) {
	t.Parallel()
	f := newFixture(notification.Setting{Enabled: true, MaxAttachmentBytes: 2})
	if err := f.svc.SongReady(context.Background(), songFixture()); err != nil {
		t.Fatal(err)
	}
	if len(f.sender.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(f.sender.sent))
	}
	if len(f.sender.sent[0].Files) != 0 {
		t.Errorf("files = %d, want 0 (too big)", len(f.sender.sent[0].Files))
	}
}

func TestSongReadyDisabled(t *testing.T) {
	t.Parallel()
	f := newFixture(notification.Setting{Enabled: false})
	if err := f.svc.SongReady(context.Background(), songFixture()); err != nil {
		t.Fatal(err)
	}
	if len(f.sender.sent) != 0 {
		t.Errorf("sent = %d, want 0", len(f.sender.sent))
	}
}

func TestSongReadyWithoutRecipient(t *testing.T) {
	t.Parallel()
	f := newFixture(notification.Setting{Enabled: true})
	sg := songFixture()
	sg.OwnerUserID = ""
	if err := f.svc.SongReady(context.Background(), sg); !errors.Is(err, notification.ErrNoRecipient) {
		t.Fatalf("err = %v, want ErrNoRecipient", err)
	}
	if len(f.sender.sent) != 0 {
		t.Errorf("sent = %d, want 0", len(f.sender.sent))
	}
}

func TestSongReadyReportsSendFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(notification.Setting{Enabled: true})
	f.sender.err = errors.New("brevo down")
	if err := f.svc.SongReady(context.Background(), songFixture()); err == nil {
		t.Fatal("want error")
	}
}
