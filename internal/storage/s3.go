package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ErrNotFound is returned by Get for a key that is not stored.
var ErrNotFound = errors.New("storage object not found")

// S3Options configure an S3-compatible bucket (AWS S3, Cloudflare R2, MinIO).
type S3Options struct {
	Bucket          string
	Region          string
	Endpoint        string // empty for AWS
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	Prefix          string // prepended to every key, e.g. "prod/"
}

// S3 stores objects in a private bucket. Same method set as Local.
type S3 struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3 builds the client; it does not call the bucket.
func NewS3(o S3Options) *S3 {
	client := s3.New(s3.Options{
		Region:       o.Region,
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(o.AccessKeyID, o.SecretAccessKey, "")),
		UsePathStyle: o.UsePathStyle,
		BaseEndpoint: nilIfEmpty(o.Endpoint),
		// R2 and MinIO reject the newer default checksum headers on some versions.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	prefix := strings.Trim(o.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return &S3{client: client, bucket: o.Bucket, prefix: prefix}
}

// Put uploads data under key, e.g. "users/<id>/songs/<id>/full.mp3".
func (s *S3) Put(ctx context.Context, key string, data []byte, contentType string) error {
	k, err := s.key(key)
	if err != nil {
		return err
	}
	in := &s3.PutObjectInput{Bucket: &s.bucket, Key: &k, Body: bytes.NewReader(data), ContentLength: aws.Int64(int64(len(data)))}
	if contentType != "" {
		in.ContentType = &contentType
	}
	if _, err := s.client.PutObject(ctx, in); err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

// Get downloads the object stored under key.
func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	k, err := s.key(key)
	if err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &k})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	defer func() { _ = out.Body.Close() }()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 read %s: %w", key, err)
	}
	return data, nil
}

func (s *S3) key(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return s.prefix + key, nil
}

func nilIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
