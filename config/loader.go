// Package config loads application settings once at startup. Nothing else in the app reads os.Getenv.
//
// Sources, highest precedence first:
//
//	shell env          SERVER_REST_PORT=9090 (key path with "." → "_", upper-cased)
//	.env               repo-root file, same keys as shell env; never overrides the shell
//	config/<dir>/*.yml one file per concern (server.yml, logger.yml, ...)
//
// <dir> is "resources" (per-machine values) unless CONFIG_DIR is set, e.g. CONFIG_DIR=example.
// CONFIG_PATH replaces the whole directory (used in the container image, where sources are not shipped).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

// Config is the root of all settings. Add one struct (and one YAML file) per concern.
type Config struct {
	Server       ServerList
	Logger       LoggerConfig
	Database     DatabaseList
	Redis        RedisConfig
	Storage      StorageConfig
	Security     SecurityConfig
	Generation   GenerationConfig
	Mail         MailConfig
	Payment      PaymentConfig
	Notification NotificationConfig
}

// files are read in order. Every file is required; add new concerns here.
var files = []string{"server.yml", "logger.yml", "database.yml", "redis.yml", "storage.yml", "security.yml", "generation.yml", "mail.yml", "payment.yml", "notification.yml"}

// basepath is the directory of this source file (<repo>/config). It is only valid when the
// binary was built from source without -trimpath (go run, go test); otherwise set CONFIG_PATH.
var (
	_, b, _, _ = runtime.Caller(0)
	basepath   = filepath.Dir(b)
	repoRoot   = filepath.Dir(basepath)
)

// Load reads YAML, .env and env overrides into a validated Config.
func Load() (Config, error) {
	if err := loadDotEnv(filepath.Join(repoRoot, ".env")); err != nil {
		return Config{}, err
	}

	v := viper.New()
	v.AddConfigPath(configDir())
	v.SetConfigType("yml")
	for _, name := range files {
		v.SetConfigName(strings.TrimSuffix(name, ".yml"))
		if err := v.MergeInConfig(); err != nil {
			return Config{}, fmt.Errorf("read %s from %s: %w", name, configDir(), err)
		}
	}

	// Any YAML key can be overridden by env: server.rest.port → SERVER_REST_PORT.
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	for _, k := range v.AllKeys() {
		_ = v.BindEnv(k)
	}
	// 12-factor aliases for the listen address.
	_ = v.BindEnv("server.rest.port", "SERVER_REST_PORT", "PORT")
	_ = v.BindEnv("server.rest.host", "SERVER_REST_HOST", "HOST")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func configDir() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	dir := os.Getenv("CONFIG_DIR")
	if dir == "" {
		dir = "resources"
	}
	return filepath.Join(basepath, dir)
}

// loadDotEnv copies KEY=VALUE lines from .env into the process env, without overriding
// variables already set by the shell. A missing file is fine.
func loadDotEnv(path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // fixed path under the repo root
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read .env: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, set := os.LookupEnv(key); !set {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

func (c Config) validate() error {
	switch c.Server.Rest.Env {
	case "development", "staging", "production":
	default:
		return fmt.Errorf("server.rest.env must be development, staging or production, got %q", c.Server.Rest.Env)
	}
	if c.Server.Rest.Port <= 0 || c.Server.Rest.Port > 65535 {
		return fmt.Errorf("server.rest.port out of range: %d", c.Server.Rest.Port)
	}
	switch c.Storage.Driver {
	case "local":
	case "s3":
		s3 := c.Storage.S3
		if s3.Bucket == "" || s3.Region == "" || s3.AccessKeyID == "" || s3.SecretAccessKey == "" {
			return fmt.Errorf("storage.driver s3 needs STORAGE_S3_BUCKET, STORAGE_S3_REGION, STORAGE_S3_ACCESSKEYID and STORAGE_S3_SECRETACCESSKEY")
		}
	default:
		return fmt.Errorf("storage.driver must be local or s3, got %q", c.Storage.Driver)
	}
	if c.Generation.PreviewSeconds <= 0 || c.Generation.Concurrency <= 0 || c.Generation.Queue == "" {
		return fmt.Errorf("generation.previewSeconds, generation.concurrency and generation.queue are required")
	}
	switch c.Mail.Driver {
	case "log":
		if c.Server.Rest.Env == "production" {
			return fmt.Errorf("mail.driver log prints login codes to the log; use smtp or brevo in production")
		}
	case "smtp":
		if c.Mail.SMTP.Host == "" || c.Mail.SMTP.Port == 0 || c.Mail.From == "" {
			return fmt.Errorf("mail.driver smtp needs mail.from, MAIL_SMTP_HOST and MAIL_SMTP_PORT")
		}
	case "brevo":
		if c.Mail.Brevo.APIKey == "" {
			return fmt.Errorf("mail.driver brevo needs MAIL_BREVO_APIKEY")
		}
		if c.Mail.Brevo.SenderEmail == "" {
			return fmt.Errorf("mail.driver brevo needs MAIL_BREVO_SENDEREMAIL (a verified sender)")
		}
	default:
		return fmt.Errorf("mail.driver must be log, smtp or brevo, got %q", c.Mail.Driver)
	}
	if c.Notification.Enabled && c.Notification.MaxAttachmentMB <= 0 {
		return fmt.Errorf("notification.maxAttachmentMB must be > 0 when notification.enabled is on")
	}
	if c.Payment.UnlockPrice <= 0 {
		return fmt.Errorf("payment.unlockPrice must be > 0")
	}
	if c.Database.Main.Host == "" || c.Database.Main.Name == "" {
		return fmt.Errorf("database.main.host and database.main.name are required")
	}
	return nil
}
