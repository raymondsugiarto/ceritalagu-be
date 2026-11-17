// Package masterdata holds the admin-managed option lists of the questionnaire (PRD §6.3):
// recipient types (step 1), occasions (step 3), genres and moods (step 4). Each row has a code (what the
// questionnaire stores), a customer-facing name, and a hint that is added to the lyrics prompt.
package masterdata

import (
	"slices"
	"time"
)

// Kind is one option list. Each kind has its own table.
type Kind string

// Kinds.
const (
	KindRecipientType Kind = "recipient_type"
	KindOccasion      Kind = "occasion"
	KindGenre         Kind = "genre"
	KindMood          Kind = "mood"
)

// Kinds lists every kind in questionnaire order.
var Kinds = []Kind{KindRecipientType, KindOccasion, KindGenre, KindMood}

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return slices.Contains(Kinds, k) }

// OtherCode is reserved: choosing it in the questionnaire means "Lainnya" with free text.
const OtherCode = "other"

// Item is one option row.
type Item struct {
	ID          string
	Kind        Kind
	Code        string
	Name        string
	Description string // genre only
	Icon        string // genre only (lucide icon name or emoji)
	Hint        string // genre.style_hint, occasion/recipient_type/mood.prompt_hint
	SortOrder   int
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
