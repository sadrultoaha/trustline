// Package blob stores evidence files. Real: any S3-compatible store (MinIO
// locally, S3 in production). Fake: local filesystem, used by tests.
package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var ErrNotFound = errors.New("blob not found")

type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Name() string
}

// --- S3 ---------------------------------------------------------------------

type S3Config struct {
	Endpoint  string // empty for AWS
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle bool
}

type S3 struct {
	c      *s3.Client
	bucket string
}

func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("blob: S3_BUCKET is required")
	}
	opts := s3.Options{
		Region:       cfg.Region,
		UsePathStyle: cfg.PathStyle,
	}
	if cfg.AccessKey != "" {
		opts.Credentials = credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	}
	if cfg.Endpoint != "" {
		opts.BaseEndpoint = aws.String(cfg.Endpoint)
	}
	c := s3.New(opts)
	st := &S3{c: c, bucket: cfg.Bucket}
	if _, err := c.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(cfg.Bucket)}); err != nil {
		if _, cerr := c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(cfg.Bucket)}); cerr != nil {
			var owned *types.BucketAlreadyOwnedByYou
			if !errors.As(cerr, &owned) {
				return nil, fmt.Errorf("blob: bucket %s unavailable: %v (create: %w)", cfg.Bucket, err, cerr)
			}
		}
	}
	return st, nil
}

func (s *S3) Name() string { return "s3:" + s.bucket }

func (s *S3) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(data),
		ContentType: aws.String(contentType), ServerSideEncryption: "",
	})
	return err
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.c.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}

// --- filesystem ------------------------------------------------------------

type FS struct{ Root string }

func (f *FS) Name() string { return "fs:" + f.Root }

func (f *FS) path(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if strings.Contains(key, "..") {
		return "", errors.New("blob: invalid key")
	}
	return filepath.Join(f.Root, clean), nil
}

func (f *FS) Put(_ context.Context, key string, data []byte, _ string) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o640)
}

func (f *FS) Get(_ context.Context, key string) ([]byte, error) {
	p, err := f.path(key)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

func (f *FS) Delete(_ context.Context, key string) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
