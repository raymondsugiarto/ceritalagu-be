package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yourorg/music-api/config"
)

// DefaultBrevoBaseURL is the Brevo API host.
const DefaultBrevoBaseURL = "https://api.brevo.com"

// brevoPath is the transactional email endpoint (POST /v3/smtp/email).
const brevoPath = "/v3/smtp/email"

// maxResponseBytes caps how much of a Brevo error body is read.
const maxResponseBytes = 8 << 10

// ErrBrevoNotConfigured is returned when the Brevo driver is missing its API key or sender.
var ErrBrevoNotConfigured = errors.New("brevo is not configured")

// BrevoSender sends email through the Brevo transactional API.
type BrevoSender struct {
	apiKey      string
	baseURL     string
	senderName  string
	senderEmail string
	sandbox     bool
	timeout     time.Duration
	maxAttempts int
	client      *http.Client
	// RetryBaseDelay is the wait before the second attempt; it doubles after every retry.
	RetryBaseDelay time.Duration
}

// NewBrevo builds a Brevo sender from mail.yml → brevo.
func NewBrevo(cfg config.BrevoConfig) (*BrevoSender, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%w: MAIL_BREVO_APIKEY is missing", ErrBrevoNotConfigured)
	}
	if cfg.SenderEmail == "" {
		return nil, fmt.Errorf("%w: MAIL_BREVO_SENDEREMAIL is missing", ErrBrevoNotConfigured)
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = DefaultBrevoBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	attempts := cfg.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}
	return &BrevoSender{
		apiKey:         cfg.APIKey,
		baseURL:        base,
		senderName:     cfg.SenderName,
		senderEmail:    cfg.SenderEmail,
		sandbox:        cfg.Sandbox,
		timeout:        timeout,
		maxAttempts:    attempts,
		client:         &http.Client{Timeout: timeout},
		RetryBaseDelay: time.Second,
	}, nil
}

// brevoRequest is the POST /v3/smtp/email body.
type brevoRequest struct {
	Sender      brevoAddress      `json:"sender"`
	To          []brevoAddress    `json:"to"`
	Subject     string            `json:"subject,omitempty"`
	TemplateID  int64             `json:"templateId,omitempty"`
	Params      map[string]any    `json:"params,omitempty"`
	HTMLContent string            `json:"htmlContent,omitempty"`
	TextContent string            `json:"textContent,omitempty"`
	Attachment  []brevoAttachment `json:"attachment,omitempty"`
	ReplyTo     *brevoAddress     `json:"replyTo,omitempty"`
}

// brevoAddress is one mailbox.
type brevoAddress struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// brevoAttachment is one attached file; Content is base64-encoded.
type brevoAttachment struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Send delivers m. Attachments are base64-encoded, so the request grows by ~33%.
func (s *BrevoSender) Send(ctx context.Context, m Message) error {
	body, err := s.build(m)
	if err != nil {
		return err
	}
	return s.post(ctx, body)
}

// build turns a Message into the Brevo request body.
func (s *BrevoSender) build(m Message) ([]byte, error) {
	req := brevoRequest{
		Sender:      brevoAddress{Name: s.senderName, Email: s.senderEmail},
		To:          []brevoAddress{{Email: m.To}},
		Subject:     m.Subject,
		HTMLContent: m.HTML,
		TextContent: m.Text,
	}
	if m.TemplateID > 0 {
		// The Brevo template supplies subject and body; sending them too would override it.
		req.TemplateID, req.Params = m.TemplateID, m.Params
		req.Subject, req.HTMLContent, req.TextContent = "", "", ""
	}
	if m.ReplyTo != "" {
		req.ReplyTo = &brevoAddress{Name: m.ReplyToName, Email: m.ReplyTo}
	}
	for _, f := range m.Files {
		req.Attachment = append(req.Attachment, brevoAttachment{
			Name:    f.Name,
			Content: base64.StdEncoding.EncodeToString(f.Content),
		})
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode brevo request: %w", err)
	}
	return body, nil
}

// post sends the request, retrying on 5xx and rate limits.
func (s *BrevoSender) post(ctx context.Context, body []byte) error {
	var lastErr error
	for attempt := 1; attempt <= s.maxAttempts; attempt++ {
		err := s.do(ctx, body)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable(err) || attempt == s.maxAttempts {
			break
		}
		// Back off before the next attempt so a rate-limited account is not hammered.
		wait := s.RetryBaseDelay * time.Duration(1<<(attempt-1))
		select {
		case <-ctx.Done():
			return fmt.Errorf("brevo send: %w", ctx.Err())
		case <-time.After(wait):
		}
	}
	return lastErr
}

// do performs one HTTP call to Brevo.
func (s *BrevoSender) do(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+brevoPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build brevo request: %w", err)
	}
	req.Header.Set("api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.sandbox {
		// Brevo validates the payload but delivers nothing.
		req.Header.Set("X-Sib-Sandbox", "drop")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("brevo send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &brevoError{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
}

// brevoError is a non-2xx answer from Brevo.
type brevoError struct {
	Status int
	Body   string
}

func (e *brevoError) Error() string {
	return fmt.Sprintf("brevo returned %d: %s", e.Status, e.Body)
}

// retryable reports whether another attempt could succeed.
func retryable(err error) bool {
	var be *brevoError
	if !errors.As(err, &be) {
		return false
	}
	return be.Status == http.StatusTooManyRequests || be.Status >= 500
}
