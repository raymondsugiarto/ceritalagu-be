package masterdata

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/module/audit"
)

type memStore struct {
	rows     map[string]Item
	archived map[string]bool // kind:code of soft-deleted rows
}

func (m *memStore) List(_ context.Context, k Kind, activeOnly bool) ([]Item, error) {
	var out []Item
	for _, it := range m.rows {
		if it.Kind == k && (!activeOnly || it.IsActive) {
			out = append(out, it)
		}
	}
	return out, nil
}

func (m *memStore) Get(_ context.Context, k Kind, id string) (Item, error) {
	it, ok := m.rows[id]
	if !ok || it.Kind != k {
		return Item{}, ErrNotFound
	}
	return it, nil
}

func (m *memStore) ByCode(_ context.Context, k Kind, code string) (Item, error) {
	for _, it := range m.rows {
		if it.Kind == k && it.Code == code {
			return it, nil
		}
	}
	return Item{}, ErrNotFound
}

func (m *memStore) Create(_ context.Context, it Item) (Item, error) {
	it.ID = string(it.Kind) + ":" + it.Code
	m.rows[it.ID] = it
	return it, nil
}

func (m *memStore) Update(_ context.Context, it Item) (Item, error) {
	m.rows[it.ID] = it
	return it, nil
}

func (m *memStore) Archive(_ context.Context, _ Kind, id string) error {
	m.archived[id] = true // IDs are kind:code
	delete(m.rows, id)    // stand-in for the soft delete
	return nil
}

func (m *memStore) CodeEverUsed(ctx context.Context, k Kind, code string) (bool, error) {
	if m.archived[string(k)+":"+code] {
		return true, nil
	}
	_, err := m.ByCode(ctx, k, code)
	return err == nil, nil
}

type noAudit struct{}

func (noAudit) Record(context.Context, audit.Entry) error { return nil }

func newSvc() (*Service, *memStore) {
	st := &memStore{rows: map[string]Item{}, archived: map[string]bool{}}
	return NewService(st, noAudit{}), st
}

func TestCreateRules(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "a", KindGenre, CreateItemInput{Code: "city_pop", Name: "City Pop", Icon: "🎷"}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		kind Kind
		code string
		want string
	}{
		"duplicate":    {KindGenre, "city_pop", "CONFLICT"},
		"bad format":   {KindGenre, "City Pop", "VALIDATION"},
		"reserved":     {KindOccasion, "other", "VALIDATION"},
		"unknown kind": {"tempo", "fast", "NOT_FOUND"},
	}
	for name, c := range cases {
		if _, err := svc.Create(ctx, "a", c.kind, CreateItemInput{Code: c.code, Name: "x"}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %s, got %v", name, c.want, err)
		}
	}
}

func TestOptionsShowOnlyActiveAndCodeCheck(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	on, off := true, false
	pop, _ := svc.Create(ctx, "a", KindGenre, CreateItemInput{Code: "pop", Name: "Pop", Hint: "secret prompt hint"})
	_, _ = svc.Create(ctx, "a", KindGenre, CreateItemInput{Code: "jazz", Name: "Jazz", IsActive: &off})

	opts, err := svc.QuestionnaireOptions(ctx)
	if err != nil || len(opts.Genres) != 1 || opts.Genres[0].Code != "pop" || opts.RecipientTypes == nil {
		t.Fatalf("options: %v %+v", err, opts)
	}
	if ok, _ := svc.IsActiveCode(ctx, KindGenre, "jazz"); ok {
		t.Fatal("inactive code accepted")
	}
	if _, err := svc.Update(ctx, "a", KindGenre, pop.ID, UpdateItemInput{IsActive: &off}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.IsActiveCode(ctx, KindGenre, "pop"); ok {
		t.Fatal("deactivated code still accepted")
	}
	if _, err := svc.Update(ctx, "a", KindGenre, pop.ID, UpdateItemInput{IsActive: &on}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.IsActiveCode(ctx, KindGenre, "pop"); !ok {
		t.Fatal("reactivated code rejected")
	}
}

func TestImportDefaultsIsIdempotent(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	n, err := svc.ImportDefaults(ctx)
	if err != nil || n != len(Defaults()) {
		t.Fatalf("first import: %d %v", n, err)
	}
	name := "Pop Indonesia"
	_, _ = svc.Update(ctx, "a", KindGenre, "genre:pop", UpdateItemInput{Name: &name})
	if err := svc.Archive(ctx, "a", KindGenre, "genre:jazz"); err != nil {
		t.Fatal(err)
	}
	if n, err = svc.ImportDefaults(ctx); err != nil || n != 0 {
		t.Fatalf("second import: %d %v", n, err)
	}
	if st.rows["genre:pop"].Name != name {
		t.Fatal("import overwrote an admin edit")
	}
	for _, it := range Defaults() {
		if !codePattern.MatchString(it.Code) || it.Code == OtherCode {
			t.Errorf("default code %q breaks the code rules", it.Code)
		}
	}
}
