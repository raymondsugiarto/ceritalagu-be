package config

import "time"

// GenerationConfig is generation.yml: the song pipeline and its worker.
type GenerationConfig struct {
	Queue           string        `mapstructure:"queue"`           // Asynq queue name
	Concurrency     int           `mapstructure:"concurrency"`     // jobs processed in parallel per worker
	TaskTimeout     time.Duration `mapstructure:"taskTimeout"`     // max time for one job (AI calls are slow)
	TaskMaxRetry    int           `mapstructure:"taskMaxRetry"`    // Asynq redeliveries (worker crash, DB error)
	PreviewSeconds  int           `mapstructure:"previewSeconds"`  // PRD: 60
	FadeSeconds     int           `mapstructure:"fadeSeconds"`     // PRD: 3
	GuestRetention  time.Duration `mapstructure:"guestRetention"`  // PRD: 30 days
	RetryBaseDelay  time.Duration `mapstructure:"retryBaseDelay"`  // provider retry backoff start
	FFmpegPath      string        `mapstructure:"ffmpegPath"`      // empty = ffmpeg from PATH
	GuestDailyQuota int           `mapstructure:"guestDailyQuota"` // PRD: 3 new songs per guest/IP per day
	MaxRegenerate   int           `mapstructure:"maxRegenerate"`   // PRD: 2 "Buat Ulang" per song
}
