package masterdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/yourorg/music-api/internal/i18n"
)

// ErrNotFound is returned when no row matches.
var ErrNotFound = errors.New("master data not found")

// row covers the columns of all four tables; genre-only columns are ignored for the others.
type row struct {
	ID          string         `gorm:"column:id;primaryKey"`
	Code        string         `gorm:"column:code"`
	Name        string         `gorm:"column:name"`
	Description string         `gorm:"column:description"`
	Icon        string         `gorm:"column:icon"`
	Hint        string         `gorm:"column:hint"`
	SortOrder   int            `gorm:"column:sort_order"`
	IsActive    bool           `gorm:"column:is_active"`
	CreatedAt   time.Time      `gorm:"column:created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at"`
}

// hintColumn is the hint column of each table. Table and column names come from these constants,
// never from user input.
func hintColumn(k Kind) string {
	if k == KindGenre {
		return "style_hint"
	}
	return "prompt_hint"
}

func columns(k Kind) string {
	cols := "id, code, name, " + hintColumn(k) + " AS hint, sort_order, is_active, created_at, updated_at"
	if k == KindGenre {
		cols += ", description, icon"
	}
	return cols
}

// GormRepository stores master data in PostgreSQL.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) table(ctx context.Context, k Kind) *gorm.DB {
	return r.db.WithContext(ctx).Table(string(k)).Where("deleted_at IS NULL")
}

// List returns the rows of k by sort order then name; activeOnly hides inactive rows.
func (r *GormRepository) List(ctx context.Context, k Kind, activeOnly bool) ([]Item, error) {
	q := r.table(ctx, k).Select(columns(k)).Order("sort_order, name")
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	var rows []row
	if err := q.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("select %s: %w", k, err)
	}
	out := make([]Item, len(rows))
	for i, m := range rows {
		out[i] = m.toItem(k)
	}
	return out, nil
}

// Get returns one row by ID.
func (r *GormRepository) Get(ctx context.Context, k Kind, id string) (Item, error) {
	return r.first(ctx, k, "id = ?", id)
}

// ByCode returns the row of k with code (active or not).
func (r *GormRepository) ByCode(ctx context.Context, k Kind, code string) (Item, error) {
	return r.first(ctx, k, "code = ?", code)
}

func (r *GormRepository) first(ctx context.Context, k Kind, where string, arg any) (Item, error) {
	var rows []row
	if err := r.table(ctx, k).Select(columns(k)).Where(where, arg).Order("created_at").Limit(1).Scan(&rows).Error; err != nil {
		return Item{}, fmt.Errorf("select %s: %w", k, err)
	}
	if len(rows) == 0 {
		return Item{}, ErrNotFound
	}
	return rows[0].toItem(k), nil
}

// CodeEverUsed reports whether code exists in k, archived rows included.
func (r *GormRepository) CodeEverUsed(ctx context.Context, k Kind, code string) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Table(string(k)).Where("code = ?", code).Count(&n).Error; err != nil {
		return false, fmt.Errorf("count %s code: %w", k, err)
	}
	return n > 0, nil
}

