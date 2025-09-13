package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	_msgExecDone = "s3.%s executed successfully"
	_msgExecErr  = "s3.%s execution failed"
	_attrBucket  = "bucket"
	_attrKey     = "key"
)

type service struct {
	s3     *s3.Client
	logger logger.Service
}

var _ Service = (*service)(nil)

func NewService(d Dependencies) Service {
	return &service{
		s3:     d.S3Client,
		logger: d.Log,
	}
}

func (c *service) PutObject(ctx context.Context, bucket string, key string, body io.Reader, contentType string) (string, error) {
	data := map[string]interface{}{_attrBucket: bucket, _attrKey: key, "content_type": contentType}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	out, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})

	if err != nil {
		c.logRequestError(ctx, "PutObject", err, data)
		return "", fmt.Errorf("put object %q: %w", key, err)
	}

	etag := strings.Trim(aws.ToString(out.ETag), `"`)
	data["etag"] = etag
	c.logRequestSuccess(ctx, "PutObject", data)
	return etag, nil
}

func (c *service) GetObject(ctx context.Context, bucket string, key string) (*s3.GetObjectOutput, error) {
	data := map[string]interface{}{_attrBucket: bucket, _attrKey: key}
	obj, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		c.logRequestError(ctx, "GetObject", err, data)
		return nil, err
	}
	c.logRequestSuccess(ctx, "GetObject", data)
	return obj, nil
}

func (c *service) DeleteObject(ctx context.Context, bucket string, key string) (bool, error) {
	data := map[string]interface{}{_attrBucket: bucket, _attrKey: key}
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		c.logRequestError(ctx, "DeleteObject", err, data)
		return false, errors.Join(err, ErrDeleteObject)
	}
	c.logRequestSuccess(ctx, "DeleteObject", data)
	return true, nil
}

func (c *service) logRequestError(ctx context.Context, method string, err error, data map[string]interface{}) {
	data["error"] = err.Error()
	c.logger.Debug(ctx, fmt.Sprintf(_msgExecErr, method), data)
}

func (c *service) logRequestSuccess(ctx context.Context, method string, data map[string]interface{}) {
	c.logger.Debug(ctx, fmt.Sprintf(_msgExecDone, method), data)
}
