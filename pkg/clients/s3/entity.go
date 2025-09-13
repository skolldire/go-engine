package s3

import (
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

//go:generate mockery --name Service --filename service.go
type Service interface {
	PutObject(ctx context.Context, bucket string, key string, body io.Reader, contentType string) (string, error)
	GetObject(ctx context.Context, bucket string, key string) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, bucket string, key string) (bool, error)
}

type Dependencies struct {
	S3Client *s3.Client
	Log      logger.Service
}

var ErrDeleteObject = errors.New("failed to delete object")
