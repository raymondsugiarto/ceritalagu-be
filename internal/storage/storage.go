package storage

import (
	"context"
	"fmt"

	"github.com/yourorg/music-api/config"
)

// Store is what Local and S3 both provide.
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
}

// New picks the driver from storage.yml.
func New(c config.StorageConfig) (Store, error) {
	switch c.Driver {
	case "", "local":
		return NewLocal(c.LocalRoot), nil
	case "s3":
		return NewS3(S3Options{
			Bucket: c.S3.Bucket, Region: c.S3.Region, Endpoint: c.S3.Endpoint,
			AccessKeyID: c.S3.AccessKeyID, SecretAccessKey: c.S3.SecretAccessKey,
			UsePathStyle: c.S3.UsePathStyle, Prefix: c.S3.Prefix,
		}), nil
	default:
		return nil, fmt.Errorf("unknown storage driver %q", c.Driver)
	}
}

// Key layout (one place so every module agrees).

// UserSongKey is users/<userID>/songs/<songID>/<file>.
func UserSongKey(userID, songID, file string) string {
	return "users/" + userID + "/songs/" + songID + "/" + file
}

// GuestSongKey is guests/<guestSessionID>/songs/<songID>/<file>, for songs without a user yet.
func GuestSongKey(guestSessionID, songID, file string) string {
	return "guests/" + guestSessionID + "/songs/" + songID + "/" + file
}

// PlaygroundKey is playground/<id>.<format>.
func PlaygroundKey(id, format string) string { return "playground/" + id + "." + format }