// Create inserts it, minting its ID.
func (r *GormRepository) Create(ctx context.Context, it Item) (Item, error) {
	if it.ID == "" {
		it.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	it.CreatedAt, it.UpdatedAt = now, now
	vals := values(it)
	vals["id"], vals["code"], vals["created_at"] = it.ID, it.Code, it.CreatedAt
	if err := r.db.WithContext(ctx).Table(string(it.Kind)).Create(vals).Error; err != nil {
		return Item{}, fmt.Errorf("insert %s: %w", it.Kind, err)
	}
	return it, nil
}

// Update saves the editable columns of it (the code never changes: questionnaires store it).
func (r *GormRepository) Update(ctx context.Context, it Item) (Item, error) {
	it.UpdatedAt = time.Now().UTC()
	res := r.table(ctx, it.Kind).Where("id = ?", it.ID).Updates(values(it))
	if res.Error != nil {
		return Item{}, fmt.Errorf("update %s %s: %w", it.Kind, it.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return Item{}, ErrNotFound
	}
	return it, nil
}

// Archive soft-deletes a row (deleted_at); questionnaires that used its code keep working.
func (r *GormRepository) Archive(ctx context.Context, k Kind, id string) error {
	now := time.Now().UTC()
	res := r.table(ctx, k).Where("id = ?", id).Updates(map[string]any{"deleted_at": now, "updated_at": now, "is_active": false})
	if res.Error != nil {
		return fmt.Errorf("archive %s %s: %w", k, id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func values(it Item) map[string]any {
	v := map[string]any{
		"name": it.Name, hintColumn(it.Kind): it.Hint, "sort_order": it.SortOrder,
		"is_active": it.IsActive, "updated_at": it.UpdatedAt,
	}
	if it.Kind == KindGenre {
		v["description"], v["icon"] = it.Description, it.Icon
	}
	return v
}

func (m row) toItem(k Kind) Item {
	return Item{ID: m.ID, Kind: k, Code: m.Code, Name: m.Name, Description: m.Description, Icon: m.Icon,
		Hint: m.Hint, SortOrder: m.SortOrder, IsActive: m.IsActive, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// translationRow maps table translation (i18n plan §4.3).
type translationRow struct {
	ID         string         `gorm:"column:id;primaryKey"`
	EntityType string         `gorm:"column:entity_type"`
	EntityID   string         `gorm:"column:entity_id"`
	Locale     string         `gorm:"column:locale"`
	Field      string         `gorm:"column:field"`
	Value      string         `gorm:"column:value"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (translationRow) TableName() string { return "translation" }

// Translations returns every live translation of kind k in one query, keyed by entity ID then
// locale. When a key has several live rows (should not happen; the service upserts) the most
// recently updated one wins.
func (r *GormRepository) Translations(ctx context.Context, k Kind) (map[string]map[i18n.Locale]Text, error) {
	var rows []translationRow
	err := r.db.WithContext(ctx).
		Where("entity_type = ?", string(k)).
		Order("updated_at, created_at").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("select %s translations: %w", k, err)
	}
	out := map[string]map[i18n.Locale]Text{}
	for _, t := range rows {
		byLocale := out[t.EntityID]
		if byLocale == nil {
			byLocale = map[i18n.Locale]Text{}
			out[t.EntityID] = byLocale
		}
		tx := byLocale[i18n.Locale(t.Locale)]
		switch t.Field {
		case FieldName:
			tx.Name = t.Value
		case FieldDescription:
			tx.Description = t.Value
		}
		byLocale[i18n.Locale(t.Locale)] = tx
	}
	return out, nil
}

// SetTranslation upserts one (entity, locale, field) value inside a transaction: the live row is
// updated, or one is created when none exists. An empty value soft-deletes the live row(s).
// Uniqueness is the app's job (no UNIQUE constraint, database-standards §3).
func (r *GormRepository) SetTranslation(ctx context.Context, k Kind, entityID string, l i18n.Locale, field, value string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var live []translationRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("entity_type = ? AND entity_id = ? AND locale = ? AND field = ?", string(k), entityID, string(l), field).
			Order("updated_at DESC").
			Find(&live).Error
		if err != nil {
			return fmt.Errorf("select translation: %w", err)
		}
		now := time.Now().UTC()
		if value == "" {
			if len(live) == 0 {
				return nil
			}
			ids := make([]string, len(live))
			for i, t := range live {
				ids[i] = t.ID
			}
			return tx.Model(&translationRow{}).Where("id IN ?", ids).
				Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
		}
		if len(live) > 0 {
			return tx.Model(&translationRow{}).Where("id = ?", live[0].ID).
				Updates(map[string]any{"value": value, "updated_at": now}).Error
		}
		row := translationRow{ID: uuid.NewString(), EntityType: string(k), EntityID: entityID, Locale: string(l),
			Field: field, Value: value, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("insert translation: %w", err)
		}
		return nil
	})
}

// ArchiveTranslations soft-deletes every live translation of one row (when the row is archived).
func (r *GormRepository) ArchiveTranslations(ctx context.Context, k Kind, entityID string) error {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Model(&translationRow{}).
		Where("entity_type = ? AND entity_id = ?", string(k), entityID).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
	if err != nil {
		return fmt.Errorf("archive %s %s translations: %w", k, entityID, err)
	}
	return nil
}
