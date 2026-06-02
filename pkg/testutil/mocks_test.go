package testutil_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ── MockRedisClient ───────────────────────────────────────────────────────────

func TestMockRedisClient_SetAndGet(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.On("Set", mock.Anything, "key1", "value1", time.Minute).Return(nil)
	m.On("Get", mock.Anything, "key1").Return("value1", nil)
	defer m.AssertExpectations(t)

	err := m.Set(context.Background(), "key1", "value1", time.Minute)
	assert.NoError(t, err)

	val, err := m.Get(context.Background(), "key1")
	assert.NoError(t, err)
	assert.Equal(t, "value1", val)
}

func TestMockRedisClient_SetupKeyNotFound(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.SetupKeyNotFound("missing")
	defer m.AssertExpectations(t)

	_, err := m.Get(context.Background(), "missing")
	assert.Error(t, err)
}

func TestMockRedisClient_SetupGetReturn(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.SetupGetReturn("key", "cached-value")
	defer m.AssertExpectations(t)

	val, err := m.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "cached-value", val)
}

func TestMockRedisClient_SetupSetOK(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.SetupSetOK()
	defer m.AssertExpectations(t)

	err := m.Set(context.Background(), "any", "value", time.Hour)
	assert.NoError(t, err)
}

func TestMockRedisClient_Ping(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.On("Ping", mock.Anything).Return(nil)
	defer m.AssertExpectations(t)

	assert.NoError(t, m.Ping(context.Background()))
}

func TestMockRedisClient_Del(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.On("Del", mock.Anything, "k1", "k2").Return(int64(2), nil)
	defer m.AssertExpectations(t)

	n, err := m.Del(context.Background(), "k1", "k2")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestMockRedisClient_HGetAll(t *testing.T) {
	m := testutil.NewMockRedisClient()
	expected := map[string]string{"field": "value"}
	m.On("HGetAll", mock.Anything, "hash-key").Return(expected, nil)
	defer m.AssertExpectations(t)

	result, err := m.HGetAll(context.Background(), "hash-key")
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestMockRedisClient_SMembers(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.On("SMembers", mock.Anything, "set-key").Return([]string{"a", "b"}, nil)
	defer m.AssertExpectations(t)

	members, err := m.SMembers(context.Background(), "set-key")
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, members)
}

// ── MockRestClient ────────────────────────────────────────────────────────────

func TestMockRestClient_GetError(t *testing.T) {
	m := testutil.NewMockRestClient()
	m.On("Get", mock.Anything, "/items/1", mock.Anything).
		Return(nil, errors.New("timeout"))
	defer m.AssertExpectations(t)

	_, err := m.Get(context.Background(), "/items/1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestMockRestClient_Post(t *testing.T) {
	m := testutil.NewMockRestClient()
	m.On("Post", mock.Anything, "/items", mock.Anything, mock.Anything).
		Return(nil, nil)
	defer m.AssertExpectations(t)

	resp, err := m.Post(context.Background(), "/items", map[string]string{"name": "test"}, nil)
	assert.NoError(t, err)
	assert.Nil(t, resp)
}

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

func TestMockSQSClient_DeleteMsj(t *testing.T) {
	m := testutil.NewMockSQSClient()
	m.On("DeleteMsj", mock.Anything, "https://sqs.../queue", "receipt-abc").
		Return(nil)
	defer m.AssertExpectations(t)

	err := m.DeleteMsj(context.Background(), "https://sqs.../queue", "receipt-abc")
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

// ── context helpers ───────────────────────────────────────────────────────────

func TestNewTestContext_ClaimsReadable(t *testing.T) {
	ctx := testutil.NewTestContext(&router.Claims{
		Sub:    "user-abc",
		Email:  "dev@example.com",
		Groups: []string{"admins"},
	})

	claims := router.ClaimsFromContext(ctx)
	require.NotNil(t, claims)
	assert.Equal(t, "user-abc", claims.Sub)
	assert.Equal(t, "dev@example.com", claims.Email)
	assert.Contains(t, claims.Groups, "admins")
}

func TestNewEmptyTestContext_NoClaims(t *testing.T) {
	ctx := testutil.NewEmptyTestContext()
	assert.Nil(t, router.ClaimsFromContext(ctx))
}

// ── helpers ───────────────────────────────────────────────────────────────────

type fakeT struct {
	fail func()
}

func (f *fakeT) Errorf(_ string, _ ...interface{}) { f.fail() }
