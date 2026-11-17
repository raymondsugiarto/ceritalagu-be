// Package mail sends transactional email (customer login codes, song notifications).
package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

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
	case "smtp":
		from, err := mail.ParseAddress(cfg.From)
		if err != nil {
			return nil, fmt.Errorf("mail.from: %w", err)
		}
		return &SMTPSender{cfg: cfg.SMTP, from: from}, nil
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
	s.log.Warn("mail driver is log: email NOT sent", "to", m.To, "subject", m.Subject, "body", m.Text, "attachments", names)
	return nil
}

// SMTPSender sends through an SMTP relay.
type SMTPSender struct {
	cfg  config.SMTPConfig
	from *mail.Address
}

const smtpTimeout = 15 * time.Second

// Send delivers m. Port 465 uses implicit TLS; other ports upgrade with STARTTLS when offered.
func (s *SMTPSender) Send(ctx context.Context, m Message) error {
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("mail to: %w", err)
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	d := net.Dialer{Timeout: smtpTimeout}
	var conn net.Conn
	if s.cfg.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp hello: %w", err)
	}
	defer func() { _ = c.Close() }()
	if ok, _ := c.Extension("STARTTLS"); ok && s.cfg.Port != 465 {
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(build(s.from, to, m)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp end data: %w", err)
	}
	if err := c.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}

func build(from, to *mail.Address, m Message) []byte {
	host := from.Address[strings.LastIndex(from.Address, "@")+1:]
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from.String())
	fmt.Fprintf(&b, "To: %s\r\n", to.String())
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", uuid.NewString(), host)
	b.WriteString("MIME-Version: 1.0\r\n")
	if len(m.Files) == 0 && m.HTML == "" {
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
		b.WriteString(strings.ReplaceAll(m.Text, "\n", "\r\n"))
		return []byte(b.String())
	}
	b.WriteString(buildMultipart(m))
	return []byte(b.String())
}

// buildMultipart builds a multipart/mixed body (an alternative part plus the attachments).
// Attachments are base64-encoded and wrapped at 76 characters, as MIME requires.
func buildMultipart(m Message) string {
	boundary := "mail-" + uuid.NewString()
	var b strings.Builder
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", boundary)

	// The body itself is an alternative part (plain text and/or HTML).
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary+"-alt")
	if m.Text != "" {
		fmt.Fprintf(&b, "--%s-alt\r\n", boundary)
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
		b.WriteString(strings.ReplaceAll(m.Text, "\n", "\r\n"))
		b.WriteString("\r\n")
	}
	if m.HTML != "" {
		fmt.Fprintf(&b, "--%s-alt\r\n", boundary)
		b.WriteString("Content-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
		b.WriteString(strings.ReplaceAll(m.HTML, "\n", "\r\n"))
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s-alt--\r\n", boundary)

	for _, f := range m.Files {
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		fmt.Fprintf(&b, "Content-Type: %s; name=%q\r\n", contentType(f.Name), f.Name)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-Disposition: attachment; filename=%q\r\n\r\n", f.Name)
		b.WriteString(wrapBase64(f.Content))
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.String()
}

// contentType guesses the MIME type of an attachment from its extension.
func contentType(name string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")) {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "m4a":
		return "audio/mp4"
	case "flac":
		return "audio/flac"
	case "ogg":
		return "audio/ogg"
	case "pdf":
		return "application/pdf"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}

// wrapBase64 encodes data and wraps it at 76 characters per line.
func wrapBase64(data []byte) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76])
		b.WriteString("\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
	return b.String()
}
