package aiprovider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/audit"
)

// Defaults applied when the admin leaves a field empty.
const (
	defaultMaxRetries    = 2
	defaultLyricsTimeout = 60  // seconds
	defaultMusicTimeout  = 300 // seconds; a full song takes minutes
	testTimeout          = 30 * time.Second
	maxParamsKeys        = 30
)

// Store is the provider storage the admin service needs (GormRepository).
type Store interface {
	Create(ctx context.Context, p AIProvider) (AIProvider, error)
	List(ctx context.Context, c Category) ([]AIProvider, error)
	Get(ctx context.Context, id string) (AIProvider, error)
	Update(ctx context.Context, p AIProvider) (AIProvider, error)
	Activate(ctx context.Context, id string) error
	Deactivate(ctx context.Context, id string) error
	Archive(ctx context.Context, id string) error
}

// Cipher encrypts API keys at rest (secret.Box).
type Cipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(encoded string) (string, error)
}

// Auditor records back-office changes (audit.GormRecorder).
type Auditor interface {
	Record(ctx context.Context, e audit.Entry) error
}

// Adapters looks up the adapter that tests a provider (Registry).
type Adapters interface {
	LyricsStyle(t AdapterType) (LyricsStyleProvider, error)
	Music(t AdapterType) (MusicProvider, error)
}

// Service is the admin side of AI providers: CRUD, activation, priority, test connection.
// Changes apply to new jobs only; the worker picks a provider when a job starts.
type Service struct {
	store    Store
	cipher   Cipher
	audit    Auditor
	adapters Adapters
	catalog  map[AdapterType]AdapterInfo
	order    []AdapterInfo
	now      func() time.Time
}

// NewService builds the service. catalog lists the adapters an admin may pick.
func NewService(store Store, cipher Cipher, auditor Auditor, adapters Adapters, catalog []AdapterInfo) *Service {
	m := make(map[AdapterType]AdapterInfo, len(catalog))
	for _, a := range catalog {
		m[a.Type] = a
	}
	return &Service{store: store, cipher: cipher, audit: auditor, adapters: adapters, catalog: m, order: catalog,
		now: time.Now}
}

// Adapters returns the adapter catalog for the form.
func (s *Service) Adapters() []AdapterOutput {
	out := make([]AdapterOutput, len(s.order))
	for i, a := range s.order {
		out[i] = AdapterOutput(a)
	}
	return out
}

// List returns providers, optionally of one category.
func (s *Service) List(ctx context.Context, in ListProvidersInput) ([]ProviderOutput, error) {
	ps, err := s.store.List(ctx, in.Category)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]ProviderOutput, len(ps))
	for i, p := range ps {
		out[i] = toOutput(p)
	}
	return out, nil
}

// Get returns one provider.
func (s *Service) Get(ctx context.Context, id string) (ProviderOutput, error) {
	p, err := s.get(ctx, id)
	if err != nil {
		return ProviderOutput{}, err
	}
	return toOutput(p), nil
}

// Create adds a provider. With is_active it is switched on right away.
func (s *Service) Create(ctx context.Context, actorID string, in CreateProviderInput) (ProviderOutput, error) {
	info, ok := s.catalog[in.AdapterType]
	if !ok {
		return ProviderOutput{}, fieldErr("adapter_type", "is not a known adapter")
	}
	if info.Category != in.Category {
		return ProviderOutput{}, fieldErr("adapter_type", fmt.Sprintf("serves %s, not %s", info.Category, in.Category))
	}
	if err := checkParams(in.Params, info.Params); err != nil {
		return ProviderOutput{}, err
	}
	key := strings.TrimSpace(in.APIKey)
	enc, err := s.cipher.Encrypt(key)
	if err != nil {
		return ProviderOutput{}, apperror.Internal(err)
	}
	p := AIProvider{
		Category: in.Category, AdapterType: in.AdapterType, Name: strings.TrimSpace(in.Name),
		BaseURL: or(strings.TrimSpace(in.BaseURL), info.DefaultBaseURL), Model: or(strings.TrimSpace(in.Model), info.DefaultModel),
		APIKeyEncrypted: enc, APIKeyLast4: last4(key), Params: in.Params,
		TimeoutSec: in.TimeoutSec, MaxRetries: defaultMaxRetries, Priority: DefaultPriority,
	}
	if in.Priority != nil {
		p.Priority = *in.Priority
	}
	if p.TimeoutSec == 0 {
		p.TimeoutSec = defaultTimeout(in.Category)
	}
	if in.MaxRetries != nil {
		p.MaxRetries = *in.MaxRetries
	}
	if p, err = s.store.Create(ctx, p); err != nil {
		return ProviderOutput{}, apperror.Internal(err)
	}
	s.record(ctx, actorID, "ai_provider.create", p.ID, map[string]any{
		"name": p.Name, "category": p.Category, "adapter_type": p.AdapterType, "model": p.Model, "priority": p.Priority, "api_key": "****" + p.APIKeyLast4,
	})
	if in.IsActive {
		return s.Activate(ctx, actorID, p.ID)
	}
	return toOutput(p), nil
}

