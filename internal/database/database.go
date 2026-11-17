// Package database opens the PostgreSQL connection (GORM) and runs the embedded migrations.
package database

import (
	"context"
	"fmt"
	"net/url"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/yourorg/music-api/config"
)

// Open connects to PostgreSQL with search_path set to schema (empty = cfg.Schema) and pings it.
func Open(ctx context.Context, cfg config.Database, schema string) (*gorm.DB, error) {
	if schema == "" {
		schema = cfg.Schema
	}
	db, err := gorm.Open(postgres.Open(DSN(cfg, schema)), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

// DSN builds a PostgreSQL URL. The password never appears in logs: log cfg fields, not the DSN.
func DSN(cfg config.Database, schema string) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   cfg.Name,
	}
	q := url.Values{}
	q.Set("sslmode", cfg.SSLMode)
	q.Set("search_path", schema)
	q.Set("TimeZone", "UTC")
	u.RawQuery = q.Encode()
	return u.String()
}

// Close closes the underlying connection pool.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("database handle: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}

// Checker reports database reachability for /readyz. It implements app.Checker.
type Checker struct {
	db *gorm.DB
}

// NewChecker wraps db for the readiness probe.
func NewChecker(db *gorm.DB) *Checker { return &Checker{db: db} }

// Name is the key shown in the /readyz response.
func (c *Checker) Name() string { return "database" }

// Check pings the database.
func (c *Checker) Check(ctx context.Context) error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("database handle: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}
