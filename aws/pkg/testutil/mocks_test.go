package testutil_test

import (
	"context"
	"testing"
	"time"

	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/skolldire/go-engine/aws/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ── MockSQSClient ─────────────────────────────────────────────────────────────

func TestMockSQSClient_SendJSON(t *testing.T) {
	m := testutil.NewMockSQSClient()
	m.On("SendJSON", mock.Anything, "https://sqs.../queue",
		mock.Anything, mock.Anything).Return("msg-123", nil)
	defer m.AssertExpectations(t)

	id, err := m.SendJSON(context.Background(), "https://sqs.../queue",
		map[string]string{"event": "created"},
		map[string]sqstypes.MessageAttributeValue{})
	assert.NoError(t, err)
	assert.Equal(t, "msg-123", id)
}

func TestMockSQSClient_DeleteMessage(t *testing.T) {
	m := testutil.NewMockSQSClient()
	m.On("DeleteMessage", mock.Anything, "https://sqs.../queue", "receipt-abc").
		Return(nil)
	defer m.AssertExpectations(t)

	err := m.DeleteMessage(context.Background(), "https://sqs.../queue", "receipt-abc")
	assert.NoError(t, err)
}

func TestMockSQSClient_ListQueue(t *testing.T) {
	m := testutil.NewMockSQSClient()
	m.On("ListQueue", mock.Anything, "my-prefix").
		Return([]string{"q1", "q2"}, nil)
	defer m.AssertExpectations(t)

	urls, err := m.ListQueue(context.Background(), "my-prefix")
	assert.NoError(t, err)
	assert.Equal(t, []string{"q1", "q2"}, urls)
}

// ── MockS3Client ──────────────────────────────────────────────────────────────

func TestMockS3Client_PutObject(t *testing.T) {
	m := testutil.NewMockS3Client()
	m.On("PutObject", mock.Anything, "docs/file.pdf",
		mock.Anything, "application/pdf", mock.Anything).Return(nil)
	defer m.AssertExpectations(t)

	err := m.PutObject(context.Background(), "docs/file.pdf", nil, "application/pdf", nil)
	require.NoError(t, err)
	m.AssertUploaded(t, "docs/file.pdf")
}

func TestMockS3Client_AssertUploaded_Fails(t *testing.T) {
	m := testutil.NewMockS3Client()
	m.On("PutObject", mock.Anything, "docs/other.pdf",
		mock.Anything, mock.Anything, mock.Anything).Return(nil)
	defer m.AssertExpectations(t)

	_ = m.PutObject(context.Background(), "docs/other.pdf", nil, "application/pdf", nil)

	failed := false
	m.AssertUploaded(&fakeT{fail: func() { failed = true }}, "docs/not-uploaded.pdf")
	assert.True(t, failed)
}

func TestMockS3Client_GetPresignedURL(t *testing.T) {
	m := testutil.NewMockS3Client()
	m.On("GetPresignedURL", mock.Anything, "assets/img.png", 15*time.Minute).
		Return("https://signed-url.example.com/img.png", nil)
	defer m.AssertExpectations(t)

	url, err := m.GetPresignedURL(context.Background(), "assets/img.png", 15*time.Minute)
	assert.NoError(t, err)
	assert.Contains(t, url, "signed-url")
}

// ── helpers ───────────────────────────────────────────────────────────────────

type fakeT struct {
	fail func()
}

func (f *fakeT) Errorf(_ string, _ ...any) { f.fail() }
