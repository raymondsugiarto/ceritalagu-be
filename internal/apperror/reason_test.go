package apperror

import "testing"

func TestWithReasonCopies(t *testing.T) {
	t.Parallel()
	base := BadRequest("Kode voucher tidak ditemukan.")
	got := base.WithReason(ReasonVoucherNotFound)
	if got.Reason != ReasonVoucherNotFound || got.Code != CodeBadRequest || got.Message != base.Message {
		t.Fatalf("WithReason: %+v", got)
	}
	if base.Reason != "" {
		t.Fatal("WithReason mutated the shared error")
	}
}
