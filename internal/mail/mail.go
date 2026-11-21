// Package mail sends transactional email (customer login codes, song notifications).
package mail

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yourorg/music-api/config"
)

// Attachment is a file sent along with a message (e.g. the finished song).
type Attachment struct {
	Name    string // file name shown to the customer, e.g. "lagu-untuk-nadia.mp3"
	Content []byte
}

// Message is one email.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string       // optional HTML body; sent as an alternative to Text
	Files   []Attachment // optional attachments
	// ReplyTo, when set, is where the recipient's "Reply" goes (e.g. the customer who used the contact form).
	ReplyTo     string
	ReplyToName string
	// TemplateID, when > 0, sends a template stored in Brevo (Marketing → Templates) instead of
	// Subject/Text/HTML: the template owns the subject and the body, filled from Params
	// ({{ params.KEY }} in the template). Drivers without templates fall back to Subject/Text/HTML,
	// so callers should still fill those.
	TemplateID int64
	Params     map[string]any
}

// Sender delivers a message.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// New picks the driver from cfg (mail.driver).
func New(cfg config.MailConfig, log *slog.Logger) (Sender, error) {
	switch cfg.Driver {
	case "log":
		return LogSender{log: log}, nil
	case "brevo":
		return NewBrevo(cfg.Brevo)
	default:
		return nil, fmt.Errorf("unknown mail driver %q", cfg.Driver)
	}
}

// LogSender writes emails to the log instead of sending them (development).
type LogSender struct{ log *slog.Logger }

// Send logs m, body included, so a developer can read the login code.
func (s LogSender) Send(_ context.Context, m Message) error {
	names := make([]string, 0, len(m.Files))
	for _, f := range m.Files {
		names = append(names, fmt.Sprintf("%s (%d bytes)", f.Name, len(f.Content)))
	}
	s.log.Warn("mail driver is log: email NOT sent", "to", m.To, "reply_to", m.ReplyTo, "subject", m.Subject, "body", m.Text,
		"template_id", m.TemplateID, "params", m.Params, "attachments", names)
	return nil
}