// Update changes the fields that are set. Setting api_key replaces (and re-encrypts) the key.
func (s *Service) Update(ctx context.Context, actorID, id string, in UpdateProviderInput) (ProviderOutput, error) {
	p, err := s.get(ctx, id)
	if err != nil {
		return ProviderOutput{}, err
	}
	changed := map[string]any{}
	set := func(field string, dst *string, v *string) {
		if v != nil && strings.TrimSpace(*v) != *dst {
			*dst = strings.TrimSpace(*v)
			changed[field] = *dst
		}
	}
	set("name", &p.Name, in.Name)
	set("base_url", &p.BaseURL, in.BaseURL)
	set("model", &p.Model, in.Model)
	if p.Name == "" {
		return ProviderOutput{}, fieldErr("name", "is required")
	}
	info := s.catalog[p.AdapterType]
	p.BaseURL, p.Model = or(p.BaseURL, info.DefaultBaseURL), or(p.Model, info.DefaultModel)
	if in.APIKey != nil {
		key := strings.TrimSpace(*in.APIKey)
		if p.APIKeyEncrypted, err = s.cipher.Encrypt(key); err != nil {
			return ProviderOutput{}, apperror.Internal(err)
		}
		p.APIKeyLast4 = last4(key)
		changed["api_key"] = "****" + p.APIKeyLast4
	}
	if in.Params != nil {
		if err := checkParams(in.Params, info.Params); err != nil {
			return ProviderOutput{}, err
		}
		p.Params, changed["params"] = in.Params, in.Params
	}
	if in.TimeoutSec != nil && *in.TimeoutSec != p.TimeoutSec {
		p.TimeoutSec, changed["timeout_sec"] = *in.TimeoutSec, *in.TimeoutSec
	}
	if in.MaxRetries != nil && *in.MaxRetries != p.MaxRetries {
		p.MaxRetries, changed["max_retries"] = *in.MaxRetries, *in.MaxRetries
	}
	if in.Priority != nil && *in.Priority != p.Priority {
		p.Priority, changed["priority"] = *in.Priority, *in.Priority
	}
	if len(changed) == 0 {
		return toOutput(p), nil
	}
	if p, err = s.store.Update(ctx, p); err != nil {
		return ProviderOutput{}, mapErr(err)
	}
	s.record(ctx, actorID, "ai_provider.update", p.ID, changed)
	return toOutput(p), nil
}

// Activate switches id on. Other active providers of the category stay on; priority decides.
func (s *Service) Activate(ctx context.Context, actorID, id string) (ProviderOutput, error) {
	p, err := s.get(ctx, id)
	if err != nil {
		return ProviderOutput{}, err
	}
	if err := s.store.Activate(ctx, id); err != nil {
		return ProviderOutput{}, mapErr(err)
	}
	s.record(ctx, actorID, "ai_provider.activate", id, map[string]any{"category": p.Category})
	return s.Get(ctx, id)
}

// Deactivate turns id off. If it was the last active one of its category, new jobs fail until
// another one is activated, so the UI asks for confirmation.
func (s *Service) Deactivate(ctx context.Context, actorID, id string) (ProviderOutput, error) {
	if _, err := s.get(ctx, id); err != nil {
		return ProviderOutput{}, err
	}
	if err := s.store.Deactivate(ctx, id); err != nil {
		return ProviderOutput{}, mapErr(err)
	}
	s.record(ctx, actorID, "ai_provider.deactivate", id, nil)
	return s.Get(ctx, id)
}

// Archive soft-deletes an inactive provider (409 while it is active).
func (s *Service) Archive(ctx context.Context, actorID, id string) error {
	p, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	if p.IsActive {
		return apperror.Conflict("provider masih aktif; nonaktifkan dulu sebelum diarsipkan")
	}
	if err := s.store.Archive(ctx, id); err != nil {
		return mapErr(err)
	}
	s.record(ctx, actorID, "ai_provider.archive", id, nil)
	return nil
}

