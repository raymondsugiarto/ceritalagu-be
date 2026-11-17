package config

import "time"

// MailConfig is mail.yml: outgoing email (login OTP codes, song notifications).
//
// driver log prints every email to the server log instead of sending it (development only).
// driver smtp sends through an SMTP server; the password comes from env only: MAIL_SMTP_PASSWORD.
// driver brevo sends through the Brevo API; the API key comes from env only: MAIL_BREVO_APIKEY.
type MailConfig struct {
	Driver string      `mapstructure:"driver"` // log | smtp | brevo
	From   string      `mapstructure:"from"`   // e.g. "Lagu Untukmu <no-reply@example.com>"
	SMTP   SMTPConfig  `mapstructure:"smtp"`
	Brevo  BrevoConfig `mapstructure:"brevo"`
}

// SMTPConfig is an SMTP relay. Port 465 uses implicit TLS; any other port uses STARTTLS when offered.
type SMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
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
