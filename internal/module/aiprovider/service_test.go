package aiprovider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/module/audit"
)

type memStore struct {
	rows map[string]AIProvider
	n    int
}

func (m *memStore) Create(_ context.Context, p AIProvider) (AIProvider, error) {
	m.n++
	p.ID = "00000000-0000-0000-0000-00000000000" + string(rune('0'+m.n))
	m.rows[p.ID] = p
	return p, nil
}

func (m *memStore) List(_ context.Context, c Category) ([]AIProvider, error) {
	var out []AIProvider
	for _, p := range m.rows {
		if c == "" || p.Category == c {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *memStore) Get(_ context.Context, id string) (AIProvider, error) {
	p, ok := m.rows[id]
	if !ok {
		return AIProvider{}, ErrNotFound
	}
	return p, nil
}

func (m *memStore) Update(_ context.Context, p AIProvider) (AIProvider, error) {
	m.rows[p.ID] = p
	return p, nil
}

func (m *memStore) Activate(_ context.Context, id string) error {
	p := m.rows[id]
	p.IsActive = true
	m.rows[id] = p
	return nil
}

func (m *memStore) Deactivate(_ context.Context, id string) error {
	p := m.rows[id]
	p.IsActive = false
	m.rows[id] = p
	return nil
}

func (m *memStore) Archive(_ context.Context, id string) error {
	delete(m.rows, id) // in-memory stand-in for the soft delete
	return nil
}

type rot struct{}

func (rot) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (rot) Decrypt(e string) (string, error) { return strings.TrimPrefix(e, "enc:"), nil }

type auditLog struct{ actions []string }

func (a *auditLog) Record(_ context.Context, e audit.Entry) error {
	for _, v := range e.Diff {
		if s, ok := v.(string); ok && strings.Contains(s, "sk-secret") {
			return errors.New("secret leaked into audit diff")
		}
	}
	a.actions = append(a.actions, e.Action)
	return nil
}

type fakeAdapter struct {
	err     error
	lastKey string
}

func (f *fakeAdapter) GenerateLyricsStyle(context.Context, LyricsStyleRequest, Config) (LyricsStyleResult, error) {
	return LyricsStyleResult{}, nil
}

func (f *fakeAdapter) GenerateMusic(context.Context, MusicRequest, Config) (MusicResult, error) {
	return MusicResult{}, nil
}

func (f *fakeAdapter) TestConnection(_ context.Context, cfg Config) error {
	f.lastKey = cfg.APIKey
	return f.err
}

var catalog = []AdapterInfo{
	{Type: AdapterMiniMaxText, Category: CategoryLyricsStyle, DefaultBaseURL: "https://api.example", DefaultModel: "text-1"},
	{Type: AdapterMiniMaxMusic, Category: CategoryMusic, DefaultBaseURL: "https://api.example", DefaultModel: "music-1",
		Params: []ParamSpec{
			{Key: "format", Kind: ParamString, Options: []string{"mp3", "wav"}},
			{Key: "duration", Kind: ParamInt, Min: Bound(10), Max: Bound(360)},
			{Key: "rate", Kind: ParamInt, Options: []string{"44100"}},
		}},
}

func TestParamsCheckedAgainstAdapterSpec(t *testing.T) {
	svc, _, _, _ := newSvc()
	ctx := context.Background()
	mk := func(params map[string]any) error {
		_, err := svc.Create(ctx, "a", CreateProviderInput{Category: CategoryMusic, AdapterType: AdapterMiniMaxMusic,
			Name: "p", APIKey: "key-12345678", Params: params})
		return err
	}
	if err := mk(map[string]any{"format": "wav", "duration": float64(120), "rate": float64(44100), "other": "x"}); err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}
	for _, bad := range []map[string]any{
		{"format": "ogg"}, {"duration": float64(5)}, {"duration": 12.5}, {"duration": "120"}, {"rate": float64(8000)},
	} {
		if err := mk(bad); err == nil || !strings.Contains(err.Error(), "VALIDATION") {
			t.Fatalf("params %v: expected validation error, got %v", bad, err)
		}
	}
}

func newSvc() (*Service, *memStore, *auditLog, *fakeAdapter) {
	store, log, ad := &memStore{rows: map[string]AIProvider{}}, &auditLog{}, &fakeAdapter{}
	reg := NewRegistry()
	reg.RegisterLyricsStyle(AdapterMiniMaxText, ad)
	reg.RegisterMusic(AdapterMiniMaxMusic, ad)
	return NewService(store, rot{}, log, reg, catalog), store, log, ad
}

func TestCreateEncryptsMasksAndDefaults(t *testing.T) {
	svc, store, log, _ := newSvc()
	ctx := context.Background()
	out, err := svc.Create(ctx, "admin1", CreateProviderInput{Category: CategoryMusic, AdapterType: AdapterMiniMaxMusic,
		Name: " MiniMax ", APIKey: "sk-secret-abcd", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	row := store.rows[out.ID]
	if row.APIKeyEncrypted != "enc:sk-secret-abcd" || out.APIKeyMasked != "****abcd" {
		t.Fatalf("key: %q masked %q", row.APIKeyEncrypted, out.APIKeyMasked)
	}
	if out.Name != "MiniMax" || out.Model != "music-1" || out.BaseURL != "https://api.example" ||
		out.TimeoutSec != defaultMusicTimeout || out.MaxRetries != defaultMaxRetries || !out.IsActive {
		t.Fatalf("defaults: %+v", out)
	}
	if strings.Join(log.actions, ",") != "ai_provider.create,ai_provider.activate" {
		t.Fatalf("audit: %v", log.actions)
	}

	for _, in := range []CreateProviderInput{
		{Category: CategoryMusic, AdapterType: AdapterMiniMaxText, Name: "x", APIKey: "12345678"}, // wrong category
		{Category: CategoryMusic, AdapterType: "openai", Name: "x", APIKey: "12345678"},           // unknown adapter
	} {
		if _, err := svc.Create(ctx, "admin1", in); err == nil || !strings.Contains(err.Error(), "VALIDATION") {
			t.Fatalf("expected validation error, got %v", err)
		}
	}
}

func TestSeveralActivePerCategoryAndArchive(t *testing.T) {
	svc, _, _, _ := newSvc()
	ctx := context.Background()
	mk := func(c Category, a AdapterType, active bool) ProviderOutput {
		out, err := svc.Create(ctx, "a", CreateProviderInput{Category: c, AdapterType: a, Name: "p", APIKey: "key-12345678", IsActive: active})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	m1, m2, txt := mk(CategoryMusic, AdapterMiniMaxMusic, true), mk(CategoryMusic, AdapterMiniMaxMusic, false), mk(CategoryLyricsStyle, AdapterMiniMaxText, true)
	if _, err := svc.Activate(ctx, "a", m2.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.List(ctx, ListProvidersInput{})
	active := map[string]bool{}
	for _, p := range list {
		active[p.ID] = p.IsActive
	}
	if !active[m1.ID] || !active[m2.ID] || !active[txt.ID] {
		t.Fatalf("active flags: %v", active)
	}
	if err := svc.Archive(ctx, "a", m2.ID); err == nil || !strings.Contains(err.Error(), "CONFLICT") {
		t.Fatalf("archive active: %v", err)
	}
	if _, err := svc.Deactivate(ctx, "a", m1.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Archive(ctx, "a", m1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, m1.ID); err == nil || !strings.Contains(err.Error(), "NOT_FOUND") {
		t.Fatalf("get archived: %v", err)
	}
}

func TestPriorityDefaultsAndUpdates(t *testing.T) {
	svc, _, log, _ := newSvc()
	ctx := context.Background()
	p, err := svc.Create(ctx, "a", CreateProviderInput{Category: CategoryMusic, AdapterType: AdapterMiniMaxMusic, Name: "p", APIKey: "key-12345678"})
	if err != nil || p.Priority != DefaultPriority {
		t.Fatalf("default priority: %v %+v", err, p)
	}
	three := 3
	if p, _ = svc.Update(ctx, "a", p.ID, UpdateProviderInput{Priority: &three}); p.Priority != 3 {
		t.Fatalf("updated priority: %+v", p)
	}
	if log.actions[len(log.actions)-1] != "ai_provider.update" {
		t.Fatalf("audit: %v", log.actions)
	}
	two := 2
	q, _ := svc.Create(ctx, "a", CreateProviderInput{Category: CategoryMusic, AdapterType: AdapterMiniMaxMusic, Name: "q", APIKey: "key-12345678", Priority: &two})
	if q.Priority != 2 {
		t.Fatalf("create priority: %+v", q)
	}
}

func TestPickByPriority(t *testing.T) {
	ps := []AIProvider{{ID: "a", Priority: 2}, {ID: "b", Priority: 1}, {ID: "c", Priority: 3}, {ID: "d", Priority: 1}}
	seen := map[string]bool{}
	for i := range 2 {
		p, ok := PickByPriority(ps, func(n int) int {
			if n != 2 {
				t.Fatalf("tie group size %d, want 2", n)
			}
			return i
		})
		if !ok {
			t.Fatal("no pick")
		}
		seen[p.ID] = true
	}
	if !seen["b"] || !seen["d"] || len(seen) != 2 {
		t.Fatalf("picked %v, want b and d only", seen)
	}
	if _, ok := PickByPriority(nil, func(int) int { return 0 }); ok {
		t.Fatal("picked from empty list")
	}
}

func TestUpdateReplacesKeyOnlyWhenSet(t *testing.T) {
	svc, store, _, _ := newSvc()
	ctx := context.Background()
	p, _ := svc.Create(ctx, "a", CreateProviderInput{Category: CategoryLyricsStyle, AdapterType: AdapterMiniMaxText, Name: "p", APIKey: "old-key-1111"})

	name := "renamed"
	out, err := svc.Update(ctx, "a", p.ID, UpdateProviderInput{Name: &name})
	if err != nil || out.Name != "renamed" || store.rows[p.ID].APIKeyEncrypted != "enc:old-key-1111" {
		t.Fatalf("rename: %v %+v", err, out)
	}
	key, empty := "sk-secret-2222", ""
	if out, _ = svc.Update(ctx, "a", p.ID, UpdateProviderInput{APIKey: &key}); out.APIKeyMasked != "****2222" {
		t.Fatalf("new key: %+v", out)
	}
	if _, err := svc.Update(ctx, "a", p.ID, UpdateProviderInput{Name: &empty}); err == nil {
		t.Fatal("empty name accepted")
	}
}

func TestConnectionResult(t *testing.T) {
	svc, _, _, ad := newSvc()
	ctx := context.Background()
	p, _ := svc.Create(ctx, "a", CreateProviderInput{Category: CategoryMusic, AdapterType: AdapterMiniMaxMusic, Name: "p", APIKey: "sk-secret-9999"})

	out, err := svc.TestConnection(ctx, p.ID)
	if err != nil || !out.OK || ad.lastKey != "sk-secret-9999" {
		t.Fatalf("ok: %v %+v key=%q", err, out, ad.lastKey)
	}
	ad.err = &Error{Kind: KindAuth, Provider: "minimax", ProviderCode: 1004}
	if out, err = svc.TestConnection(ctx, p.ID); err != nil || out.OK || out.Kind != "auth" || !strings.Contains(out.Message, "API key") {
		t.Fatalf("auth: %v %+v", err, out)
	}
}
