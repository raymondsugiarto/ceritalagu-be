package contact

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/guest"
)

type fakeSender struct {
	sent []mail.Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, m mail.Message) error {
	f.sent = append(f.sent, m)
	return f.err
}

func newSvc(s mail.Sender) *Service {
	return NewService(s, "hello@ceritalagu.com", slog.New(slog.NewTextHandler(io.Discard, nil)))
}

var visitor = guest.Visitor{SessionID: "sess-1", IPHash: "abc123"}

func validInput() SendContactInput {
	return SendContactInput{Name: "  Rina  ", Email: " Rina@Example.com ", Topic: "refund", Message: "Lagu saya belum terbuka setelah bayar."}
}

func TestSendForwardsToInboxWithReplyTo(t *testing.T) {
	t.Parallel()
	fs := &fakeSender{}
	out, err := newSvc(fs).Send(context.Background(), visitor, validInput())
	if err != nil || !out.Sent {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if len(fs.sent) != 1 {
		t.Fatalf("sent %d emails", len(fs.sent))
	}
	m := fs.sent[0]
	if m.To != "hello@ceritalagu.com" {
		t.Errorf("to = %q", m.To)
	}
	if m.ReplyTo != "rina@example.com" || m.ReplyToName != "Rina" {
		t.Errorf("reply-to = %q <%q>", m.ReplyToName, m.ReplyTo)
	}
	if !strings.Contains(m.Subject, "Pengembalian dana") || !strings.Contains(m.Subject, "Rina") {
		t.Errorf("subject = %q", m.Subject)
	}
	if !strings.Contains(m.Text, "Lagu saya belum terbuka") || !strings.Contains(m.Text, "sess-1") {
		t.Errorf("text body missing content: %q", m.Text)
	}
}

func TestSendEscapesHTMLAndSubjectNewlines(t *testing.T) {
	t.Parallel()
	fs := &fakeSender{}
	in := validInput()
	in.Name = "Evil\r\nBcc: x@y.z"
	in.Message = "<script>alert(1)</script> halo halo"
	if _, err := newSvc(fs).Send(context.Background(), visitor, in); err != nil {
		t.Fatal(err)
	}
	m := fs.sent[0]
	if strings.ContainsAny(m.Subject, "\r\n") {
		t.Errorf("subject has a line break: %q", m.Subject)
	}
	if strings.Contains(m.HTML, "<script>") || !strings.Contains(m.HTML, "&lt;script&gt;") {
		t.Errorf("html not escaped: %q", m.HTML)
	}
}

func TestSendHoneypotSendsNothing(t *testing.T) {
	t.Parallel()
	fs := &fakeSender{}
	in := validInput()
	in.Website = "http://spam.example"
	out, err := newSvc(fs).Send(context.Background(), visitor, in)
	if err != nil || !out.Sent {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if len(fs.sent) != 0 {
		t.Errorf("honeypot still sent %d emails", len(fs.sent))
	}
}

func TestSendMailerFailureIsUnavailable(t *testing.T) {
	t.Parallel()
	fs := &fakeSender{err: errors.New("brevo down")}
	_, err := newSvc(fs).Send(context.Background(), visitor, validInput())
	var ae *apperror.Error
	if !errors.As(err, &ae) || ae.Status != 503 {
		t.Fatalf("err = %#v", err)
	}
}
