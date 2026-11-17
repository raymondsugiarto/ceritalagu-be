package masterdata

import "time"

// CreateItemInput is the admin "add option" form. Code is fixed after creation.
type CreateItemInput struct {
	Code        string `json:"code" validate:"required,min=2,max=50"`
	Name        string `json:"name" validate:"required,max=100"`
	Description string `json:"description" validate:"max=500"`
	Icon        string `json:"icon" validate:"max=100"`
	Hint        string `json:"hint" validate:"max=1000"`
	SortOrder   int    `json:"sort_order" validate:"min=0,max=10000"`
	IsActive    *bool  `json:"is_active"` // default true
}

// UpdateItemInput changes only the fields that are set.
type UpdateItemInput struct {
	Name        *string `json:"name" validate:"omitempty,min=1,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
	Icon        *string `json:"icon" validate:"omitempty,max=100"`
	Hint        *string `json:"hint" validate:"omitempty,max=1000"`
	SortOrder   *int    `json:"sort_order" validate:"omitempty,min=0,max=10000"`
	IsActive    *bool   `json:"is_active"`
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
}

// OptionOutput is one public questionnaire option (no prompt hint: that stays internal).
type OptionOutput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

// QuestionnaireOptionsOutput is GET /questionnaire-options: the active options per step.
type QuestionnaireOptionsOutput struct {
	RecipientTypes []OptionOutput `json:"recipient_types"`
	Occasions      []OptionOutput `json:"occasions"`
	Genres         []OptionOutput `json:"genres"`
	Moods          []OptionOutput `json:"moods"`
}

func toItemOutput(it Item) ItemOutput {
	return ItemOutput(it)
}

func toOptions(items []Item) []OptionOutput {
	out := make([]OptionOutput, len(items))
	for i, it := range items {
		out[i] = OptionOutput{Code: it.Code, Name: it.Name, Description: it.Description, Icon: it.Icon}
	}
	return out
}
