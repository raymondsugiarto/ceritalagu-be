package aiprovider

import "time"

// ListProvidersInput filters the provider list.
type ListProvidersInput struct {
	Category Category `query:"category" validate:"omitempty,oneof=lyrics_style music"`
}

// CreateProviderInput is the "add provider" form. Empty base_url / model use the adapter defaults.
type CreateProviderInput struct {
	Category    Category       `json:"category" validate:"required,oneof=lyrics_style music"`
	AdapterType AdapterType    `json:"adapter_type" validate:"required,max=50"`
	Name        string         `json:"name" validate:"required,max=100"`
	BaseURL     string         `json:"base_url" validate:"omitempty,http_url,max=500"`
	Model       string         `json:"model" validate:"max=100"`
	APIKey      string         `json:"api_key" validate:"required,min=8,max=1000"`
	Params      map[string]any `json:"params"`
	TimeoutSec  int            `json:"timeout_sec" validate:"omitempty,min=5,max=900"`
	MaxRetries  *int           `json:"max_retries" validate:"omitempty,min=0,max=5"`
	Priority    *int           `json:"priority" validate:"omitempty,min=1,max=100"` // empty = 1
	IsActive    bool           `json:"is_active"`
}

// UpdateProviderInput changes only the fields that are set. api_key is set only to replace the key.
type UpdateProviderInput struct {
	Name       *string        `json:"name" validate:"omitempty,min=1,max=100"`
	BaseURL    *string        `json:"base_url" validate:"omitempty,http_url,max=500"`
	Model      *string        `json:"model" validate:"omitempty,max=100"`
	APIKey     *string        `json:"api_key" validate:"omitempty,min=8,max=1000"`
	Params     map[string]any `json:"params"`
	TimeoutSec *int           `json:"timeout_sec" validate:"omitempty,min=5,max=900"`
	MaxRetries *int           `json:"max_retries" validate:"omitempty,min=0,max=5"`
	Priority   *int           `json:"priority" validate:"omitempty,min=1,max=100"`
}

// ProviderOutput is a provider as the admin sees it. The API key is only ever shown masked.
type ProviderOutput struct {
	ID           string         `json:"id"`
	Category     Category       `json:"category"`
	AdapterType  AdapterType    `json:"adapter_type"`
	Name         string         `json:"name"`
	BaseURL      string         `json:"base_url"`
	Model        string         `json:"model"`
	APIKeyMasked string         `json:"api_key_masked"`
	Params       map[string]any `json:"params"`
	TimeoutSec   int            `json:"timeout_sec"`
	MaxRetries   int            `json:"max_retries"`
	Priority     int            `json:"priority"`
	IsActive     bool           `json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// TestConnectionOutput is the result of "Test Connection". A failing provider is still a 200:
// the request worked, the provider said no.
type TestConnectionOutput struct {
	OK        bool   `json:"ok"`
	Kind      string `json:"kind,omitempty"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
}

// AdapterOutput is one entry of the adapter catalog for the form.
type AdapterOutput struct {
	Type           AdapterType `json:"type"`
	Category       Category    `json:"category"`
	Label          string      `json:"label"`
	Help           string      `json:"help,omitempty"`
	DefaultBaseURL string      `json:"default_base_url"`
	DefaultModel   string      `json:"default_model"`
	Models         []string    `json:"models"`
	Params         []ParamSpec `json:"params"`
}

func toOutput(p AIProvider) ProviderOutput {
	masked := ""
	if p.APIKeyLast4 != "" {
		masked = "****" + p.APIKeyLast4
	}
	params := p.Params
	if params == nil {
		params = map[string]any{}
	}
	return ProviderOutput{
		ID: p.ID, Category: p.Category, AdapterType: p.AdapterType, Name: p.Name, BaseURL: p.BaseURL,
		Model: p.Model, APIKeyMasked: masked, Params: params, TimeoutSec: p.TimeoutSec, MaxRetries: p.MaxRetries,
		Priority: p.Priority, IsActive: p.IsActive, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}
