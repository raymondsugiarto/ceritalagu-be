package config

import "time"

// ServerList is server.yml. Add sibling blocks (JWT, storage, ...) here as features need them.
type ServerList struct {
	Rest Server
}

// Server is the HTTP listener (server.rest.*). Env override example: SERVER_REST_PORT=9090.
type Server struct {
	Name            string        `mapstructure:"name"`
	Env             string        `mapstructure:"env"` // development | staging | production
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"readTimeout"`
	WriteTimeout    time.Duration `mapstructure:"writeTimeout"`
	IdleTimeout     time.Duration `mapstructure:"idleTimeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdownTimeout"`
	BodyLimitBytes  int           `mapstructure:"bodyLimitBytes"`
	// CorsOrigins is the browser allowlist. Env: SERVER_REST_CORSORIGINS=url1,url2. Never "*".
	CorsOrigins []string `mapstructure:"corsOrigins"`
	// TrustedProxies are proxy IPs/CIDRs (nginx, the Next.js dev server) allowed to set X-Forwarded-For.
	// Without them c.IP() is the proxy and every guest shares one quota. Env: SERVER_REST_TRUSTEDPROXIES=10.0.0.0/8
	TrustedProxies []string `mapstructure:"trustedProxies"`
	// TrustLoopbackProxy trusts 127.0.0.1 / ::1 as a proxy (local dev behind `next dev`).
	TrustLoopbackProxy bool `mapstructure:"trustLoopbackProxy"`
}

// IsProduction reports whether the app runs in production.
func (s Server) IsProduction() bool { return s.Env == "production" }
