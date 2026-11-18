package apperror

// Reasons are stable, machine-readable causes sent as the optional `reason` of an error response
// (and of a field error). They let clients show specific, localized text where Code alone is too
// generic (several different 400s or 409s on one endpoint). Values are snake_case and never change
// once shipped; add new ones instead of renaming.
const (
	// Auth (customer sign-in).
	ReasonEmailNotRegistered  = "email_not_registered"
	ReasonAccountSuspended    = "account_suspended"
	ReasonInvalidCredentials  = "invalid_credentials"
	ReasonOTPInvalid          = "otp_invalid"
	ReasonOTPExpired          = "otp_expired"
	ReasonOTPTooManyAttempts  = "otp_too_many_attempts"
	ReasonOTPSendFailed       = "otp_send_failed"
	ReasonSetupSessionExpired = "setup_session_expired"
	ReasonPasswordAlreadySet  = "password_already_set"
	ReasonPasswordTooShort    = "password_too_short"
	ReasonPasswordTooLong     = "password_too_long"
	ReasonPasswordBlank       = "password_blank"
	ReasonLoginRequired       = "login_required"
	ReasonTooManyAttempts     = "too_many_attempts"

	// Vouchers and payment.
	ReasonVoucherRequired       = "voucher_required"
	ReasonVoucherNotFound       = "voucher_not_found"
	ReasonVoucherInactive       = "voucher_inactive"
	ReasonVoucherExpired        = "voucher_expired"
	ReasonVoucherAlreadyUsed    = "voucher_already_used"
	ReasonVoucherQuotaExhausted = "voucher_quota_exhausted"
	ReasonPaymentUnavailable    = "payment_unavailable"
	ReasonCheckoutUnavailable   = "checkout_unavailable"
	ReasonSongAlreadyUnlocked   = "song_already_unlocked"
	ReasonContactDetailsMissing = "contact_details_missing"
	ReasonAccountDisabled       = "account_disabled"

	// Song generation.
	ReasonDailyQuotaExceeded     = "daily_quota_exceeded"
	ReasonRegenerateLimitReached = "regenerate_limit_reached"
	ReasonJobInProgress          = "job_in_progress"
	ReasonGenerationUnavailable  = "generation_unavailable"
	ReasonFullSongLocked         = "full_song_locked"

	// Field errors (validator tags and questionnaire option checks).
	ReasonRequired          = "required"
	ReasonInvalidEmail      = "invalid_email"
	ReasonBelowMin          = "below_min"
	ReasonAboveMax          = "above_max"
	ReasonInvalidChoice     = "invalid_choice"
	ReasonInvalidFormat     = "invalid_format"
	ReasonOptionUnavailable = "option_unavailable"

	// Contact form.
	ReasonContactFieldsMissing = "contact_fields_missing"
	ReasonContactSendFailed    = "contact_send_failed"
	ReasonTooManyMessages      = "too_many_messages"
)
