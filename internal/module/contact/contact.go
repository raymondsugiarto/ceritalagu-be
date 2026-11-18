// Package contact forwards messages from the website's "Hubungi Kami" form to the support inbox.
package contact

// Message is one contact-form submission. It is emailed, not stored.
type Message struct {
	Name    string
	Email   string
	Topic   string
	Body    string
	IPHash  string // for spotting abuse in the forwarded email; never the raw IP
	Session string // guest session ID, to look up the visitor's songs
}
