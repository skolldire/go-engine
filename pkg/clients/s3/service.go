package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

func NewClient(acf aws.Config, cfg Config, log logger.Service) Service {
	s3Client := s3.NewFromConfig(acf, func(o *s3.Options) {
		if cfg.Region != "" {
			o.Region = cfg.Region
		}
	})

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	baseConfig := client.BaseConfig{
		EnableLogging:  cfg.EnableLogging,
		WithResilience: cfg.WithResilience,
		Resilience:     cfg.Resilience,
		Timeout:        timeout,
	}

	c := &S3Client{
		BaseClient: client.NewBaseClientWithName(baseConfig, log, "S3"),
		s3Client:   s3Client,
		bucket:     cfg.Bucket,
		region:     cfg.Region,
	}

	if c.IsLoggingEnabled() {
		log.Debug(context.Background(), "S3 client initialized",
			map[string]interface{}{
				"region": cfg.Region,
				"bucket": cfg.Bucket,
			})
	}

	return c
}

func (c *S3Client) PutObject(ctx context.Context, key string, body io.Reader, contentType string, metadata map[string]string) error {
	if key == "" || body == nil {
		return ErrInvalidInput
	}

	uploader := manager.NewUploader(c.s3Client)

	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	}

	if len(metadata) > 0 {
		input.Metadata = metadata
	}

	_, err := c.Execute(ctx, "PutObject", func() (interface{}, error) {
		return uploader.Upload(ctx, input)
	})

	if err != nil {
		return c.GetLogger().WrapError(err, ErrUploadFailed.Error())
	}

	return nil
}

func (c *S3Client) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, ErrInvalidInput
	}

	result, err := c.Execute(ctx, "GetObject", func() (interface{}, error) {
		return c.s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(key),
		})
	})

	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrObjectNotFound
		}
		return nil, c.GetLogger().WrapError(err, ErrDownloadFailed.Error())
	}

	response, err := client.SafeTypeAssert[*s3.GetObjectOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrDownloadFailed.Error())
	}
	return response.Body, nil
}

func (c *S3Client) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "DeleteObject", func() (interface{}, error) {
		return c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(key),
		})
	})

	if err != nil {
		return c.GetLogger().WrapError(err, "error deleting object")
	}

	return nil
}

func (c *S3Client) HeadObject(ctx context.Context, key string) (*ObjectMetadata, error) {
	if key == "" {
		return nil, ErrInvalidInput
	}

	result, err := c.Execute(ctx, "HeadObject", func() (interface{}, error) {
		return c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(key),
		})
	})

	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrObjectNotFound
		}
		return nil, c.GetLogger().WrapError(err, "error getting object metadata")
	}

	response, err := client.SafeTypeAssert[*s3.HeadObjectOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error getting object metadata")
	}
	return &ObjectMetadata{
		Key:          key,
		Size:         *response.ContentLength,
		LastModified: *response.LastModified,
		ETag:         aws.ToString(response.ETag),
		ContentType:  aws.ToString(response.ContentType),
	}, nil
}

func (c *S3Client) ListObjects(ctx context.Context, prefix string, maxKeys int32) ([]ObjectMetadata, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	var allObjects []ObjectMetadata
	var continuationToken *string

	for {
		result, err := c.Execute(ctx, "ListObjects", func() (interface{}, error) {
			input := &s3.ListObjectsV2Input{
				Bucket:  aws.String(c.bucket),
				Prefix:  aws.String(prefix),
				MaxKeys: aws.Int32(maxKeys),
			}
			if continuationToken != nil {
				input.ContinuationToken = continuationToken
			}
			return c.s3Client.ListObjectsV2(ctx, input)
		})

		if err != nil {
			return nil, c.GetLogger().WrapError(err, "error listing objects")
		}

		response, err := client.SafeTypeAssert[*s3.ListObjectsV2Output](result)
		if err != nil {
			return nil, c.GetLogger().WrapError(err, "error listing objects")
		}

		for _, obj := range response.Contents {
			allObjects = append(allObjects, ObjectMetadata{
				Key:          aws.ToString(obj.Key),
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
				ETag:         aws.ToString(obj.ETag),
			})
		}

		if response.NextContinuationToken == nil {
			break
		}
		continuationToken = response.NextContinuationToken
	}

	return allObjects, nil
}

func (c *S3Client) CopyObject(ctx context.Context, sourceKey, destKey string) error {
	if sourceKey == "" || destKey == "" {
		return ErrInvalidInput
	}

	source := fmt.Sprintf("%s/%s", c.bucket, url.PathEscape(sourceKey))
	_, err := c.Execute(ctx, "CopyObject", func() (interface{}, error) {
		return c.s3Client.CopyObject(ctx, &s3.CopyObjectInput{
			Bucket:     aws.String(c.bucket),
			CopySource:  aws.String(source),
			Key:         aws.String(destKey),
		})
	})

	if err != nil {
		return c.GetLogger().WrapError(err, "error copying object")
	}

	return nil
}

func (c *S3Client) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	if key == "" {
		return "", ErrInvalidInput
	}

	if expiration == 0 {
		expiration = 15 * time.Minute
	}

	presignClient := s3.NewPresignClient(c.s3Client)
	
	result, err := c.Execute(ctx, "GetPresignedURL", func() (interface{}, error) {
		request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(key),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = expiration
		})
		if err != nil {
			return nil, err
		}
		return request.URL, nil
	})

	if err != nil {
		return "", c.GetLogger().WrapError(err, "error generating presigned URL")
	}

	urlStr, err := client.SafeTypeAssert[string](result)
	if err != nil {
		return "", c.GetLogger().WrapError(err, "error generating presigned URL")
	}
	return urlStr, nil
}

func (c *S3Client) EnableLogging(enable bool) {
	c.SetLogging(enable)
}

