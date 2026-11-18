package contact

import "strings"

// Topics a visitor can pick; the code goes into the email subject.
var Topics = map[string]string{
	"general": "Pertanyaan umum",
	"order":   "Pesanan & pembayaran",
	"refund":  "Pengembalian dana",
	"song":    "Masalah lagu",
	"partner": "Kerja sama",
	"other":   "Lainnya",
}

// SendContactInput is the body of POST /contact.
type SendContactInput struct {
	Name    string `json:"name"    validate:"required,min=2,max=80"`
	Email   string `json:"email"   validate:"required,email,max=254"`
	Topic   string `json:"topic"   validate:"required,oneof=general order refund song partner other"`
	Message string `json:"message" validate:"required,min=10,max=2000"`
	// Website is a honeypot: hidden from people, filled in by naive bots.
	Website string `json:"website" validate:"max=200"`
}

// normalize trims whitespace and lower-cases the email.
func (in *SendContactInput) normalize() {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Message = strings.TrimSpace(in.Message)
}

// SendContactOutput is the "data" of a successful POST /contact.
type SendContactOutput struct {
	Sent bool `json:"sent"`
}
