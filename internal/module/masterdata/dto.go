package masterdata

import (
	"time"

	"github.com/yourorg/music-api/internal/i18n"
)

// TranslationInput is one locale's text in an admin create/update. A nil field is left as it is;
// an empty string clears that translation (the option falls back to the base name/description).
type TranslationInput struct {
	Name        *string `json:"name" validate:"omitempty,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
}

// TranslationOutput is one locale's text as the admin sees it ("" = not filled yet).
type TranslationOutput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateItemInput is the admin "add option" form. Code is fixed after creation.
type CreateItemInput struct {
	Code        string `json:"code" validate:"required,min=2,max=50"`
	Name        string `json:"name" validate:"required,max=100"`
	Description string `json:"description" validate:"max=500"`
	Icon        string `json:"icon" validate:"max=100"`
	Hint        string `json:"hint" validate:"max=1000"`
	SortOrder   int    `json:"sort_order" validate:"min=0,max=10000"`
	IsActive    *bool  `json:"is_active"` // default true
	// Translations per non-default locale, e.g. {"en": {"name": "Mom"}} (i18n plan §4.7).
	Translations map[string]TranslationInput `json:"translations" validate:"omitempty,dive"`
}

// UpdateItemInput changes only the fields that are set.
type UpdateItemInput struct {
	Name        *string `json:"name" validate:"omitempty,min=1,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
	Icon        *string `json:"icon" validate:"omitempty,max=100"`
	Hint        *string `json:"hint" validate:"omitempty,max=1000"`
	SortOrder   *int    `json:"sort_order" validate:"omitempty,min=0,max=10000"`
	IsActive    *bool   `json:"is_active"`
	// Translations per non-default locale; only the locales and fields sent change.
	Translations map[string]TranslationInput `json:"translations" validate:"omitempty,dive"`
}

// ItemOutput is a row as the admin sees it (with the prompt hint).
type ItemOutput struct {
	ID          string    `json:"id"`
	Kind        Kind      `json:"kind"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	Hint        string    `json:"hint"`
	SortOrder   int       `json:"sort_order"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Translations has every non-default supported locale, e.g. {"en": {"name": "Mom", ...}};
	// empty strings mean "not filled yet" (admin badge "EN belum diisi").
	Translations map[string]TranslationOutput `json:"translations"`
}

// OptionOutput is one public questionnaire option (no prompt hint: that stays internal). Name and
// Description are resolved for the request locale; Locale is the locale the name came from
// (differs from the requested one when it fell back to the base text, §4.5).
type OptionOutput struct {
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Icon        string      `json:"icon,omitempty"`
	Locale      i18n.Locale `json:"locale"`
}

// QuestionnaireOptionsOutput is GET /questionnaire-options: the active options per step.
type QuestionnaireOptionsOutput struct {
	RecipientTypes []OptionOutput `json:"recipient_types"`
	Occasions      []OptionOutput `json:"occasions"`
	Genres         []OptionOutput `json:"genres"`
	Moods          []OptionOutput `json:"moods"`
}

func toItemOutput(it Item) ItemOutput {
	tr := map[string]TranslationOutput{}
	for _, l := range i18n.Supported {
		if l == i18n.Default {
			continue
		}
		t := it.Translations[l]
		tr[string(l)] = TranslationOutput{Name: t.Name, Description: t.Description}
	}
	return ItemOutput{ID: it.ID, Kind: it.Kind, Code: it.Code, Name: it.Name, Description: it.Description,
		Icon: it.Icon, Hint: it.Hint, SortOrder: it.SortOrder, IsActive: it.IsActive, CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt, Translations: tr}
}

func toOptions(items []Item, l i18n.Locale) []OptionOutput {
	out := make([]OptionOutput, len(items))
	for i, it := range items {
		name, desc, used := it.Localized(l)
		out[i] = OptionOutput{Code: it.Code, Name: name, Description: desc, Icon: it.Icon, Locale: used}
	}
	return out
}
