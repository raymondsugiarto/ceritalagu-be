package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/module/user"
)

// Sender delivers an email (mail.Sender).
type Sender interface {
	Send(ctx context.Context, m mail.Message) error
}

// Users reads the customer's account (user.Repository).
type Users interface {
	GetByID(ctx context.Context, id string) (user.User, error)
}

// AudioStore reads the stored audio (storage.Store).
type AudioStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// ErrNoRecipient is returned when the song has no customer to write to.
var ErrNoRecipient = errors.New("notification: song has no recipient email")

// Service sends the "your song is ready" email.
type Service struct {
	sender Sender
	users  Users
	audio  AudioStore
	opts   Setting
	log    *slog.Logger
}

// NewService builds a notification service.
func NewService(sender Sender, users Users, audio AudioStore, opts Setting, log *slog.Logger) *Service {
	return &Service{sender: sender, users: users, audio: audio, opts: opts, log: log}
}

// SongReady emails the song to its owner, with the audio attached.
//
// The preview is attached by default; the full song only when the setting allows it. A file over
// the size cap is skipped: the customer still gets the email with the link to the song page.
func (s *Service) SongReady(ctx context.Context, sg song.Song) error {
	if !s.opts.Enabled {
		return nil
	}
	if sg.OwnerUserID == "" {
		return fmt.Errorf("%w: song %s", ErrNoRecipient, sg.ID)
	}
	u, err := s.users.GetByID(ctx, sg.OwnerUserID)
	if err != nil {
		return fmt.Errorf("load song owner %s: %w", sg.OwnerUserID, err)
	}
	if u.Email == "" {
		return fmt.Errorf("%w: song %s", ErrNoRecipient, sg.ID)
	}

	key := sg.PreviewAudioKey
	if s.opts.AttachFullAudio {
		key = sg.FullAudioKey
	}

	var files []mail.Attachment
	if key != "" {
		data, err := s.audio.Get(ctx, key)
		switch {
		case err != nil:
			// A missing file must not lose the email: the link is still useful.
			s.log.Warn("song audio not attached", "song_id", sg.ID, "key", key, "err", err)
		case s.opts.MaxAttachmentBytes > 0 && len(data) > s.opts.MaxAttachmentBytes:
			s.log.Warn("song audio too big to attach", "song_id", sg.ID, "bytes", len(data),
				"max_bytes", s.opts.MaxAttachmentBytes)
		default:
			files = append(files, mail.Attachment{Name: fileName(sg.Title, filepath.Ext(key)), Content: data})
		}
	}

	msg := mail.Message{
		To:      u.Email,
		Subject: subject(sg),
		Text:    s.body(sg, u),
		HTML:    s.html(sg, u),
		Files:   files,
	}
	if err := s.sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("send song-ready email to %s: %w", u.Email, err)
	}
	s.log.Info("song-ready email sent", "song_id", sg.ID, "to", u.Email, "attached", len(files) > 0)
	return nil
}

// subject is the email subject line.
func subject(sg song.Song) string {
	if sg.Title == "" {
		return "Lagu kamu sudah siap!"
	}
	return "Lagu kamu sudah siap: " + sg.Title
}

// body is the plain-text email. It shows the lyrics up to the first chorus, like the preview:
// the full lyrics belong to the paid version.
func (s *Service) body(sg song.Song, u user.User) string {
	var b strings.Builder
	b.WriteString("Halo" + greeting(u) + "!\n\n")
	b.WriteString("Lagu " + title(sg) + " sudah selesai dibuat.\n\n")
	if lyrics := song.PreviewLyrics(sg.Lyrics); lyrics != "" {
		b.WriteString("Lirik:\n" + lyrics + "\n\n")
	}
	if s.opts.SiteURL != "" {
		b.WriteString("Dengarkan di " + strings.TrimRight(s.opts.SiteURL, "/") + "/songs/" + sg.ID + "\n\n")
	}
	b.WriteString("Sampai jumpa,\nLagu Untukmu\n")
	return b.String()
}

// html is the HTML email.
func (s *Service) html(sg song.Song, u user.User) string {
	var b strings.Builder
	b.WriteString("<html><body>")
	b.WriteString("<p>Halo" + escapeHTML(greeting(u)) + "!</p>")
	b.WriteString("<p>Lagu " + escapeHTML(title(sg)) + " sudah selesai dibuat.</p>")
	if lyrics := song.PreviewLyrics(sg.Lyrics); lyrics != "" {
		b.WriteString("<p>Lirik:</p><pre>" + escapeHTML(lyrics) + "</pre>")
	}
	if s.opts.SiteURL != "" {
		link := strings.TrimRight(s.opts.SiteURL, "/") + "/songs/" + sg.ID
		b.WriteString(`<p><a href="` + escapeHTML(link) + `">Dengarkan lagumu</a></p>`)
	}
	b.WriteString("<p>Sampai jumpa,<br>Lagu Untukmu</p>")
	b.WriteString("</body></html>")
	return b.String()
}

// title names the song in the email.
func title(sg song.Song) string {
	if sg.Title == "" {
		return "kamu"
	}
	return sg.Title
}

// greeting is the name suffix ("", " Nadia").
func greeting(u user.User) string {
	if u.Name == "" {
		return ""
	}
	return " " + u.Name
}

// fileName builds a safe attachment name, e.g. "untuk-nadia.mp3".
func fileName(title, ext string) string {
	slug := strings.ToLower(title)
	var b strings.Builder
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "lagu"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	return name + ext
}

// escapeHTML escapes text for the HTML body.
func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}
