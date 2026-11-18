// Package masterdata holds the admin-managed option lists of the questionnaire (PRD §6.3):
// recipient types (step 1), occasions (step 3), genres and moods (step 4). Each row has a code (what the
// questionnaire stores), a customer-facing name, and a hint that is added to the lyrics prompt.
package masterdata

import (
	"slices"
	"time"

	"github.com/yourorg/music-api/internal/i18n"
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
	// Translations holds the non-default locales of Name/Description (table translation), keyed
	// by locale. Name and Description above are the default-locale (id) text and the fallback.
	Translations map[i18n.Locale]Text
}

// Text is the translatable part of an option. Hints are AI-facing and never translated (§4.4).
type Text struct {
	Name        string
	Description string
}

// Translatable fields (translation.field).
const (
	FieldName        = "name"
	FieldDescription = "description"
)

// Localized returns the option's name and description for locale l with the fallback order
// requested locale → base column (id) → code (§4.5), plus the locale the name actually came from.
// Description falls back to the base description only (never to the code).
func (it Item) Localized(l i18n.Locale) (name, description string, used i18n.Locale) {
	name, description, used = it.Name, it.Description, i18n.Default
	if l != i18n.Default {
		if tr, ok := it.Translations[l]; ok {
			if tr.Name != "" {
				name, used = tr.Name, l
			}
			if tr.Description != "" {
				description = tr.Description
			}
		}
	}
	if name == "" {
		name = it.Code
	}
	return name, description, used
}
