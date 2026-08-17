package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/skolldire/go-engine/http/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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
