// Package db embeds the SQL migrations so the binary can migrate without the source tree.
// Rules for every migration: docs/database-standards.md.
package db

import "embed"

// Migrations holds migrations/postgres/*.sql (golang-migrate format).
//
//go:embed migrations/postgres/*.sql
var Migrations embed.FS

// MigrationsDir is the path of the migration files inside Migrations.
const MigrationsDir = "migrations/postgres"
