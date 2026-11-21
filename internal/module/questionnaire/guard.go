package questionnaire

import (
	"context"
	"log/slog"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/inputguard"
	"github.com/yourorg/music-api/internal/module/guest"
)

// msgInputNotAllowed is the customer-facing fallback; clients show their own localized text for
// apperror.ReasonInputNotAllowed. It never says which rule matched.
const msgInputNotAllowed = "Isian ini berisi hal yang tidak bisa kami proses. Ceritakan saja tentang dia dan kenanganmu, ya."

// screenAnswers rejects free-text answers that ask about the AI or technology behind the product,
// try to command the system or the AI, or carry malicious or harmful content (inputguard). Every
// offending field is reported so the form can mark them all at once.
func screenAnswers(ctx context.Context, v guest.Visitor, a Answers) error {
	fields := []struct{ name, value string }{
		{"recipient_name", a.RecipientName}, {"recipient_nickname", a.RecipientNickname},
		{"recipient_other", a.RecipientOther}, {"occasion_other", a.OccasionOther},
		{"special", a.Special}, {"memory", a.Memory}, {"message", a.Message}, {"mood", a.Mood},
	}
	var details []apperror.ErrorDetail
	for _, f := range fields {
		viol, bad := inputguard.Check(f.value)
		if !bad {
			continue
		}
		slog.WarnContext(ctx, "questionnaire input rejected", "field", f.name, "category", viol.Category,
			"match", viol.Match, "guest_session_id", v.SessionID, "ip_hash", v.IPHash)
		details = append(details, apperror.ErrorDetail{Field: f.name, Message: msgInputNotAllowed, Reason: apperror.ReasonInputNotAllowed})
	}
	if len(details) > 0 {
		return apperror.Validation(details)
	}
	return nil
}
