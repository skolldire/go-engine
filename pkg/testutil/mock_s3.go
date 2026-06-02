package testutil

import (
	"context"
	"io"
	"time"

	s3client "github.com/skolldire/go-engine/aws/pkg/clients/s3"
	"github.com/stretchr/testify/mock"
)

// MockS3Client implements s3.Service with testify/mock.
//
// Usage:
//
//	m := testutil.NewMockS3Client()
//	m.On("PutObject", mock.Anything, "docs/file.pdf", mock.Anything, "application/pdf", mock.Anything).
//	    Return(nil)
//	defer m.AssertExpectations(t)
type MockS3Client struct {
	mock.Mock
}

// NewMockS3Client creates an empty MockS3Client.
func NewMockS3Client() *MockS3Client { return &MockS3Client{} }

func (m *MockS3Client) PutObject(ctx context.Context, key string, body io.Reader, contentType string, metadata map[string]string) error {
	return m.Called(ctx, key, body, contentType, metadata).Error(0)
}

func (m *MockS3Client) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	rc, _ := args.Get(0).(io.ReadCloser)
	return rc, args.Error(1)
}

func (m *MockS3Client) DeleteObject(ctx context.Context, key string) error {
	return m.Called(ctx, key).Error(0)
}

func (m *MockS3Client) HeadObject(ctx context.Context, key string) (*s3client.ObjectMetadata, error) {
	args := m.Called(ctx, key)
	meta, _ := args.Get(0).(*s3client.ObjectMetadata)
	return meta, args.Error(1)
}

func (m *MockS3Client) ListObjects(ctx context.Context, prefix string, maxKeys int32) ([]s3client.ObjectMetadata, error) {
	args := m.Called(ctx, prefix, maxKeys)
	objs, _ := args.Get(0).([]s3client.ObjectMetadata)
	return objs, args.Error(1)
}

func (m *MockS3Client) CopyObject(ctx context.Context, sourceKey, destKey string) error {
	return m.Called(ctx, sourceKey, destKey).Error(0)
}

func (m *MockS3Client) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, expiration)
	return args.String(0), args.Error(1)
}

func (m *MockS3Client) EnableLogging(enable bool) {
	m.Called(enable)
}

// AssertUploaded verifies that PutObject was called with the given key.
func (m *MockS3Client) AssertUploaded(t interface{ Errorf(string, ...interface{}) }, key string) {
	for _, call := range m.Calls {
		if call.Method == "PutObject" {
			if k, ok := call.Arguments[1].(string); ok && k == key {
				return
			}
		}
	}
	t.Errorf("expected PutObject with key %q — not found in recorded calls", key)
}
