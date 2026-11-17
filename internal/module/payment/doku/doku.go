// Package doku is a minimal DOKU Checkout (Non-SNAP) client: create a hosted payment page, check an
// order's status and verify HTTP notifications.
//
// Every request is signed: Signature: HMACSHA256=base64(HMAC-SHA256(secretKey, components)), where
// components are "Client-Id:…\nRequest-Id:…\nRequest-Timestamp:…\nRequest-Target:<path>" plus
// "\nDigest:base64(SHA-256(body))" for requests with a body.
package doku

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// API paths.
const (
	checkoutPath = "/checkout/v1/payment"
	statusPath   = "/orders/v1/status/"
)

// Transaction statuses DOKU reports (transaction.status).
const (
	StatusSuccess = "SUCCESS"
	StatusFailed  = "FAILED"
	StatusPending = "PENDING"
	StatusExpired = "EXPIRED"
	StatusTimeout = "TIMEOUT"
)

// Client calls DOKU.
type Client struct {
	baseURL   string
	clientID  string
	secretKey string
	http      *http.Client
	now       func() time.Time
}

// New builds a client. baseURL: https://api-sandbox.doku.com or https://api.doku.com.
func New(baseURL, clientID, secretKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"), clientID: clientID, secretKey: secretKey,
		http: &http.Client{Timeout: 30 * time.Second},
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// CheckoutRequest is what the merchant sends to open a payment page.
type CheckoutRequest struct {
	InvoiceNumber string
	Amount        int64 // whole IDR
	DueMinutes    int
	CallbackURL   string // "Back to merchant" after paying
	ItemName      string
	CustomerID    string
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
}

// CheckoutResult is the hosted payment page.
type CheckoutResult struct {
	RequestID  string
	PaymentURL string
	Raw        map[string]any
}

// StatusResult is an order's status as DOKU sees it.
type StatusResult struct {
	Status        string // SUCCESS | FAILED | PENDING | EXPIRED | TIMEOUT | …
	InvoiceNumber string
	Amount        int64
	Channel       string
	Raw           map[string]any
}

// Error is a non-2xx answer from DOKU.
type Error struct {
	HTTPStatus int
	Messages   []string
}

func (e *Error) Error() string {
	return fmt.Sprintf("doku: http %d: %s", e.HTTPStatus, strings.Join(e.Messages, "; "))
}

// ErrNotFound is a status check for an order DOKU does not know (the customer never opened the page).
var ErrNotFound = errors.New("doku: order not found")

// CreateCheckout opens a DOKU Checkout payment page for the order.
func (c *Client) CreateCheckout(ctx context.Context, r CheckoutRequest) (CheckoutResult, error) {
	name := r.ItemName
	if name == "" {
		name = "Order " + r.InvoiceNumber
	}
	body := map[string]any{
		"order": map[string]any{
			"amount":         r.Amount,
			"invoice_number": r.InvoiceNumber,
			"currency":       "IDR",
			"callback_url":   r.CallbackURL,
			"auto_redirect":  true,
			"line_items": []map[string]any{
				{"id": "song-unlock", "name": truncate(name, 255), "price": r.Amount, "quantity": 1},
			},
		},
		"payment": map[string]any{"payment_due_date": r.DueMinutes},
	}
	customer := map[string]any{}
	for k, v := range map[string]string{"id": r.CustomerID, "name": r.CustomerName, "email": r.CustomerEmail, "phone": r.CustomerPhone} {
		if v != "" {
			customer[k] = v
		}
	}
	if len(customer) > 0 {
		body["customer"] = customer
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("doku: encode checkout: %w", err)
	}
	reqID := uuid.NewString()
	resp, err := c.do(ctx, http.MethodPost, checkoutPath, reqID, raw)
	if err != nil {
		return CheckoutResult{}, err
	}
	var parsed struct {
		Response struct {
			Payment struct {
				URL string `json:"url"`
			} `json:"payment"`
		} `json:"response"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return CheckoutResult{}, fmt.Errorf("doku: decode checkout: %w", err)
	}
	if parsed.Response.Payment.URL == "" {
		return CheckoutResult{}, errors.New("doku: checkout response has no payment url")
	}
	return CheckoutResult{RequestID: reqID, PaymentURL: parsed.Response.Payment.URL, Raw: asMap(resp)}, nil
}

// CheckStatus asks DOKU for the order's transaction status.
func (c *Client) CheckStatus(ctx context.Context, invoiceNumber string) (StatusResult, error) {
	resp, err := c.do(ctx, http.MethodGet, statusPath+url.PathEscape(invoiceNumber), uuid.NewString(), nil)
	var de *Error
	if errors.As(err, &de) && de.HTTPStatus == http.StatusNotFound {
		return StatusResult{}, ErrNotFound
	}
	if err != nil {
		return StatusResult{}, err
	}
	return ParseResult(resp)
}

// ParseResult reads a notification or status-check body (both share the same shape).
func ParseResult(body []byte) (StatusResult, error) {
	var p struct {
		Order struct {
			InvoiceNumber string `json:"invoice_number"`
			Amount        any    `json:"amount"`
		} `json:"order"`
		Transaction struct {
			Status string `json:"status"`
		} `json:"transaction"`
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
		Service struct {
			ID string `json:"id"`
		} `json:"service"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return StatusResult{}, fmt.Errorf("doku: decode result: %w", err)
	}
	channel := p.Channel.ID
	if channel == "" {
		channel = p.Service.ID
	}
	return StatusResult{
		Status: strings.ToUpper(p.Transaction.Status), InvoiceNumber: p.Order.InvoiceNumber,
		Amount: toInt(p.Order.Amount), Channel: channel, Raw: asMap(body),
	}, nil
}

// VerifyNotification checks the headers DOKU sent with a notification to target (the path of the
// Notification URL) against body.
func (c *Client) VerifyNotification(h http.Header, target string, body []byte) bool {
	if h.Get("Client-Id") != c.clientID {
		return false
	}
	want := c.Sign(h.Get("Client-Id"), h.Get("Request-Id"), h.Get("Request-Timestamp"), target, body)
	return hmac.Equal([]byte(h.Get("Signature")), []byte(want))
}

// Sign returns the Signature header value. body nil means no Digest component (GET).
func (c *Client) Sign(clientID, requestID, timestamp, target string, body []byte) string {
	var b strings.Builder
	b.WriteString("Client-Id:" + clientID)
	b.WriteString("\nRequest-Id:" + requestID)
	b.WriteString("\nRequest-Timestamp:" + timestamp)
	b.WriteString("\nRequest-Target:" + target)
	if body != nil {
		sum := sha256.Sum256(body)
		b.WriteString("\nDigest:" + base64.StdEncoding.EncodeToString(sum[:]))
	}
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(b.String()))
	return "HMACSHA256=" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (c *Client) do(ctx context.Context, method, path, requestID string, body []byte) ([]byte, error) {
	ts := c.now().Format("2006-01-02T15:04:05Z")
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("doku: build request: %w", err)
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Request-Id", requestID)
	req.Header.Set("Request-Timestamp", ts)
	req.Header.Set("Signature", c.Sign(c.clientID, requestID, ts, path, body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doku: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("doku: read response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, &Error{HTTPStatus: resp.StatusCode, Messages: errorMessages(data)}
	}
	return data, nil
}

// errorMessages reads {"error_messages": [...]} or {"message": [...]}, falling back to the raw body.
func errorMessages(body []byte) []string {
	var p struct {
		ErrorMessages []string `json:"error_messages"`
		Message       any      `json:"message"`
	}
	if json.Unmarshal(body, &p) == nil {
		if len(p.ErrorMessages) > 0 {
			return p.ErrorMessages
		}
		switch m := p.Message.(type) {
		case string:
			return []string{m}
		case []any:
			out := make([]string, 0, len(m))
			for _, x := range m {
				out = append(out, fmt.Sprint(x))
			}
			return out
		}
	}
	return []string{truncate(string(body), 300)}
}

func asMap(b []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// toInt reads an amount sent as a number or a string ("20000", "20000.00").
func toInt(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err == nil {
			return int64(f)
		}
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
