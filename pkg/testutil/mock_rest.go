package testutil

import (
	"context"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/mock"
)

// MockRestClient implements rest.Service with testify/mock.
//
// Usage:
//
//	m := testutil.NewMockRestClient()
//	m.On("Get", mock.Anything, "/users/1", mock.Anything).
//	    Return(nil, errors.New("timeout"))
//	defer m.AssertExpectations(t)
type MockRestClient struct {
	mock.Mock
}

// NewMockRestClient creates an empty MockRestClient.
func NewMockRestClient() *MockRestClient { return &MockRestClient{} }

func (m *MockRestClient) Get(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	args := m.Called(ctx, endpoint, headers)
	resp, _ := args.Get(0).(*resty.Response)
	return resp, args.Error(1)
}

func (m *MockRestClient) Post(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	args := m.Called(ctx, endpoint, body, headers)
	resp, _ := args.Get(0).(*resty.Response)
	return resp, args.Error(1)
}

func (m *MockRestClient) Put(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	args := m.Called(ctx, endpoint, body, headers)
	resp, _ := args.Get(0).(*resty.Response)
	return resp, args.Error(1)
}

func (m *MockRestClient) Patch(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	args := m.Called(ctx, endpoint, body, headers)
	resp, _ := args.Get(0).(*resty.Response)
	return resp, args.Error(1)
}

func (m *MockRestClient) Delete(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	args := m.Called(ctx, endpoint, headers)
	resp, _ := args.Get(0).(*resty.Response)
	return resp, args.Error(1)
}

func (m *MockRestClient) WithLogging(enable bool) {
	m.Called(enable)
}
