package cognito

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClient_AssociateSoftwareToken_InvalidToken(t *testing.T) {
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
			_, err := client.AssociateSoftwareToken(ctx, tt.accessToken)
			assert.Error(t, err)
		})
	}
}

func TestClient_VerifySoftwareToken_InvalidRequest(t *testing.T) {
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
		userCode    string
		session     string
	}{
		{
			name:        "empty access token",
			accessToken: "",
			userCode:    "123456",
			session:     "session-token",
		},
		{
			name:        "empty user code",
			accessToken: "valid-token",
			userCode:    "",
			session:     "session-token",
		},
		{
			name:        "empty session",
			accessToken: "valid-token",
			userCode:    "123456",
			session:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.VerifySoftwareToken(ctx, tt.accessToken, tt.userCode, tt.session)
			assert.Error(t, err)
		})
	}
}

func TestClient_SetUserMFAPreference_InvalidToken(t *testing.T) {
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
			err := client.SetUserMFAPreference(ctx, tt.accessToken, true, false)
			assert.Error(t, err)
		})
	}
}

func TestClient_GetUserMFAStatus_InvalidToken(t *testing.T) {
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
			_, err := client.GetUserMFAStatus(ctx, tt.accessToken)
			assert.Error(t, err)
		})
	}
}

func TestClient_SetUserMFAPreference_Combinations(t *testing.T) {
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
		smsEnabled  bool
		totpEnabled bool
	}{
		{
			name:        "both enabled",
			accessToken: "valid-token",
			smsEnabled:  true,
			totpEnabled: true,
		},
		{
			name:        "only SMS enabled",
			accessToken: "valid-token",
			smsEnabled:  true,
			totpEnabled: false,
		},
		{
			name:        "only TOTP enabled",
			accessToken: "valid-token",
			smsEnabled:  false,
			totpEnabled: true,
		},
		{
			name:        "both disabled",
			accessToken: "valid-token",
			smsEnabled:  false,
			totpEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetUserMFAPreference(ctx, tt.accessToken, tt.smsEnabled, tt.totpEnabled)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid token")
		})
	}
}
