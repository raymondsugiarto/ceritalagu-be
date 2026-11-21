package notification

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/song"
)

// Songs reads a song (song.GormRepository).
type Songs interface {
	GetSong(ctx context.Context, id string) (song.Song, error)
}

// ReceiptSetting are the "payment succeeded" email rules (notification.yml → paymentReceipt).
type ReceiptSetting struct {
	// Enabled turns the email off.
	Enabled bool
	// TemplateID is the Brevo template to send (0 = the built-in subject and body below).
	TemplateID int64
	// AttachFullAudio attaches the full MP3 when it is under MaxAttachmentBytes.
	AttachFullAudio    bool
	MaxAttachmentBytes int
	// SiteURL is the public site, used to build the link to the song page.
	SiteURL string
	// BrandName signs the email ("Cerita Lagu").
	BrandName string
}

// PaidOrder is what the receipt needs from a paid order (payment.Order).
type PaidOrder struct {
	OrderID         string
	InvoiceNumber   string
	UserID          string
	SongID          string
	Amount          int64 // charged, whole IDR
	NormalPrice     int64
	SalePrice       int64
	VoucherCode     string
	VoucherDiscount int64
	PaymentMethod   string // DOKU channel, e.g. VIRTUAL_ACCOUNT_BCA
	PaidAt          time.Time
}

// ReceiptService sends the "payment succeeded, here is your full song" email.
type ReceiptService struct {
	sender Sender
	users  Users
	songs  Songs
	audio  AudioStore
	opts   ReceiptSetting
	log    *slog.Logger
}

// NewReceiptService builds the payment receipt sender. audio may be nil (no attachment).
func NewReceiptService(sender Sender, users Users, songs Songs, audio AudioStore, opts ReceiptSetting, log *slog.Logger) *ReceiptService {
	if opts.BrandName == "" {
		opts.BrandName = "Cerita Lagu"
	}
	return &ReceiptService{sender: sender, users: users, songs: songs, audio: audio, opts: opts, log: log}
}

// jakarta is WIB, the time zone shown to customers.
var jakarta = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*60*60)
}()

// PaymentSucceeded emails the buyer the receipt and the link to the full song.
func (s *ReceiptService) PaymentSucceeded(ctx context.Context, o PaidOrder) error {
	if !s.opts.Enabled {
		return nil
	}
	_, err := s.deliver(ctx, o, "")
	return err
}

// ResendPaymentReceipt sends the receipt again, even when the automatic email is off.
// to overrides the recipient (a test inbox); empty sends to the buyer.
func (s *ReceiptService) ResendPaymentReceipt(ctx context.Context, o PaidOrder, to string) (Sent, error) {
	return s.deliver(ctx, o, to)
}

func (s *ReceiptService) deliver(ctx context.Context, o PaidOrder, to string) (Sent, error) {
	var name string
	if o.UserID != "" {
		u, err := s.users.GetByID(ctx, o.UserID)
		if err != nil && to == "" {
			return Sent{}, fmt.Errorf("load buyer %s: %w", o.UserID, err)
		}
		name = u.Name
		if to == "" {
			to = u.Email
		}
	}
	if to == "" {
		return Sent{}, fmt.Errorf("%w: order %s", ErrNoRecipient, o.OrderID)
	}
	sg, err := s.songs.GetSong(ctx, o.SongID)
	if err != nil {
		return Sent{}, fmt.Errorf("load song %s: %w", o.SongID, err)
	}

	p := s.params(o, sg, name)
	msg := mail.Message{
		To:         to,
		Subject:    "Pembayaran berhasil: " + p["SONG_TITLE"].(string),
		Text:       receiptText(p),
		HTML:       receiptHTML(p),
		TemplateID: s.opts.TemplateID,
		Params:     p,
		Files:      s.attachment(ctx, sg),
	}
	if err := s.sender.Send(ctx, msg); err != nil {
		return Sent{}, fmt.Errorf("send payment receipt to %s: %w", to, err)
	}
	s.log.Info("payment receipt email sent", "order_id", o.OrderID, "song_id", sg.ID, "to", to,
		"template_id", s.opts.TemplateID, "attached", len(msg.Files) > 0)
	return Sent{To: to, TemplateID: s.opts.TemplateID, Attached: len(msg.Files) > 0}, nil
}

