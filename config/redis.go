package config

// RedisConfig is redis.yml: the Asynq job queue (and later rate limiting). Env: REDIS_PASSWORD=...
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"` // env only
	DB       int    `mapstructure:"db"`
}
