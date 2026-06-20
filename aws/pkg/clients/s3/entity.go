package s3

import (
	"context"
	"errors"
	"io"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout = 30 * time.Second
)

type s3APIClient interface {
	transfermanager.S3APIClient
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
}

type s3Presigner interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
	PresignPutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

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

	// GetPresignedPutURL generates a presigned URL for uploading an object directly
	// to S3 via PUT (client→S3), without routing the file through the backend.
	// The upload must use the same contentType supplied here.
	// If expiration is 0, defaults to 15 minutes.
	GetPresignedPutURL(ctx context.Context, key, contentType string, expiration time.Duration) (string, error)

	// EnableLogging enables or disables logging for this client.
	EnableLogging(enable bool)
}

type S3Client struct {
	*client.BaseClient
	s3Client        s3APIClient
	transferManager *transfermanager.Client
	presigner       s3Presigner
	bucket          string
	region          string
}
