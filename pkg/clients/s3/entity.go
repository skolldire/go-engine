package s3

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout = 30 * time.Second
)

var (
	ErrObjectNotFound = errors.New("object not found")
	ErrInvalidInput   = errors.New("invalid input")
	ErrUploadFailed   = errors.New("error uploading object")
	ErrDownloadFailed = errors.New("error downloading object")
)

type Config struct {
	Region         string            `mapstructure:"region" json:"region"`
	Bucket         string            `mapstructure:"bucket" json:"bucket"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
	Timeout        time.Duration     `mapstructure:"timeout" json:"timeout"`
}

type ObjectMetadata struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
	ContentType  string
}

type Service interface {
	// PutObject uploads an object to S3.
	// The caller is responsible for closing the body reader if needed.
	PutObject(ctx context.Context, key string, body io.Reader, contentType string, metadata map[string]string) error

	// GetObject retrieves an object from S3.
	// The caller MUST close the returned ReadCloser to avoid resource leaks.
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)

	// DeleteObject deletes an object from S3.
	DeleteObject(ctx context.Context, key string) error

	// HeadObject retrieves object metadata without downloading the object.
	HeadObject(ctx context.Context, key string) (*ObjectMetadata, error)

	// ListObjects lists objects in the bucket with the given prefix.
	// maxKeys limits the number of results (defaults to 1000 if <= 0).
	ListObjects(ctx context.Context, prefix string, maxKeys int32) ([]ObjectMetadata, error)

	// CopyObject copies an object within the same bucket.
	CopyObject(ctx context.Context, sourceKey, destKey string) error

	// GetPresignedURL generates a presigned URL for temporary access to an object.
	// If expiration is 0, defaults to 15 minutes.
	GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error)

	// EnableLogging enables or disables logging for this client.
	EnableLogging(enable bool)
}

type S3Client struct {
	*client.BaseClient
	s3Client *s3.Client
	bucket   string
	region   string
}
