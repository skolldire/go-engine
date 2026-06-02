package testutil

import (
	"context"

	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/mock"
)

// MockSQSClient implements sqs.Service with testify/mock.
//
// Usage:
//
//	m := testutil.NewMockSQSClient()
//	m.On("SendJSON", mock.Anything, queueURL, mock.Anything, mock.Anything).
//	    Return("msg-id-123", nil)
//	defer m.AssertExpectations(t)
type MockSQSClient struct {
	mock.Mock
}

// NewMockSQSClient creates an empty MockSQSClient.
func NewMockSQSClient() *MockSQSClient { return &MockSQSClient{} }

func (m *MockSQSClient) SendMsj(ctx context.Context, queueURL, mensaje string, atributos map[string]sqstypes.MessageAttributeValue) (string, error) {
	args := m.Called(ctx, queueURL, mensaje, atributos)
	return args.String(0), args.Error(1)
}

func (m *MockSQSClient) SendJSON(ctx context.Context, queueURL string, mensaje interface{}, atributos map[string]sqstypes.MessageAttributeValue) (string, error) {
	args := m.Called(ctx, queueURL, mensaje, atributos)
	return args.String(0), args.Error(1)
}

func (m *MockSQSClient) ReceiveMsj(ctx context.Context, queueURL string, maxMensajes, tiempoEspera int32) ([]sqstypes.Message, error) {
	args := m.Called(ctx, queueURL, maxMensajes, tiempoEspera)
	msgs, _ := args.Get(0).([]sqstypes.Message)
	return msgs, args.Error(1)
}

func (m *MockSQSClient) DeleteMsj(ctx context.Context, queueURL, receiptHandle string) error {
	return m.Called(ctx, queueURL, receiptHandle).Error(0)
}

func (m *MockSQSClient) CreateQueue(ctx context.Context, nombre string, atributos map[string]string) (string, error) {
	args := m.Called(ctx, nombre, atributos)
	return args.String(0), args.Error(1)
}

func (m *MockSQSClient) DeleteQueue(ctx context.Context, queueURL string) error {
	return m.Called(ctx, queueURL).Error(0)
}

func (m *MockSQSClient) ListQueue(ctx context.Context, prefijo string) ([]string, error) {
	args := m.Called(ctx, prefijo)
	urls, _ := args.Get(0).([]string)
	return urls, args.Error(1)
}

func (m *MockSQSClient) GetURLQueue(ctx context.Context, nombre string) (string, error) {
	args := m.Called(ctx, nombre)
	return args.String(0), args.Error(1)
}

func (m *MockSQSClient) EnableLogging(activar bool) {
	m.Called(activar)
}
