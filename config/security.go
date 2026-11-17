package config

// SecurityConfig is security.yml. MasterKey encrypts provider API keys (AES-256-GCM):
// base64 of 32 random bytes, set by env only: SECURITY_MASTERKEY=$(openssl rand -base64 32).
type SecurityConfig struct {
	MasterKey string `mapstructure:"masterKey"`
	// IPHashSalt salts the SHA-256 of client IPs stored in guest_session.ip_hash (env SECURITY_IPHASHSALT).
	IPHashSalt string `mapstructure:"ipHashSalt"`
	// CookieSecure marks the guest cookie Secure (true behind HTTPS).
	CookieSecure bool `mapstructure:"cookieSecure"`
}
