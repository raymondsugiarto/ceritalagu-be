package masterdata

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/i18n"
	"github.com/yourorg/music-api/internal/module/audit"
)

// trKey is one live translation (entity, locale, field); the map value is the text.
type trKey struct {
	kind          Kind
	entity, field string
	locale        i18n.Locale
}

type memStore struct {
	rows      map[string]Item
	archived  map[string]bool // kind:code of soft-deleted rows
	tr        map[trKey]string
	trWrites  int // SetTranslation calls that changed a row
	trQueries int
}

func (m *memStore) Translations(_ context.Context, k Kind) (map[string]map[i18n.Locale]Text, error) {
	m.trQueries++
	out := map[string]map[i18n.Locale]Text{}
	for key, v := range m.tr {
		if key.kind != k {
			continue
		}
		if out[key.entity] == nil {
			out[key.entity] = map[i18n.Locale]Text{}
		}
		t := out[key.entity][key.locale]
		if key.field == FieldName {
			t.Name = v
		} else {
			t.Description = v
		}
		out[key.entity][key.locale] = t
	}
	return out, nil
}

func (m *memStore) SetTranslation(_ context.Context, k Kind, id string, l i18n.Locale, field, value string) error {
	key := trKey{k, id, field, l}
	m.trWrites++
	if value == "" {
		delete(m.tr, key) // stand-in for the soft delete
		return nil
	}
	m.tr[key] = value // one live row per key: the upsert
	return nil
}

