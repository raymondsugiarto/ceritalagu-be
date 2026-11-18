package config

import "time"

// MailConfig is mail.yml: outgoing email (login OTP codes, song notifications).
//
// driver log prints every email to the server log instead of sending it (development only).
// driver brevo sends through the Brevo API; the API key comes from env only: MAIL_BREVO_APIKEY.
type MailConfig struct {
	Driver string      `mapstructure:"driver"` // log | brevo
	Brevo  BrevoConfig `mapstructure:"brevo"`
	// ContactTo is the inbox that receives messages from the website contact form.
	ContactTo string `mapstructure:"contactTo"` // MAIL_CONTACTTO
}

// DefaultContactTo receives contact-form messages when mail.contactTo is empty.
const DefaultContactTo = "hello@ceritalagu.com"

// ContactInbox returns ContactTo, or DefaultContactTo when it is unset.
func (c MailConfig) ContactInbox() string {
	if c.ContactTo == "" {
		return DefaultContactTo
	}
	return c.ContactTo
}

// BrevoConfig is the Brevo transactional email API (POST /v3/smtp/email).
//
// The API key is a secret and comes from env only: MAIL_BREVO_APIKEY.
// The sender address must be a verified sender in the Brevo account.
type BrevoConfig struct {
	APIKey      string        `mapstructure:"apiKey"`      // MAIL_BREVO_APIKEY
	BaseURL     string        `mapstructure:"baseUrl"`     // MAIL_BREVO_BASEURL
	SenderName  string        `mapstructure:"senderName"`  // MAIL_BREVO_SENDERNAME
	SenderEmail string        `mapstructure:"senderEmail"` // MAIL_BREVO_SENDEREMAIL
	Timeout     time.Duration `mapstructure:"timeout"`     // MAIL_BREVO_TIMEOUT
	MaxAttempts int           `mapstructure:"maxAttempts"` // MAIL_BREVO_MAXATTEMPTS
	// Sandbox sends the request with X-Sib-Sandbox: drop: Brevo validates the payload but
	// delivers nothing. Useful to try the setup without mailing real customers.
	Sandbox bool `mapstructure:"sandbox"` // MAIL_BREVO_SANDBOX
}
