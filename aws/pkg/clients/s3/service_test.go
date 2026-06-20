package s3

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ── mocks ─────────────────────────────────────────────────────────────────────

type mockLogger struct{ mock.Mock }

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]interface{}) {
	m.Called(ctx, err, fields)
}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (m *mockLogger) WrapError(err error, msg string) error                                    { return err }
func (m *mockLogger) WithField(key string, value interface{}) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                                      { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                           { return nil }

type mockS3API struct{ mock.Mock }

func (m *mockS3API) PutObject(ctx context.Context, params *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.PutObjectOutput), args.Error(1)
}
func (m *mockS3API) UploadPart(ctx context.Context, params *awss3.UploadPartInput, _ ...func(*awss3.Options)) (*awss3.UploadPartOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.UploadPartOutput), args.Error(1)
}
func (m *mockS3API) CreateMultipartUpload(ctx context.Context, params *awss3.CreateMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.CreateMultipartUploadOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.CreateMultipartUploadOutput), args.Error(1)
}
func (m *mockS3API) CompleteMultipartUpload(ctx context.Context, params *awss3.CompleteMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.CompleteMultipartUploadOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.CompleteMultipartUploadOutput), args.Error(1)
}
func (m *mockS3API) AbortMultipartUpload(ctx context.Context, params *awss3.AbortMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.AbortMultipartUploadOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.AbortMultipartUploadOutput), args.Error(1)
}
func (m *mockS3API) GetObject(ctx context.Context, params *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.GetObjectOutput), args.Error(1)
}
func (m *mockS3API) HeadObject(ctx context.Context, params *awss3.HeadObjectInput, _ ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.HeadObjectOutput), args.Error(1)
}
func (m *mockS3API) ListObjectsV2(ctx context.Context, params *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.ListObjectsV2Output), args.Error(1)
}
func (m *mockS3API) DeleteObject(ctx context.Context, params *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.DeleteObjectOutput), args.Error(1)
}
func (m *mockS3API) CopyObject(ctx context.Context, params *awss3.CopyObjectInput, _ ...func(*awss3.Options)) (*awss3.CopyObjectOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*awss3.CopyObjectOutput), args.Error(1)
}

type mockPresigner struct{ mock.Mock }

func (m *mockPresigner) PresignGetObject(ctx context.Context, params *awss3.GetObjectInput, _ ...func(*awss3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*v4.PresignedHTTPRequest), args.Error(1)
}

func (m *mockPresigner) PresignPutObject(ctx context.Context, params *awss3.PutObjectInput, _ ...func(*awss3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*v4.PresignedHTTPRequest), args.Error(1)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestS3Client(api *mockS3API, presigner *mockPresigner) *S3Client {
	log := &mockLogger{}
	return &S3Client{
		BaseClient:      client.NewBaseClientWithName(client.BaseConfig{}, log, "S3"),
		s3Client:        api,
		transferManager: transfermanager.New(api),
		presigner:       presigner,
		bucket:          "test-bucket",
		region:          "us-east-1",
	}
}

func sampleObjects(n int) []types.Object {
	now := time.Now()
	objs := make([]types.Object, n)
	for i := range objs {
		objs[i] = types.Object{
			Key:          aws.String("key"),
			Size:         aws.Int64(100),
			LastModified: aws.Time(now),
			ETag:         aws.String("etag"),
		}
	}
	return objs
}

// ── NewClient ─────────────────────────────────────────────────────────────────

func TestNewClient(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{Region: "us-east-1", Bucket: "test-bucket"}
	c := NewClient(acf, cfg, &mockLogger{})
	assert.NotNil(t, c)
	assert.IsType(t, &S3Client{}, c)
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{Region: "us-east-1", Bucket: "test-bucket", Timeout: 0}
	c := NewClient(acf, cfg, &mockLogger{})
	assert.NotNil(t, c)
	s3c := c.(*S3Client)
	assert.NotNil(t, s3c.s3Client)
	assert.Equal(t, "test-bucket", s3c.bucket)
}

func TestNewClient_WithLogging(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{Region: "us-east-1", Bucket: "test-bucket", EnableLogging: true}
	log := &mockLogger{}
	log.On("Debug", mock.Anything, "S3 client initialized", mock.Anything).Return()
	c := NewClient(acf, cfg, log)
	assert.NotNil(t, c)
	log.AssertExpectations(t)
}

func TestNewClient_WithResilience(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region:         "us-east-1",
		Bucket:         "test-bucket",
		WithResilience: true,
		Resilience: resilience.Config{
			RetryConfig:          &retry_backoff.Config{MaxRetries: 3},
			CircuitBreakerConfig: &circuit_breaker.Config{Name: "test-cb"},
		},
	}
	c := NewClient(acf, cfg, &mockLogger{})
	assert.NotNil(t, c)
	s3c := c.(*S3Client)
	assert.NotNil(t, s3c.s3Client)
}

