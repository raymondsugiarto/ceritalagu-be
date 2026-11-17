package mail_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/mail"
)

// smtpServer is a minimal SMTP server that captures one message.
type smtpServer struct {
	ln       net.Listener
	messages chan string
}

// startSMTP starts the fake server on a random port.
func startSMTP(t *testing.T) *smtpServer {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &smtpServer{ln: ln, messages: make(chan string, 1)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handle(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

// handle runs one SMTP conversation.
func (s *smtpServer) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	say := func(line string) {
		_, _ = w.WriteString(line + "\r\n")
		_ = w.Flush()
	}
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			say("250-fake")
			say("250 SIZE 52428800")
		case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"):
			say("250 OK")
		case cmd == "DATA":
			say("354 send data")
			var body strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
				body.WriteString(l)
			}
			s.messages <- body.String()
			say("250 OK queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 OK")
		}
	}
}

// addr is the host:port of the fake server.
func (s *smtpServer) addr() (string, int) {
	host, portStr, _ := net.SplitHostPort(s.ln.Addr().String())
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	return host, port
}

func TestSMTPSendsAttachmentAsMultipart(t *testing.T) {
	t.Parallel()
	srv := startSMTP(t)
	host, port := srv.addr()

	sender, err := mail.New(config.MailConfig{
		Driver: "smtp",
		From:   "Lagu Untukmu <no-reply@example.com>",
		SMTP:   config.SMTPConfig{Host: host, Port: port},
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(context.Background(), mail.Message{
		To:      "nadia@example.com",
		Subject: "Lagu kamu sudah siap",
		Text:    "Halo!",
		Files:   []mail.Attachment{{Name: "lagu.mp3", Content: []byte("MP3DATA")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var raw string
	select {
	case raw = <-srv.messages:
	case <-time.After(5 * time.Second):
		t.Fatal("no message received")
	}

	if !strings.Contains(raw, "multipart/mixed") {
		t.Errorf("message is not multipart:\n%s", raw)
	}
	if !strings.Contains(raw, "attachment; filename=\"lagu.mp3\"") {
		t.Errorf("attachment header missing:\n%s", raw)
	}
	if !strings.Contains(raw, base64.StdEncoding.EncodeToString([]byte("MP3DATA"))) {
		t.Errorf("attachment content missing:\n%s", raw)
	}
	if !strings.Contains(raw, "Halo!") {
		t.Errorf("body missing:\n%s", raw)
	}
}

func TestSMTPPlainTextStaysSinglePart(t *testing.T) {
	t.Parallel()
	srv := startSMTP(t)
	host, port := srv.addr()

	sender, err := mail.New(config.MailConfig{
		Driver: "smtp",
		From:   "Lagu Untukmu <no-reply@example.com>",
		SMTP:   config.SMTPConfig{Host: host, Port: port},
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(context.Background(), mail.Message{To: "a@example.com", Subject: "s", Text: "Halo!"}); err != nil {
		t.Fatal(err)
	}

	var raw string
	select {
	case raw = <-srv.messages:
	case <-time.After(5 * time.Second):
		t.Fatal("no message received")
	}
	if strings.Contains(raw, "multipart") {
		t.Errorf("plain text must stay a single part:\n%s", raw)
	}
}
