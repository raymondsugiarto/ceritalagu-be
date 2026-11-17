// Package notification sends the "your song is ready" email to the customer.
//
// The email carries the finished song as an attachment. Which file is attached is a setting:
// the preview for everyone, or the full MP3 once the customer has bought the song.
// The recipient address comes from the account created when the customer filled the questionnaire.
package notification

// Setting are the notification rules (notification.yml).
type Setting struct {
	// Enabled turns the module off: no email is sent and nothing is read.
	Enabled bool
	// AttachFullAudio attaches the full song instead of the 60 s preview.
	AttachFullAudio bool
	// MaxAttachmentBytes caps the attached file. A bigger file is skipped (the email still goes out).
	MaxAttachmentBytes int
	// SiteURL is the public site, used to build the "listen" link in the email.
	SiteURL string
}