// ── PutObject ─────────────────────────────────────────────────────────────────

func TestS3Client_PutObject_InvalidInput(t *testing.T) {
	c := newTestS3Client(&mockS3API{}, &mockPresigner{})
	ctx := context.Background()

	err := c.PutObject(ctx, "", strings.NewReader("body"), "text/plain", nil)
	assert.ErrorIs(t, err, ErrInvalidInput)

	err = c.PutObject(ctx, "key", nil, "text/plain", nil)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestS3Client_PutObject_Success(t *testing.T) {
	api := &mockS3API{}
	api.On("PutObject", mock.Anything, mock.Anything).Return(&awss3.PutObjectOutput{}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	err := c.PutObject(context.Background(), "test-key", strings.NewReader("body"), "text/plain", nil)
	assert.NoError(t, err)
	api.AssertExpectations(t)
}

func TestS3Client_PutObject_WithMetadata_Success(t *testing.T) {
	api := &mockS3API{}
	api.On("PutObject", mock.Anything, mock.Anything).Return(&awss3.PutObjectOutput{}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	err := c.PutObject(context.Background(), "test-key", strings.NewReader("body"), "text/plain", map[string]string{"env": "test"})
	assert.NoError(t, err)
	api.AssertExpectations(t)
}

func TestS3Client_PutObject_Error(t *testing.T) {
	api := &mockS3API{}
	api.On("PutObject", mock.Anything, mock.Anything).Return(nil, errors.New("upload failed"))

	c := newTestS3Client(api, &mockPresigner{})
	err := c.PutObject(context.Background(), "test-key", strings.NewReader("body"), "text/plain", nil)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidInput)
}

// ── GetObject ─────────────────────────────────────────────────────────────────

func TestS3Client_GetObject_InvalidInput(t *testing.T) {
	c := newTestS3Client(&mockS3API{}, &mockPresigner{})
	_, err := c.GetObject(context.Background(), "")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestS3Client_GetObject_Success(t *testing.T) {
	api := &mockS3API{}
	body := io.NopCloser(strings.NewReader("content"))
	api.On("GetObject", mock.Anything, mock.Anything).Return(&awss3.GetObjectOutput{Body: body}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	rc, err := c.GetObject(context.Background(), "test-key")
	assert.NoError(t, err)
	assert.NotNil(t, rc)
	rc.Close()
	api.AssertExpectations(t)
}

func TestS3Client_GetObject_NoSuchKey(t *testing.T) {
	api := &mockS3API{}
	api.On("GetObject", mock.Anything, mock.Anything).Return(nil, &types.NoSuchKey{})

	c := newTestS3Client(api, &mockPresigner{})
	_, err := c.GetObject(context.Background(), "missing-key")
	assert.ErrorIs(t, err, ErrObjectNotFound)
}

func TestS3Client_GetObject_Error(t *testing.T) {
	api := &mockS3API{}
	api.On("GetObject", mock.Anything, mock.Anything).Return(nil, errors.New("network error"))

	c := newTestS3Client(api, &mockPresigner{})
	_, err := c.GetObject(context.Background(), "test-key")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrObjectNotFound)
}

// ── DeleteObject ──────────────────────────────────────────────────────────────

func TestS3Client_DeleteObject_InvalidInput(t *testing.T) {
	c := newTestS3Client(&mockS3API{}, &mockPresigner{})
	err := c.DeleteObject(context.Background(), "")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestS3Client_DeleteObject_Success(t *testing.T) {
	api := &mockS3API{}
	api.On("DeleteObject", mock.Anything, mock.Anything).Return(&awss3.DeleteObjectOutput{}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	err := c.DeleteObject(context.Background(), "test-key")
	assert.NoError(t, err)
	api.AssertExpectations(t)
}

func TestS3Client_DeleteObject_Error(t *testing.T) {
	api := &mockS3API{}
	api.On("DeleteObject", mock.Anything, mock.Anything).Return(nil, errors.New("delete failed"))

	c := newTestS3Client(api, &mockPresigner{})
	err := c.DeleteObject(context.Background(), "test-key")
	assert.Error(t, err)
}

// ── HeadObject ────────────────────────────────────────────────────────────────

func TestS3Client_HeadObject_InvalidInput(t *testing.T) {
	c := newTestS3Client(&mockS3API{}, &mockPresigner{})
	_, err := c.HeadObject(context.Background(), "")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestS3Client_HeadObject_Success(t *testing.T) {
	api := &mockS3API{}
	now := time.Now()
	api.On("HeadObject", mock.Anything, mock.Anything).Return(&awss3.HeadObjectOutput{
		ContentLength: aws.Int64(512),
		LastModified:  aws.Time(now),
		ETag:          aws.String(`"abc123"`),
		ContentType:   aws.String("image/png"),
	}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	meta, err := c.HeadObject(context.Background(), "test-key")
	assert.NoError(t, err)
	assert.Equal(t, "test-key", meta.Key)
	assert.Equal(t, int64(512), meta.Size)
	assert.Equal(t, `"abc123"`, meta.ETag)
	assert.Equal(t, "image/png", meta.ContentType)
	api.AssertExpectations(t)
}

func TestS3Client_HeadObject_NotFound(t *testing.T) {
	api := &mockS3API{}
	api.On("HeadObject", mock.Anything, mock.Anything).Return(nil, &types.NotFound{})

	c := newTestS3Client(api, &mockPresigner{})
	_, err := c.HeadObject(context.Background(), "missing-key")
	assert.ErrorIs(t, err, ErrObjectNotFound)
}

func TestS3Client_HeadObject_Error(t *testing.T) {
	api := &mockS3API{}
	api.On("HeadObject", mock.Anything, mock.Anything).Return(nil, errors.New("head failed"))

	c := newTestS3Client(api, &mockPresigner{})
	_, err := c.HeadObject(context.Background(), "test-key")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrObjectNotFound)
}

// ── ListObjects ───────────────────────────────────────────────────────────────

func TestS3Client_ListObjects_InvalidMaxKeys_DefaultsTo1000(t *testing.T) {
	api := &mockS3API{}
	api.On("ListObjectsV2", mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents: sampleObjects(5),
	}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	objs, err := c.ListObjects(context.Background(), "", 0)
	assert.NoError(t, err)
	assert.Len(t, objs, 5)
}

func TestS3Client_ListObjects_Success(t *testing.T) {
	api := &mockS3API{}
	api.On("ListObjectsV2", mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents: sampleObjects(3),
	}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	objs, err := c.ListObjects(context.Background(), "prefix/", 10)
	assert.NoError(t, err)
	assert.Len(t, objs, 3)
}

func TestS3Client_ListObjects_Pagination(t *testing.T) {
	api := &mockS3API{}
	api.On("ListObjectsV2", mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents:              sampleObjects(2),
		NextContinuationToken: aws.String("token1"),
	}, nil).Once()
	api.On("ListObjectsV2", mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents: sampleObjects(1),
	}, nil).Once()

	c := newTestS3Client(api, &mockPresigner{})
	objs, err := c.ListObjects(context.Background(), "", 10)
	assert.NoError(t, err)
	assert.Len(t, objs, 3)
	api.AssertExpectations(t)
}

func TestS3Client_ListObjects_LimitReached(t *testing.T) {
	api := &mockS3API{}
	api.On("ListObjectsV2", mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents:              sampleObjects(5),
		NextContinuationToken: aws.String("token1"),
	}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	objs, err := c.ListObjects(context.Background(), "", 3)
	assert.NoError(t, err)
	assert.Len(t, objs, 3)
}

func TestS3Client_ListObjects_Error(t *testing.T) {
	api := &mockS3API{}
	api.On("ListObjectsV2", mock.Anything, mock.Anything).Return(nil, errors.New("list failed"))

	c := newTestS3Client(api, &mockPresigner{})
	_, err := c.ListObjects(context.Background(), "", 10)
	assert.Error(t, err)
}

// ── CopyObject ────────────────────────────────────────────────────────────────

func TestS3Client_CopyObject_InvalidInput(t *testing.T) {
	c := newTestS3Client(&mockS3API{}, &mockPresigner{})
	ctx := context.Background()

	err := c.CopyObject(ctx, "", "dest")
	assert.ErrorIs(t, err, ErrInvalidInput)

	err = c.CopyObject(ctx, "src", "")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestS3Client_CopyObject_Success(t *testing.T) {
	api := &mockS3API{}
	api.On("CopyObject", mock.Anything, mock.Anything).Return(&awss3.CopyObjectOutput{}, nil)

	c := newTestS3Client(api, &mockPresigner{})
	err := c.CopyObject(context.Background(), "src-key", "dest-key")
	assert.NoError(t, err)
	api.AssertExpectations(t)
}

func TestS3Client_CopyObject_Error(t *testing.T) {
	api := &mockS3API{}
	api.On("CopyObject", mock.Anything, mock.Anything).Return(nil, errors.New("copy failed"))

	c := newTestS3Client(api, &mockPresigner{})
	err := c.CopyObject(context.Background(), "src-key", "dest-key")
	assert.Error(t, err)
}

// ── GetPresignedURL ───────────────────────────────────────────────────────────

func TestS3Client_GetPresignedURL_InvalidInput(t *testing.T) {
	c := newTestS3Client(&mockS3API{}, &mockPresigner{})
	_, err := c.GetPresignedURL(context.Background(), "", 15*time.Minute)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestS3Client_GetPresignedURL_Success(t *testing.T) {
	presigner := &mockPresigner{}
	presigner.On("PresignGetObject", mock.Anything, mock.Anything).
		Return(&v4.PresignedHTTPRequest{URL: "https://s3.example.com/test-bucket/test-key?sig=abc"}, nil)

	c := newTestS3Client(&mockS3API{}, presigner)
	url, err := c.GetPresignedURL(context.Background(), "test-key", 30*time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "https://s3.example.com/test-bucket/test-key?sig=abc", url)
	presigner.AssertExpectations(t)
}

func TestS3Client_GetPresignedURL_DefaultExpiration(t *testing.T) {
	presigner := &mockPresigner{}
	presigner.On("PresignGetObject", mock.Anything, mock.Anything).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/signed"}, nil)

	c := newTestS3Client(&mockS3API{}, presigner)
	url, err := c.GetPresignedURL(context.Background(), "test-key", 0)
	assert.NoError(t, err)
	assert.NotEmpty(t, url)
	presigner.AssertExpectations(t)
}

func TestS3Client_GetPresignedURL_Error(t *testing.T) {
	presigner := &mockPresigner{}
	presigner.On("PresignGetObject", mock.Anything, mock.Anything).
		Return(nil, errors.New("presign failed"))

	c := newTestS3Client(&mockS3API{}, presigner)
	_, err := c.GetPresignedURL(context.Background(), "test-key", 15*time.Minute)
	assert.Error(t, err)
}

// ── GetPresignedPutURL ────────────────────────────────────────────────────────

func TestS3Client_GetPresignedPutURL_InvalidInput(t *testing.T) {
	c := newTestS3Client(&mockS3API{}, &mockPresigner{})
	_, err := c.GetPresignedPutURL(context.Background(), "", "image/png", 15*time.Minute)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestS3Client_GetPresignedPutURL_Success(t *testing.T) {
	presigner := &mockPresigner{}
	presigner.On("PresignPutObject", mock.Anything, mock.MatchedBy(func(in *awss3.PutObjectInput) bool {
		return in.Key != nil && *in.Key == "uploads/photo.png" &&
			in.ContentType != nil && *in.ContentType == "image/png"
	})).Return(&v4.PresignedHTTPRequest{URL: "https://s3.example.com/test-bucket/uploads/photo.png?sig=put"}, nil)

	c := newTestS3Client(&mockS3API{}, presigner)
	url, err := c.GetPresignedPutURL(context.Background(), "uploads/photo.png", "image/png", 30*time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "https://s3.example.com/test-bucket/uploads/photo.png?sig=put", url)
	presigner.AssertExpectations(t)
}

func TestS3Client_GetPresignedPutURL_DefaultExpiration(t *testing.T) {
	presigner := &mockPresigner{}
	presigner.On("PresignPutObject", mock.Anything, mock.Anything).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/signed-put"}, nil)

	c := newTestS3Client(&mockS3API{}, presigner)
	url, err := c.GetPresignedPutURL(context.Background(), "test-key", "", 0)
	assert.NoError(t, err)
	assert.NotEmpty(t, url)
	presigner.AssertExpectations(t)
}

func TestS3Client_GetPresignedPutURL_Error(t *testing.T) {
	presigner := &mockPresigner{}
	presigner.On("PresignPutObject", mock.Anything, mock.Anything).
		Return(nil, errors.New("presign failed"))

	c := newTestS3Client(&mockS3API{}, presigner)
	_, err := c.GetPresignedPutURL(context.Background(), "test-key", "application/pdf", 15*time.Minute)
	assert.Error(t, err)
}

// ── EnableLogging ─────────────────────────────────────────────────────────────

func TestS3Client_EnableLogging(t *testing.T) {
	c := newTestS3Client(&mockS3API{}, &mockPresigner{})
	c.EnableLogging(true)
	assert.True(t, c.IsLoggingEnabled())
	c.EnableLogging(false)
	assert.False(t, c.IsLoggingEnabled())
}
