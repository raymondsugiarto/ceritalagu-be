package config

// NotificationConfig is notification.yml: the "your song is ready" email.
//
// The email is sent by the worker right after a song is generated. The attached file is the
// 60 s preview, or the full song when attachFullAudio is on.
type NotificationConfig struct {
	Enabled bool `mapstructure:"enabled"` // NOTIFICATION_ENABLED
	// AttachFullAudio attaches the full song instead of the preview.
	AttachFullAudio bool `mapstructure:"attachFullAudio"` // NOTIFICATION_ATTACHFULLAUDIO
	// MaxAttachmentMB caps the attached file (raw bytes). A bigger file is skipped and only
	// the link is sent. Base64 grows the payload by ~33%, so keep room for that.
	MaxAttachmentMB int `mapstructure:"maxAttachmentMB"` // NOTIFICATION_MAXATTACHMENTMB
	// SiteURL is the public site, used to build the link in the email.
	SiteURL string `mapstructure:"siteUrl"` // NOTIFICATION_SITEURL
	// PaymentReceipt is the "payment succeeded" email, sent by the API when an order is paid.
	PaymentReceipt PaymentReceiptConfig `mapstructure:"paymentReceipt"`
}

// PaymentReceiptConfig is notification.yml → paymentReceipt.
type PaymentReceiptConfig struct {
	Enabled bool `mapstructure:"enabled"` // NOTIFICATION_PAYMENTRECEIPT_ENABLED
	// BrevoTemplateID is the Brevo template to send (0 = built-in plain email).
	BrevoTemplateID int64 `mapstructure:"brevoTemplateId"` // NOTIFICATION_PAYMENTRECEIPT_BREVOTEMPLATEID
	// AttachFullAudio attaches the full MP3 when it fits under notification.maxAttachmentMB.
	AttachFullAudio bool `mapstructure:"attachFullAudio"` // NOTIFICATION_PAYMENTRECEIPT_ATTACHFULLAUDIO
}
