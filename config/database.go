package config

import "time"

// DatabaseList is database.yml. Add sibling connections (e.g. a read replica) here.
type DatabaseList struct {
	Main Database
}

// Database is one PostgreSQL connection (database.main.*). Env override example: DATABASE_MAIN_PASSWORD=secret.
type Database struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"` // set by env only, never in tracked YAML
	Name            string        `mapstructure:"name"`
	Schema          string        `mapstructure:"schema"`
	SSLMode         string        `mapstructure:"sslMode"`
	MaxOpenConns    int           `mapstructure:"maxOpenConns"`
	MaxIdleConns    int           `mapstructure:"maxIdleConns"`
	ConnMaxLifetime time.Duration `mapstructure:"connMaxLifetime"`
	// AutoMigrate applies pending migrations when `serve` starts.
	AutoMigrate bool `mapstructure:"autoMigrate"`
}
