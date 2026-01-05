package s3

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockLogger struct {
	mock.Mock
}

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

func TestNewClient(t *testing.T) {
	acf := aws.Config{
		Region: "us-east-1",
	}
	cfg := Config{
		Region:         "us-east-1",
		Bucket:         "test-bucket",
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        0,
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	assert.NotNil(t, client)
	assert.IsType(t, &S3Client{}, client)
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	acf := aws.Config{
		Region: "us-east-1",
	}
	cfg := Config{
		Region:  "us-east-1",
		Bucket:  "test-bucket",
		Timeout: 0,
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	assert.NotNil(t, client)
	s3Client := client.(*S3Client)
	// Verificar que el cliente se creó correctamente
	assert.NotNil(t, s3Client.s3Client)
	assert.Equal(t, "test-bucket", s3Client.bucket)
}

func TestNewClient_WithLogging(t *testing.T) {
	acf := aws.Config{
		Region: "us-east-1",
	}
	cfg := Config{
		Region:        "us-east-1",
		Bucket:        "test-bucket",
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Debug", mock.Anything, "S3 client initialized", mock.Anything).Return()

	client := NewClient(acf, cfg, log)

	assert.NotNil(t, client)
	log.AssertExpectations(t)
}

func TestNewClient_WithResilience(t *testing.T) {
	acf := aws.Config{
		Region: "us-east-1",
	}
	cfg := Config{
		Region:         "us-east-1",
		Bucket:         "test-bucket",
		WithResilience: true,
		Resilience: resilience.Config{
			RetryConfig: &retry_backoff.Config{
				MaxRetries: 3,
			},
			CircuitBreakerConfig: &circuit_breaker.Config{
				Name: "test-cb",
			},
		},
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	assert.NotNil(t, client)
	s3Client := client.(*S3Client)
	// Verificar que el cliente se creó correctamente
	assert.NotNil(t, s3Client.s3Client)
	assert.Equal(t, "test-bucket", s3Client.bucket)
}

func TestS3Client_PutObject_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	err := client.PutObject(ctx, "", strings.NewReader("body"), "text/plain", nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)

	err = client.PutObject(ctx, "key", nil, "text/plain", nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestS3Client_GetObject_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.GetObject(ctx, "")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestS3Client_DeleteObject_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	err := client.DeleteObject(ctx, "")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestS3Client_HeadObject_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.HeadObject(ctx, "")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestS3Client_CopyObject_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	err := client.CopyObject(ctx, "", "dest-key")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)

	err = client.CopyObject(ctx, "source-key", "")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestS3Client_GetPresignedURL_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.GetPresignedURL(ctx, "", 15*time.Minute)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestS3Client_EnableLogging(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region:        "us-east-1",
		Bucket:        "test-bucket",
		EnableLogging: false,
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)
	client.EnableLogging(true)

	s3Client := client.(*S3Client)
	assert.True(t, s3Client.IsLoggingEnabled())
}

func TestS3Client_PutObject_WithMetadata(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	metadata := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	// This will fail without a real AWS connection
	err := client.PutObject(ctx, "test-key", strings.NewReader("body"), "text/plain", metadata)
	assert.Error(t, err)
	assert.NotEqual(t, ErrInvalidInput, err)
}

func TestS3Client_ListObjects(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	// This will fail without a real AWS connection
	_, err := client.ListObjects(ctx, "prefix", 10)
	assert.Error(t, err)
}

func TestS3Client_ListObjects_EmptyPrefix(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		Region: "us-east-1",
		Bucket: "test-bucket",
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.ListObjects(ctx, "", 0)
	assert.Error(t, err)
}
