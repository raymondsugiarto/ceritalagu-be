package config

// StorageConfig is storage.yml: where generated audio is kept.
type StorageConfig struct {
	Driver    string   `mapstructure:"driver"`    // local | s3
	LocalRoot string   `mapstructure:"localRoot"` // directory for the local driver
	S3        S3Config `mapstructure:"s3"`
}

// S3Config is an S3-compatible bucket (AWS S3, Cloudflare R2, MinIO...). Keys come from env only:
// STORAGE_S3_ACCESSKEYID and STORAGE_S3_SECRETACCESSKEY.
type S3Config struct {
	Bucket          string `mapstructure:"bucket"`
	Region          string `mapstructure:"region"`   // "auto" for R2
	Endpoint        string `mapstructure:"endpoint"` // empty for AWS; https://<account>.r2.cloudflarestorage.com for R2
	AccessKeyID     string `mapstructure:"accessKeyId"`
	SecretAccessKey string `mapstructure:"secretAccessKey"`
	UsePathStyle    bool   `mapstructure:"usePathStyle"` // true for MinIO
	Prefix          string `mapstructure:"prefix"`       // optional key prefix, e.g. "prod/"
}
