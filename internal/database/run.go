package database

import (
	"context"
	"fmt"

	"github.com/yourorg/music-api/config"
)

// WithMigrator opens a dedicated connection to schema (empty = cfg.Schema), runs fn with a Migrator
// and closes everything afterwards.
func WithMigrator(ctx context.Context, cfg config.Database, schema string, fn func(*Migrator) error) error {
	if schema == "" {
		schema = cfg.Schema
	}
	gdb, err := Open(ctx, cfg, schema)
	if err != nil {
		return err
	}
	mg, err := NewMigrator(gdb, schema)
	if err != nil {
		_ = Close(gdb)
		return err
	}
	defer func() { _ = mg.Close() }()
	if err := fn(mg); err != nil {
		return fmt.Errorf("migrate %s: %w", schema, err)
	}
	return nil
}
