package song

import "github.com/yourorg/music-api/internal/module/aiprovider"

// UserMessage is the customer-facing text for a failed job's ErrorCode (PRD §5.2). Internal
// details stay in Job.ErrorMessage for admins.
func UserMessage(code string) string {
	switch aiprovider.ErrorKind(code) {
	case aiprovider.KindContentFlagged:
		return "Maaf, beberapa jawabanmu tidak bisa kami proses. Coba ubah kata-katanya lalu buat ulang."
	case aiprovider.KindRateLimited, aiprovider.KindTimeout, aiprovider.KindUnavailable, aiprovider.KindInvalidOutput:
		return "Server kami sedang sibuk. Silakan coba lagi beberapa saat lagi."
	default:
		return "Maaf, terjadi kendala saat membuat lagumu. Silakan coba lagi."
	}
}
