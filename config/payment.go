package config

import "time"

// PaymentConfig is payment.yml: unlocking a song's full version through DOKU Checkout.
//
// Payments stay off (checkout answers 503) until payment.doku.clientId and the secret key are set.
// The secret key comes from env only: PAYMENT_DOKU_SECRETKEY.
type PaymentConfig struct {
	UnlockPrice int64         `mapstructure:"unlockPrice"` // whole IDR
	OrderTTL    time.Duration `mapstructure:"orderTTL"`    // DOKU payment_due_date; a pending order is reused until then
	// PublicBaseURL is the customer site, e.g. https://laguuntukmu.id. DOKU sends the customer back to
	// <PublicBaseURL>/songs/<song_id>?order=<order_id> after paying.
	PublicBaseURL string     `mapstructure:"publicBaseUrl"`
	Doku          DokuConfig `mapstructure:"doku"`
}

// DokuConfig is the DOKU Checkout (Non-SNAP) merchant account.
type DokuConfig struct {
	BaseURL   string `mapstructure:"baseUrl"` // https://api-sandbox.doku.com | https://api.doku.com
	ClientID  string `mapstructure:"clientId"`
	SecretKey string `mapstructure:"secretKey"`
	// NotifyPath is the path of the Notification URL set in DOKU Back Office. It is part of the
	// notification signature, so it must match what DOKU calls (as seen from the internet, before any proxy).
	NotifyPath string `mapstructure:"notifyPath"`
}

// Enabled reports whether DOKU credentials are configured.
func (d DokuConfig) Enabled() bool { return d.BaseURL != "" && d.ClientID != "" && d.SecretKey != "" }
