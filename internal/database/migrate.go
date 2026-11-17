package database

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"

	"github.com/yourorg/music-api/db"
)

// Migrator applies the embedded migrations (db/migrations/postgres) to one schema.
// It is shared by `serve` (auto-migrate) and the `db migrate` subcommands.
type Migrator struct {
	m *migrate.Migrate
}

// NewMigrator creates schema if missing and prepares golang-migrate on the given connection.
func NewMigrator(gdb *gorm.DB, schema string) (*Migrator, error) {
	if err := gdb.Exec(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %q`, schema)).Error; err != nil {
		return nil, fmt.Errorf("create schema %s: %w", schema, err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("database handle: %w", err)
	}
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{SchemaName: schema})
	if err != nil {
		return nil, fmt.Errorf("migrate driver: %w", err)
	}
	src, err := iofs.New(db.Migrations, db.MigrationsDir)
	if err != nil {
		return nil, fmt.Errorf("migrate source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("migrate init: %w", err)
	}
	return &Migrator{m: m}, nil
}

// Up applies steps pending migrations; steps <= 0 applies all of them.
func (mg *Migrator) Up(steps int) error {
	var err error
	if steps > 0 {
		err = mg.m.Steps(steps)
	} else {
		err = mg.m.Up()
	}
	return ignoreNoChange(err)
}

// Down rolls back steps migrations (minimum 1; there is no "down all").
func (mg *Migrator) Down(steps int) error {
	if steps <= 0 {
		steps = 1
	}
	return ignoreNoChange(mg.m.Steps(-steps))
}

// Version returns the current version and whether the schema is dirty.
func (mg *Migrator) Version() (uint, bool, error) {
	v, dirty, err := mg.m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("migration version: %w", err)
	}
	return v, dirty, nil
}

// Force sets the version without running migrations (recovery from a dirty state only).
func (mg *Migrator) Force(version int) error {
	if err := mg.m.Force(version); err != nil {
		return fmt.Errorf("force version %d: %w", version, err)
	}
	return nil
}

func ignoreNoChange(err error) error {
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Close releases the migrator. It also closes the *gorm.DB connection it was built on,
// so give the migrator its own connection (see RunMigrations).
func (mg *Migrator) Close() error {
	srcErr, dbErr := mg.m.Close()
	return errors.Join(srcErr, dbErr)
}
