package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

type s3Adapter struct {
	client  *s3.Client
	timeout time.Duration
	retries RetryPolicy
}

func newS3Adapter(cfg aws.Config, timeout time.Duration, retries RetryPolicy) cloud.Client {
	return &s3Adapter{
		client:  s3.NewFromConfig(cfg),
		timeout: timeout,
		retries: retries,
	}
}

func (a *s3Adapter) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	switch req.Operation {
	case "s3.put_object":
		return a.putObject(ctx, req)
	case "s3.get_object":
		return a.getObject(ctx, req)
	case "s3.delete_object":
		return a.deleteObject(ctx, req)
	case "s3.head_object":
		return a.headObject(ctx, req)
	case "s3.list_objects":
		return a.listObjects(ctx, req)
	case "s3.copy_object":
		return a.copyObject(ctx, req)
	default:
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("unsupported S3 operation: %s", req.Operation))
	}
}

func (a *s3Adapter) putObject(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Path format: "bucket/key" or "bucket/key/prefix"
	bucket, key := parseS3Path(req.Path)
	if bucket == "" || key == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "path must be in format 'bucket/key'")
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(req.Body),
	}

	// Parse headers for S3-specific attributes
	if req.Headers != nil {
		if contentType, ok := req.Headers["s3.content_type"]; ok {
			input.ContentType = aws.String(contentType)
		}
		if acl, ok := req.Headers["s3.acl"]; ok {
			input.ACL = s3types.ObjectCannedACL(acl)
		}

		// Parse metadata
		metadata := make(map[string]string)
		for k, v := range req.Headers {
			if strings.HasPrefix(k, "s3.metadata.") {
				metaKey := strings.TrimPrefix(k, "s3.metadata.")
				metadata[metaKey] = v
			}
		}
		if len(metadata) > 0 {
			input.Metadata = metadata
		}
	}

	result, err := a.client.PutObject(ctx, input)
	if err != nil {
		return nil, normalizeS3Error(err, "s3.put_object")
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"s3.etag": aws.ToString(result.ETag),
		},
		Metadata: map[string]interface{}{
			"s3.etag":                aws.ToString(result.ETag),
			"s3.version_id":          aws.ToString(result.VersionId),
			"s3.server_side_encryption": string(result.ServerSideEncryption),
		},
	}, nil
}

func (a *s3Adapter) getObject(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Path format: "bucket/key"
	bucket, key := parseS3Path(req.Path)
	if bucket == "" || key == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "path must be in format 'bucket/key'")
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := a.client.GetObject(ctx, input)
	if err != nil {
		return nil, normalizeS3Error(err, "s3.get_object")
	}
	defer result.Body.Close()

	// Read body
	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("failed to read object body: %v", err))
	}

	headers := make(map[string]string)
	if result.ContentType != nil {
		headers["s3.content_type"] = *result.ContentType
	}
	if result.ContentLength != nil {
		headers["s3.content_length"] = fmt.Sprintf("%d", *result.ContentLength)
	}
	if result.ETag != nil {
		headers["s3.etag"] = *result.ETag
	}

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
		Headers:    headers,
		Metadata: map[string]interface{}{
			"s3.content_type":   aws.ToString(result.ContentType),
			"s3.content_length": result.ContentLength,
			"s3.etag":           aws.ToString(result.ETag),
			"s3.last_modified":  result.LastModified,
		},
	}, nil
}

func (a *s3Adapter) deleteObject(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Path format: "bucket/key"
	bucket, key := parseS3Path(req.Path)
	if bucket == "" || key == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "path must be in format 'bucket/key'")
	}

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	_, err := a.client.DeleteObject(ctx, input)
	if err != nil {
		return nil, normalizeS3Error(err, "s3.delete_object")
	}

	return &cloud.Response{
		StatusCode: 204, // No Content
	}, nil
}