// params are the Brevo template variables ({{ params.KEY }}). Every value is a ready-to-show string.
func (s *ReceiptService) params(o PaidOrder, sg song.Song, name string) map[string]any {
	songTitle := sg.Title
	if songTitle == "" {
		songTitle = "Lagu kamu"
	}
	songURL := ""
	if s.opts.SiteURL != "" {
		songURL = strings.TrimRight(s.opts.SiteURL, "/") + "/songs/" + sg.ID
	}
	discount := ""
	if o.VoucherDiscount > 0 {
		discount = "-" + rupiah(o.VoucherDiscount)
	}
	normal := ""
	if o.NormalPrice > o.Amount {
		normal = rupiah(o.NormalPrice)
	}
	return map[string]any{
		"NAME":           name,
		"GREETING_NAME":  firstNonEmpty(name, "kamu"),
		"SONG_TITLE":     songTitle,
		"SONG_URL":       songURL,
		"SITE_URL":       strings.TrimRight(s.opts.SiteURL, "/"),
		"INVOICE":        o.InvoiceNumber,
		"AMOUNT":         rupiah(o.Amount),
		"NORMAL_PRICE":   normal,
		"VOUCHER_CODE":   o.VoucherCode,
		"DISCOUNT":       discount,
		"PAYMENT_METHOD": paymentMethod(o.PaymentMethod),
		"PAID_AT":        o.PaidAt.In(jakarta).Format("02/01/2006 15:04") + " WIB",
		"BRAND":          s.opts.BrandName,
		"YEAR":           strconv.Itoa(o.PaidAt.In(jakarta).Year()),
	}
}

// attachment is the full MP3 when the setting allows it and it fits under the cap.
func (s *ReceiptService) attachment(ctx context.Context, sg song.Song) []mail.Attachment {
	if !s.opts.AttachFullAudio || s.audio == nil || sg.FullAudioKey == "" {
		return nil
	}
	data, err := s.audio.Get(ctx, sg.FullAudioKey)
	switch {
	case err != nil:
		s.log.Warn("full song not attached to receipt", "song_id", sg.ID, "err", err)
		return nil
	case s.opts.MaxAttachmentBytes > 0 && len(data) > s.opts.MaxAttachmentBytes:
		s.log.Warn("full song too big to attach to receipt", "song_id", sg.ID, "bytes", len(data))
		return nil
	}
	return []mail.Attachment{{Name: fileName(sg.Title, filepath.Ext(sg.FullAudioKey)), Content: data}}
}

// rupiah formats whole IDR: 49000 → "Rp49.000".
func rupiah(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-Rp" + b.String()
	}
	return "Rp" + b.String()
}

// paymentMethod turns a DOKU channel id into a label: VIRTUAL_ACCOUNT_BCA → "Virtual Account BCA".
func paymentMethod(channel string) string {
	if channel == "" {
		return "-"
	}
	words := strings.Fields(strings.ReplaceAll(channel, "_", " "))
	for i, w := range words {
		if len(w) <= 4 { // bank and wallet codes stay upper case: BCA, BRI, QRIS, OVO
			continue
		}
		words[i] = w[:1] + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// receiptText is the plain-text body used when no Brevo template is set.
func receiptText(p map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Halo %s!\n\n", p["GREETING_NAME"])
	fmt.Fprintf(&b, "Pembayaran kamu berhasil. Versi lengkap \"%s\" sudah terbuka.\n\n", p["SONG_TITLE"])
	if u := p["SONG_URL"].(string); u != "" {
		fmt.Fprintf(&b, "Dengarkan & unduh lagu lengkapmu: %s\n\n", u)
	}
	fmt.Fprintf(&b, "No. invoice : %s\n", p["INVOICE"])
	fmt.Fprintf(&b, "Tanggal     : %s\n", p["PAID_AT"])
	fmt.Fprintf(&b, "Metode      : %s\n", p["PAYMENT_METHOD"])
	if d := p["DISCOUNT"].(string); d != "" {
		fmt.Fprintf(&b, "Voucher     : %s (%s)\n", p["VOUCHER_CODE"], d)
	}
	fmt.Fprintf(&b, "Total bayar : %s\n\n", p["AMOUNT"])
	fmt.Fprintf(&b, "Terima kasih,\n%s\n", p["BRAND"])
	return b.String()
}

// receiptHTML is the HTML body used when no Brevo template is set.
func receiptHTML(p map[string]any) string {
	e := func(k string) string { return escapeHTML(fmt.Sprint(p[k])) }
	var b strings.Builder
	b.WriteString("<html><body>")
	b.WriteString("<p>Halo " + e("GREETING_NAME") + "!</p>")
	b.WriteString("<p>Pembayaran kamu berhasil. Versi lengkap <b>" + e("SONG_TITLE") + "</b> sudah terbuka.</p>")
	if p["SONG_URL"].(string) != "" {
		b.WriteString(`<p><a href="` + e("SONG_URL") + `">Dengarkan lagu lengkapmu</a></p>`)
	}
	b.WriteString("<p>No. invoice: " + e("INVOICE") + "<br>Tanggal: " + e("PAID_AT") +
		"<br>Metode: " + e("PAYMENT_METHOD"))
	if p["DISCOUNT"].(string) != "" {
		b.WriteString("<br>Voucher: " + e("VOUCHER_CODE") + " (" + e("DISCOUNT") + ")")
	}
	b.WriteString("<br><b>Total bayar: " + e("AMOUNT") + "</b></p>")
	b.WriteString("<p>Terima kasih,<br>" + e("BRAND") + "</p></body></html>")
	return b.String()
}
