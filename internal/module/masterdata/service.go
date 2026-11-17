package masterdata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/audit"
)

// codePattern: lowercase snake_case, so codes are stable identifiers in stored answers.
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)

// Store is the storage the service needs (GormRepository).
type Store interface {
	List(ctx context.Context, k Kind, activeOnly bool) ([]Item, error)
	Get(ctx context.Context, k Kind, id string) (Item, error)
	ByCode(ctx context.Context, k Kind, code string) (Item, error)
	CodeEverUsed(ctx context.Context, k Kind, code string) (bool, error)
	Create(ctx context.Context, it Item) (Item, error)
	Update(ctx context.Context, it Item) (Item, error)
	Archive(ctx context.Context, k Kind, id string) error
}

// Auditor records back-office changes (audit.GormRecorder).
type Auditor interface {
	Record(ctx context.Context, e audit.Entry) error
}

// Service manages master data and answers questionnaire lookups.
type Service struct {
	store Store
	audit Auditor
}

// NewService builds the service.
func NewService(store Store, auditor Auditor) *Service { return &Service{store: store, audit: auditor} }

// QuestionnaireOptions returns the active options of every step, in admin sort order.
func (s *Service) QuestionnaireOptions(ctx context.Context) (QuestionnaireOptionsOutput, error) {
	lists := make(map[Kind][]OptionOutput, len(Kinds))
	for _, k := range Kinds {
		items, err := s.store.List(ctx, k, true)
		if err != nil {
			return QuestionnaireOptionsOutput{}, apperror.Internal(err)
		}
		lists[k] = toOptions(items)
	}
	return QuestionnaireOptionsOutput{
		RecipientTypes: lists[KindRecipientType], Occasions: lists[KindOccasion], Genres: lists[KindGenre],
		Moods: lists[KindMood],
	}, nil
}

// IsActiveCode reports whether code is an active option of k. The questionnaire uses it to reject
// codes an admin never created or has turned off.
func (s *Service) IsActiveCode(ctx context.Context, k Kind, code string) (bool, error) {
	it, err := s.store.ByCode(ctx, k, code)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return it.IsActive, nil
}

// List returns every row of k (active and inactive) for the admin.
func (s *Service) List(ctx context.Context, k Kind) ([]ItemOutput, error) {
	if !k.Valid() {
		return nil, apperror.NotFound("jenis master data tidak dikenal")
	}
	items, err := s.store.List(ctx, k, false)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]ItemOutput, len(items))
	for i, it := range items {
		out[i] = toItemOutput(it)
	}
	return out, nil
}

// Create adds a row. The code must be snake_case, not "other", and unused in k.
func (s *Service) Create(ctx context.Context, actorID string, k Kind, in CreateItemInput) (ItemOutput, error) {
	if !k.Valid() {
		return ItemOutput{}, apperror.NotFound("jenis master data tidak dikenal")
	}
	code := strings.TrimSpace(in.Code)
	switch {
	case !codePattern.MatchString(code):
		return ItemOutput{}, fieldErr("code", "huruf kecil, angka dan garis bawah, diawali huruf (mis. hari_ibu)")
	case code == OtherCode:
		return ItemOutput{}, fieldErr("code", `"other" dipakai untuk pilihan "Lainnya"`)
	}
	if _, err := s.store.ByCode(ctx, k, code); err == nil {
		return ItemOutput{}, apperror.Conflict(fmt.Sprintf("kode %q sudah dipakai", code))
	} else if !errors.Is(err, ErrNotFound) {
		return ItemOutput{}, apperror.Internal(err)
	}
	it := Item{Kind: k, Code: code, Name: strings.TrimSpace(in.Name), Hint: strings.TrimSpace(in.Hint),
		SortOrder: in.SortOrder, IsActive: in.IsActive == nil || *in.IsActive}
	if k == KindGenre {
		it.Description, it.Icon = strings.TrimSpace(in.Description), strings.TrimSpace(in.Icon)
	}
	if it.Name == "" {
		return ItemOutput{}, fieldErr("name", "is required")
	}
	it, err := s.store.Create(ctx, it)
	if err != nil {
		return ItemOutput{}, apperror.Internal(err)
	}
	s.record(ctx, actorID, k, "create", it.ID, map[string]any{"code": it.Code, "name": it.Name})
	return toItemOutput(it), nil
}