func (a *s3Adapter) headObject(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Path format: "bucket/key"
	bucket, key := parseS3Path(req.Path)
	if bucket == "" || key == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "path must be in format 'bucket/key'")
	}

	input := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := a.client.HeadObject(ctx, input)
	if err != nil {
		return nil, normalizeS3Error(err, "s3.head_object")
	}

	headers := make(map[string]string)
	if result.ContentType != nil {
		headers["s3.content_type"] = *result.ContentType
	}
	if result.ContentLength != nil {
		headers["s3.content_length"] = fmt.Sprintf("%d", *result.ContentLength)
	}
	if result.ETag != nil {
		headers["s3.etag"] = *result.ETag
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers:    headers,
		Metadata: map[string]interface{}{
			"s3.content_type":   aws.ToString(result.ContentType),
			"s3.content_length": result.ContentLength,
			"s3.etag":           aws.ToString(result.ETag),
			"s3.last_modified":  result.LastModified,
		},
	}, nil
}

func (a *s3Adapter) listObjects(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Path format: "bucket" or "bucket/prefix"
	bucket, prefix := parseS3Path(req.Path)
	if bucket == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "bucket name is required")
	}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	// Parse query params
	if req.QueryParams != nil {
		if maxKeys, ok := req.QueryParams["MaxKeys"]; ok {
			var max int
			if _, err := fmt.Sscanf(maxKeys, "%d", &max); err == nil {
				input.MaxKeys = aws.Int32(int32(max))
			}
		}
		if delimiter, ok := req.QueryParams["Delimiter"]; ok {
			input.Delimiter = aws.String(delimiter)
		}
	}

	result, err := a.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, normalizeS3Error(err, "s3.list_objects")
	}

	// Convert to JSON array
	objects := make([]map[string]interface{}, len(result.Contents))
	for i, obj := range result.Contents {
		objects[i] = map[string]interface{}{
			"key":          aws.ToString(obj.Key),
			"size":         obj.Size,
			"last_modified": obj.LastModified,
			"etag":         aws.ToString(obj.ETag),
		}
	}

	body, _ := json.Marshal(objects)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
		Headers: map[string]string{
			"s3.object_count": fmt.Sprintf("%d", len(result.Contents)),
		},
	}, nil
}

func (a *s3Adapter) copyObject(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Path format: "destBucket/destKey"
	destBucket, destKey := parseS3Path(req.Path)
	if destBucket == "" || destKey == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "path must be in format 'destBucket/destKey'")
	}

	// Source bucket/key from headers
	if req.Headers == nil {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "source bucket and key are required in headers")
	}
	sourceBucket, ok := req.Headers["s3.source_bucket"]
	if !ok {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "s3.source_bucket header is required")
	}
	sourceKey, ok := req.Headers["s3.source_key"]
	if !ok {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "s3.source_key header is required")
	}

	source := fmt.Sprintf("%s/%s", sourceBucket, sourceKey)
	input := &s3.CopyObjectInput{
		Bucket:     aws.String(destBucket),
		CopySource: aws.String(source),
		Key:        aws.String(destKey),
	}

	result, err := a.client.CopyObject(ctx, input)
	if err != nil {
		return nil, normalizeS3Error(err, "s3.copy_object")
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"s3.copy_source_version_id": aws.ToString(result.CopySourceVersionId),
			"s3.version_id":             aws.ToString(result.VersionId),
		},
		Metadata: map[string]interface{}{
			"s3.copy_source_version_id": aws.ToString(result.CopySourceVersionId),
			"s3.version_id":              aws.ToString(result.VersionId),
		},
	}
	
	// Safely add ETag if CopyObjectResult is not nil
	if result.CopyObjectResult != nil && result.CopyObjectResult.ETag != nil {
		response.Metadata["s3.etag"] = aws.ToString(result.CopyObjectResult.ETag)
	}
	
	return response, nil
	}, nil
}

func parseS3Path(path string) (bucket, key string) {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

func normalizeS3Error(err error, operation string) *cloud.Error {
	if err == nil {
		return nil
	}

	return cloud.NewErrorWithCause(
		fmt.Sprintf("%s.error", operation),
		err.Error(),
		err,
	).WithMetadata("status_code", 500)
}

