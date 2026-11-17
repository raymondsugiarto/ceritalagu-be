package config

// LoggerConfig is logger.yml. Env override example: LOGGER_LEVEL=info.
type LoggerConfig struct {
	Level  string `mapstructure:"level"`  // debug | info | warn | error
	Format string `mapstructure:"format"` // json | text
}
