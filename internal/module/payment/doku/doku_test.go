package doku

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSignMatchesDokuFormat(t *testing.T) {
	c := New("https://example.test", "MCH-0001", "SK-secret")
	body := []byte(`{"order":{"amount":20000}}`)
	sig := c.Sign("MCH-0001", "req-1", "2020-08-11T08:45:42Z", "/checkout/v1/payment", body)
	if len(sig) < 20 || sig[:11] != "HMACSHA256=" {
		t.Fatalf("signature %q has no HMACSHA256= prefix", sig)
	}
	// GET has no Digest, so the same inputs without a body sign differently.
	if sig == c.Sign("MCH-0001", "req-1", "2020-08-11T08:45:42Z", "/checkout/v1/payment", nil) {
		t.Fatal("digest is not part of the signature")
	}
}

func TestVerifyNotification(t *testing.T) {
	c := New("https://example.test", "MCH-0001", "SK-secret")
	body := []byte(`{"order":{"invoice_number":"LU-1","amount":49000},"transaction":{"status":"SUCCESS"}}`)
	h := http.Header{}
	h.Set("Client-Id", "MCH-0001")
	h.Set("Request-Id", "n-1")
	h.Set("Request-Timestamp", "2026-11-16T10:00:00Z")
	h.Set("Signature", c.Sign("MCH-0001", "n-1", "2026-11-16T10:00:00Z", "/api/v1/payments/doku/notifications", body))

	if !c.VerifyNotification(h, "/api/v1/payments/doku/notifications", body) {
		t.Fatal("valid notification rejected")
	}
	if c.VerifyNotification(h, "/api/v1/payments/doku/notifications", []byte(`{"tampered":true}`)) {
		t.Fatal("tampered body accepted")
	}
	h.Set("Client-Id", "OTHER")
	if c.VerifyNotification(h, "/api/v1/payments/doku/notifications", body) {
		t.Fatal("other client id accepted")
	}
}

func TestCreateCheckoutSignsAndReadsURL(t *testing.T) {
	var c *Client
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		want := c.Sign(r.Header.Get("Client-Id"), r.Header.Get("Request-Id"), r.Header.Get("Request-Timestamp"), r.URL.Path, body)
		if r.URL.Path != "/checkout/v1/payment" || r.Header.Get("Signature") != want {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error_messages":["Invalid Header Signature"]}`))
			return
		}
		var got map[string]map[string]any
		_ = json.Unmarshal(body, &got)
		if got["order"]["invoice_number"] != "LU-1" || got["order"]["amount"].(float64) != 49000 {
			t.Errorf("unexpected body %s", body)
		}
		_, _ = w.Write([]byte(`{"message":["SUCCESS"],"response":{"payment":{"url":"https://sandbox.doku.com/checkout-link-v2/abc"}}}`))
	}))
	defer srv.Close()
	c = New(srv.URL, "MCH-0001", "SK-secret")
	c.now = func() time.Time { return time.Date(2026, 11, 16, 3, 0, 0, 0, time.UTC) }

	res, err := c.CreateCheckout(context.Background(), CheckoutRequest{InvoiceNumber: "LU-1", Amount: 49000, DueMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	if res.PaymentURL != "https://sandbox.doku.com/checkout-link-v2/abc" {
		t.Fatalf("payment url = %q", res.PaymentURL)
	}
}

func TestParseResultStringAmount(t *testing.T) {
	r, err := ParseResult([]byte(`{"order":{"invoice_number":"LU-2","amount":"49000.00"},"transaction":{"status":"success"},"channel":{"id":"VIRTUAL_ACCOUNT_BCA"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Amount != 49000 || r.Status != StatusSuccess || r.Channel != "VIRTUAL_ACCOUNT_BCA" {
		t.Fatalf("parsed %+v", r)
	}
}
