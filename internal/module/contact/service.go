package contact

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/inputguard"
	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/guest"
)

// Service emails contact-form messages to the support inbox.
type Service struct {
	mailer mail.Sender
	inbox  string
	log    *slog.Logger
}

// NewService builds the service; inbox is the address that receives the messages.
func NewService(mailer mail.Sender, inbox string, log *slog.Logger) *Service {
	return &Service{mailer: mailer, inbox: inbox, log: log}
}

// Send forwards one message. Replying to the email answers the visitor directly.
func (s *Service) Send(ctx context.Context, v guest.Visitor, in SendContactInput) (SendContactOutput, error) {
	in.normalize()
	if in.Website != "" {
		// Honeypot tripped: pretend it worked so the bot learns nothing.
		s.log.Info("contact form honeypot tripped", "ip_hash", v.IPHash)
		return SendContactOutput{Sent: true}, nil
	}
	if len(in.Name) < 2 || len(in.Message) < 10 {
		return SendContactOutput{}, apperror.BadRequest("Nama dan pesan wajib diisi.").WithReason(apperror.ReasonContactFieldsMissing)
	}
	// Messages go to a person, not to the AI, so a question about the technology is allowed here;
	// commands, injection payloads and harmful content are not.
	for field, value := range map[string]string{"name": in.Name, "message": in.Message} {
		if viol, bad := inputguard.Check(value); bad && viol.Category != inputguard.CategoryAIProbe {
			s.log.Warn("contact input rejected", "field", field, "category", viol.Category, "match", viol.Match, "ip_hash", v.IPHash)
			return SendContactOutput{}, apperror.Validation([]apperror.ErrorDetail{{Field: field,
				Message: "Pesan ini berisi hal yang tidak bisa kami proses.", Reason: apperror.ReasonInputNotAllowed}})
		}
	}

	m := Message{Name: in.Name, Email: in.Email, Topic: in.Topic, Body: in.Message,
		IPHash: v.IPHash, Session: v.SessionID}
	if err := s.mailer.Send(ctx, buildEmail(s.inbox, m)); err != nil {
		return SendContactOutput{}, apperror.Unavailable("Pesan belum terkirim. Coba lagi sebentar lagi.", err).WithReason(apperror.ReasonContactSendFailed)
	}
	return SendContactOutput{Sent: true}, nil
}

// buildEmail renders the message for the support inbox, with Reply-To set to the visitor.
func buildEmail(inbox string, m Message) mail.Message {
	topic := Topics[m.Topic]
	if topic == "" {
		topic = m.Topic
	}
	subject := fmt.Sprintf("[Kontak] %s — %s", topic, oneLine(m.Name))

	text := fmt.Sprintf("Pesan baru dari formulir kontak CeritaLagu\n\nNama: %s\nEmail: %s\nTopik: %s\n\n%s\n\n--\nGuest session: %s\nIP hash: %s\nBalas email ini untuk menjawab langsung ke pengirim.\n",
		m.Name, m.Email, topic, m.Body, m.Session, m.IPHash)

	e := html.EscapeString
	body := strings.ReplaceAll(e(m.Body), "\n", "<br>")
	htmlBody := fmt.Sprintf(`<p><strong>Pesan baru dari formulir kontak CeritaLagu</strong></p>
<table cellpadding="4">
<tr><td>Nama</td><td>%s</td></tr>
<tr><td>Email</td><td><a href="mailto:%s">%s</a></td></tr>
<tr><td>Topik</td><td>%s</td></tr>
</table>
<p style="white-space:pre-wrap">%s</p>
<hr><p style="color:#888;font-size:12px">Guest session: %s · IP hash: %s<br>Balas email ini untuk menjawab langsung ke pengirim.</p>`,
		e(m.Name), e(m.Email), e(m.Email), e(topic), body, e(m.Session), e(m.IPHash))

	return mail.Message{
		To: inbox, Subject: subject, Text: text, HTML: htmlBody,
		ReplyTo: m.Email, ReplyToName: oneLine(m.Name),
	}
}

// oneLine strips line breaks so user input cannot break the subject line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