// TestConnection sends a small request with the stored key and model.
func (s *Service) TestConnection(ctx context.Context, id string) (TestConnectionOutput, error) {
	p, err := s.get(ctx, id)
	if err != nil {
		return TestConnectionOutput{}, err
	}
	key, err := s.cipher.Decrypt(p.APIKeyEncrypted)
	if err != nil {
		return TestConnectionOutput{}, apperror.Internal(fmt.Errorf("decrypt api key of provider %s: %w", id, err))
	}
	cfg := Config{BaseURL: p.BaseURL, Model: p.Model, APIKey: key, Params: p.Params, Timeout: testTimeout}
	var test func(context.Context, Config) error
	switch p.Category {
	case CategoryLyricsStyle:
		a, err := s.adapters.LyricsStyle(p.AdapterType)
		if err != nil {
			return TestConnectionOutput{}, apperror.Internal(err)
		}
		test = a.TestConnection
	case CategoryMusic:
		a, err := s.adapters.Music(p.AdapterType)
		if err != nil {
			return TestConnectionOutput{}, apperror.Internal(err)
		}
		test = a.TestConnection
	default:
		return TestConnectionOutput{}, apperror.Internal(fmt.Errorf("provider %s has unknown category %q", id, p.Category))
	}

	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	start := s.now()
	err = test(ctx, cfg)
	latency := s.now().Sub(start).Milliseconds()
	if err == nil {
		return TestConnectionOutput{OK: true, Message: "Koneksi berhasil: API key dan model diterima.", LatencyMS: latency}, nil
	}
	kind := KindOf(err)
	return TestConnectionOutput{OK: false, Kind: string(kind), Message: testMessage(kind, err), LatencyMS: latency}, nil
}

func (s *Service) get(ctx context.Context, id string) (AIProvider, error) {
	p, err := s.store.Get(ctx, id)
	if err != nil {
		return AIProvider{}, mapErr(err)
	}
	return p, nil
}

// record writes an audit entry. The change is already saved, so a failed audit write is logged
// rather than returned (the admin would otherwise retry a change that already happened).
func (s *Service) record(ctx context.Context, actorID, action, id string, diff map[string]any) {
	err := s.audit.Record(ctx, audit.Entry{ActorType: "admin", ActorID: actorID, Action: action,
		EntityType: "ai_provider", EntityID: id, Diff: diff})
	if err != nil {
		slog.ErrorContext(ctx, "audit log write failed", "action", action, "entity_id", id, "error", err)
	}
}

func testMessage(kind ErrorKind, err error) string {
	switch kind {
	case KindAuth:
		return "API key ditolak provider. Periksa kembali API key-nya."
	case KindInsufficientBalance:
		return "Saldo akun provider tidak cukup. Isi saldo lalu coba lagi."
	case KindRateLimited:
		return "Provider sedang membatasi request (rate limit). Coba lagi sebentar lagi."
	case KindTimeout:
		return "Provider tidak menjawab dalam 30 detik. Periksa base URL atau coba lagi."
	case KindUnavailable:
		return "Provider sedang tidak tersedia. Coba lagi nanti."
	default:
		var pe *Error
		if errors.As(err, &pe) && pe.Message != "" {
			return "Provider menolak request: " + pe.Message
		}
		return "Koneksi gagal. Periksa base URL dan model."
	}
}

func mapErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("provider tidak ditemukan")
	}
	return apperror.Internal(err)
}

func fieldErr(field, msg string) error {
	return apperror.Validation([]apperror.ErrorDetail{{Field: field, Message: msg}})
}

// checkParams bounds the params map and checks the values of the keys the adapter declares
// (type, range, allowed options). Undeclared keys pass through for forward compatibility.
func checkParams(p map[string]any, specs []ParamSpec) error {
	if len(p) > maxParamsKeys {
		return fieldErr("params", fmt.Sprintf("must have at most %d keys", maxParamsKeys))
	}
	for _, spec := range specs {
		v, ok := p[spec.Key]
		if !ok || v == nil {
			continue
		}
		if msg := checkParam(spec, v); msg != "" {
			return fieldErr("params."+spec.Key, msg)
		}
	}
	return nil
}

func checkParam(spec ParamSpec, v any) string {
	var num float64
	switch spec.Kind {
	case ParamString, ParamText:
		s, ok := v.(string)
		if !ok {
			return "must be text"
		}
		if len(spec.Options) > 0 && !slices.Contains(spec.Options, s) {
			return "must be one of " + strings.Join(spec.Options, ", ")
		}
		return ""
	case ParamInt, ParamFloat:
		f, ok := v.(float64) // JSON numbers decode as float64
		if !ok {
			return "must be a number"
		}
		if spec.Kind == ParamInt && f != math.Trunc(f) {
			return "must be a whole number"
		}
		num = f
	default:
		return ""
	}
	if len(spec.Options) > 0 && !slices.Contains(spec.Options, strconv.FormatFloat(num, 'f', -1, 64)) {
		return "must be one of " + strings.Join(spec.Options, ", ")
	}
	if spec.Min != nil && num < *spec.Min {
		return fmt.Sprintf("must be at least %g", *spec.Min)
	}
	if spec.Max != nil && num > *spec.Max {
		return fmt.Sprintf("must be at most %g", *spec.Max)
	}
	return ""
}

func defaultTimeout(c Category) int {
	if c == CategoryMusic {
		return defaultMusicTimeout
	}
	return defaultLyricsTimeout
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// last4 is the masked-display suffix. Keys are ASCII; anything shorter than 8 shows nothing.
func last4(key string) string {
	if len(key) < 8 {
		return ""
	}
	return key[len(key)-4:]
}