func (m *memStore) ArchiveTranslations(_ context.Context, k Kind, id string) error {
	for key := range m.tr {
		if key.kind == k && key.entity == id {
			delete(m.tr, key)
		}
	}
	return nil
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

type memAudit struct{ entries []audit.Entry }

func (a *memAudit) Record(_ context.Context, e audit.Entry) error {
	a.entries = append(a.entries, e)
	return nil
}

func newSvc() (*Service, *memStore) {
	st := &memStore{rows: map[string]Item{}, archived: map[string]bool{}, tr: map[trKey]string{}}
	return NewService(st, noAudit{}), st
}

func ptr(s string) *string { return &s }

func TestLocalizedFallback(t *testing.T) {
	t.Parallel()
	en := map[i18n.Locale]Text{i18n.EN: {Name: "Mom", Description: "For mothers"}}
	tests := []struct {
		name     string
		item     Item
		locale   i18n.Locale
		wantName string
		wantDesc string
		wantUsed i18n.Locale
	}{
		{"requested locale", Item{Code: "mother", Name: "Ibu", Description: "Untuk ibu", Translations: en}, i18n.EN, "Mom", "For mothers", i18n.EN},
		{"default locale ignores translations", Item{Code: "mother", Name: "Ibu", Translations: en}, i18n.ID, "Ibu", "", i18n.ID},
		{"missing translation → base", Item{Code: "mother", Name: "Ibu", Description: "Untuk ibu"}, i18n.EN, "Ibu", "Untuk ibu", i18n.ID},
		{"empty translated name → base", Item{Code: "mother", Name: "Ibu", Translations: map[i18n.Locale]Text{i18n.EN: {Description: "x"}}}, i18n.EN, "Ibu", "x", i18n.ID},
		{"no base name → code", Item{Code: "mother"}, i18n.EN, "mother", "", i18n.ID},
	}
	for _, tt := range tests {
		name, desc, used := tt.item.Localized(tt.locale)
		if name != tt.wantName || desc != tt.wantDesc || used != tt.wantUsed {
			t.Errorf("%s: got (%q, %q, %q); want (%q, %q, %q)", tt.name, name, desc, used, tt.wantName, tt.wantDesc, tt.wantUsed)
		}
	}
}

func TestOptionsAreLocalized(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	_, err := svc.Create(ctx, "a", KindRecipientType, CreateItemInput{Code: "mother", Name: "Ibu",
		Translations: map[string]TranslationInput{"en": {Name: ptr("Mom")}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Create(ctx, "a", KindRecipientType, CreateItemInput{Code: "teacher", Name: "Guru"})

	tests := []struct {
		locale i18n.Locale
		want   map[string]OptionOutput
	}{
		{i18n.EN, map[string]OptionOutput{
			"mother":  {Code: "mother", Name: "Mom", Locale: i18n.EN},
			"teacher": {Code: "teacher", Name: "Guru", Locale: i18n.ID}, // not translated yet → base
		}},
		{i18n.ID, map[string]OptionOutput{
			"mother":  {Code: "mother", Name: "Ibu", Locale: i18n.ID},
			"teacher": {Code: "teacher", Name: "Guru", Locale: i18n.ID},
		}},
	}
	for _, tt := range tests {
		before := st.trQueries
		opts, err := svc.QuestionnaireOptions(i18n.WithLocale(ctx, tt.locale))
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range opts.RecipientTypes {
			if want := tt.want[o.Code]; o != want {
				t.Errorf("%s/%s: got %+v; want %+v", tt.locale, o.Code, o, want)
			}
		}
		// One translation query per kind at most (no N+1); none for the default locale.
		wantQueries := len(Kinds)
		if tt.locale == i18n.Default {
			wantQueries = 0
		}
		if got := st.trQueries - before; got != wantQueries {
			t.Errorf("%s: %d translation queries; want %d", tt.locale, got, wantQueries)
		}
	}
}

func TestTranslationUpsertClearAndArchive(t *testing.T) {
	st := &memStore{rows: map[string]Item{}, archived: map[string]bool{}, tr: map[trKey]string{}}
	aud := &memAudit{}
	svc := NewService(st, aud)
	ctx := context.Background()

	it, err := svc.Create(ctx, "a", KindGenre, CreateItemInput{Code: "lullaby", Name: "Lagu Nina Bobo",
		Translations: map[string]TranslationInput{"en": {Name: ptr("Lullaby"), Description: ptr("Soft and slow")}}})
	if err != nil {
		t.Fatal(err)
	}
	if it.Translations["en"] != (TranslationOutput{Name: "Lullaby", Description: "Soft and slow"}) {
		t.Fatalf("create translations: %+v", it.Translations)
	}

	// Same value again: no write, no audit entry.
	writes, entries := st.trWrites, len(aud.entries)
	if _, err := svc.Update(ctx, "a", KindGenre, it.ID, UpdateItemInput{
		Translations: map[string]TranslationInput{"en": {Name: ptr("Lullaby")}}}); err != nil {
		t.Fatal(err)
	}
	if st.trWrites != writes || len(aud.entries) != entries {
		t.Fatal("unchanged translation was written or audited")
	}

	// Change the name, clear the description: still one live row per field.
	out, err := svc.Update(ctx, "a", KindGenre, it.ID, UpdateItemInput{
		Translations: map[string]TranslationInput{"en": {Name: ptr(" Baby Lullaby "), Description: ptr("")}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Translations["en"] != (TranslationOutput{Name: "Baby Lullaby"}) || len(st.tr) != 1 {
		t.Fatalf("update translations: %+v, live rows %d", out.Translations, len(st.tr))
	}
	last := aud.entries[len(aud.entries)-1]
	if last.Diff["translations.en.name"] != "Baby Lullaby" || last.Diff["translations.en.description"] != "" {
		t.Fatalf("audit diff: %+v", last.Diff)
	}

	// Admin list shows every non-default locale, filled or not.
	list, _ := svc.List(ctx, KindGenre)
	if len(list) != 1 || list[0].Translations["en"].Name != "Baby Lullaby" {
		t.Fatalf("list: %+v", list)
	}

	if err := svc.Archive(ctx, "a", KindGenre, it.ID); err != nil {
		t.Fatal(err)
	}
	if len(st.tr) != 0 {
		t.Fatal("archive left live translations")
	}
}

func TestTranslationLocaleRules(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	for _, key := range []string{"id", "fr", "EN", "en-US"} {
		_, err := svc.Create(ctx, "a", KindMood, CreateItemInput{Code: "calm", Name: "Tenang",
			Translations: map[string]TranslationInput{key: {Name: ptr("Calm")}}})
		if err == nil || !strings.Contains(err.Error(), "VALIDATION") {
			t.Errorf("locale %q: want validation error, got %v", key, err)
		}
	}
	// Description is genre-only, like the base column.
	it, err := svc.Create(ctx, "a", KindMood, CreateItemInput{Code: "calm", Name: "Tenang",
		Translations: map[string]TranslationInput{"en": {Name: ptr("Calm"), Description: ptr("ignored")}}})
	if err != nil || it.Translations["en"] != (TranslationOutput{Name: "Calm"}) {
		t.Fatalf("mood translation: %+v %v", it.Translations, err)
	}
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
