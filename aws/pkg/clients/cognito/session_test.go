package cognito

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestClient_SignOut_InvalidToken(t *testing.T) {
	cfg := Config{
		Region:        "us-east-1",
		UserPoolID:    "us-east-1_TestPool123",
		ClientID:      "test-client-id",
		EnableLogging: false,
	}
	log := &mockLogger{}

	client, _ := NewClient(cfg, log)
	assert.NotNil(t, client)

	ctx := context.Background()

	tests := []struct {
		name        string
		accessToken string
	}{
		{
			name:        "empty access token",
			accessToken: "",
		},
		{
			name:        "invalid token format",
			accessToken: "invalid.token.format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SignOut(ctx, tt.accessToken)
			assert.Error(t, err)
		})
	}
}

func TestClient_GlobalSignOut_InvalidToken(t *testing.T) {
	cfg := Config{
		Region:        "us-east-1",
		UserPoolID:    "us-east-1_TestPool123",
		ClientID:      "test-client-id",
		EnableLogging: false,
	}
	log := &mockLogger{}

	client, _ := NewClient(cfg, log)
	assert.NotNil(t, client)

	ctx := context.Background()

	tests := []struct {
		name        string
		accessToken string
	}{
		{
			name:        "empty access token",
			accessToken: "",
		},
		{
			name:        "invalid token format",
			accessToken: "invalid.token.format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.GlobalSignOut(ctx, tt.accessToken)
			assert.Error(t, err)
		})
	}
}

func TestClient_SignOut_WithLogging(t *testing.T) {
	cfg := Config{
		Region:        "us-east-1",
		UserPoolID:    "us-east-1_TestPool123",
		ClientID:      "test-client-id",
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()
	log.On("Info", mock.Anything, mock.Anything, mock.Anything).Return()

	client, _ := NewClient(cfg, log)
	assert.NotNil(t, client)

	ctx := context.Background()

	err := client.SignOut(ctx, "invalid-token")
	assert.Error(t, err)
}

func TestClient_GlobalSignOut_WithLogging(t *testing.T) {
	cfg := Config{
		Region:        "us-east-1",
		UserPoolID:    "us-east-1_TestPool123",
		ClientID:      "test-client-id",
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()
	log.On("Info", mock.Anything, mock.Anything, mock.Anything).Return()

	client, _ := NewClient(cfg, log)
	assert.NotNil(t, client)

	ctx := context.Background()

	err := client.GlobalSignOut(ctx, "invalid-token")
	assert.Error(t, err)
}