// Update changes the fields that are set (never the code).
func (s *Service) Update(ctx context.Context, actorID string, k Kind, id string, in UpdateItemInput) (ItemOutput, error) {
	it, err := s.get(ctx, k, id)
	if err != nil {
		return ItemOutput{}, err
	}
	changed := map[string]any{}
	setStr := func(field string, dst *string, v *string) {
		if v != nil && strings.TrimSpace(*v) != *dst {
			*dst = strings.TrimSpace(*v)
			changed[field] = *dst
		}
	}
	setStr("name", &it.Name, in.Name)
	setStr("hint", &it.Hint, in.Hint)
	if k == KindGenre {
		setStr("description", &it.Description, in.Description)
		setStr("icon", &it.Icon, in.Icon)
	}
	if it.Name == "" {
		return ItemOutput{}, fieldErr("name", "is required")
	}
	if in.SortOrder != nil && *in.SortOrder != it.SortOrder {
		it.SortOrder, changed["sort_order"] = *in.SortOrder, *in.SortOrder
	}
	if in.IsActive != nil && *in.IsActive != it.IsActive {
		it.IsActive, changed["is_active"] = *in.IsActive, *in.IsActive
	}
	if len(changed) == 0 {
		return toItemOutput(it), nil
	}
	if it, err = s.store.Update(ctx, it); err != nil {
		return ItemOutput{}, mapErr(err)
	}
	s.record(ctx, actorID, k, "update", it.ID, changed)
	return toItemOutput(it), nil
}

// Archive soft-deletes a row. Stored questionnaires keep the code.
func (s *Service) Archive(ctx context.Context, actorID string, k Kind, id string) error {
	if _, err := s.get(ctx, k, id); err != nil {
		return err
	}
	if err := s.store.Archive(ctx, k, id); err != nil {
		return mapErr(err)
	}
	s.record(ctx, actorID, k, "archive", id, nil)
	return nil
}

// ImportDefaults adds the default options whose code was never used. Existing rows keep the
// admin's edits, and archived codes are not brought back. It returns how many rows were added.
func (s *Service) ImportDefaults(ctx context.Context) (int, error) {
	added := 0
	for _, it := range Defaults() {
		used, err := s.store.CodeEverUsed(ctx, it.Kind, it.Code)
		if err != nil {
			return added, err
		}
		if used {
			continue
		}
		if _, err := s.store.Create(ctx, it); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

func (s *Service) get(ctx context.Context, k Kind, id string) (Item, error) {
	if !k.Valid() {
		return Item{}, apperror.NotFound("jenis master data tidak dikenal")
	}
	it, err := s.store.Get(ctx, k, id)
	if err != nil {
		return Item{}, mapErr(err)
	}
	return it, nil
}

func (s *Service) record(ctx context.Context, actorID string, k Kind, action, id string, diff map[string]any) {
	err := s.audit.Record(ctx, audit.Entry{ActorType: "admin", ActorID: actorID, Action: string(k) + "." + action,
		EntityType: string(k), EntityID: id, Diff: diff})
	if err != nil {
		slog.ErrorContext(ctx, "audit log write failed", "action", action, "entity_id", id, "error", err)
	}
}

func mapErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("data tidak ditemukan")
	}
	return apperror.Internal(err)
}

func fieldErr(field, msg string) error {
	return apperror.Validation([]apperror.ErrorDetail{{Field: field, Message: msg}})
}
