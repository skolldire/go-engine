package cognito

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Region:     "us-east-1",
				UserPoolID: "us-east-1_TestPool123",
				ClientID:   "test-client-id",
			},
			wantErr: false,
		},
		{
			name: "missing region",
			cfg: Config{
				UserPoolID: "us-east-1_TestPool123",
				ClientID:   "test-client-id",
			},
			wantErr: true,
		},
		{
			name: "missing user_pool_id",
			cfg: Config{
				Region:   "us-east-1",
				ClientID: "test-client-id",
			},
			wantErr: true,
		},
		{
			name: "missing client_id",
			cfg: Config{
				Region:     "us-east-1",
				UserPoolID: "us-east-1_TestPool123",
			},
			wantErr: true,
		},
		{
			name: "unresolved secret variable",
			cfg: Config{
				Region:       "us-east-1",
				UserPoolID:   "us-east-1_TestPool123",
				ClientID:     "test-client-id",
				ClientSecret: "${UNRESOLVED_VAR}",
			},
			wantErr: true,
		},
		{
			name: "resolved secret variable",
			cfg: Config{
				Region:       "us-east-1",
				UserPoolID:   "us-east-1_TestPool123",
				ClientID:     "test-client-id",
				ClientSecret: "resolved-secret",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRegisterRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     RegisterUserRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: RegisterUserRequest{
				Username: "testuser",
				Email:    "test@example.com",
				Password: "password123",
			},
			wantErr: false,
		},
		{
			name: "missing username",
			req: RegisterUserRequest{
				Email:    "test@example.com",
				Password: "password123",
			},
			wantErr: true,
		},
		{
			name: "missing email",
			req: RegisterUserRequest{
				Username: "testuser",
				Password: "password123",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			req: RegisterUserRequest{
				Username: "testuser",
				Email:    "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			req: RegisterUserRequest{
				Username: "testuser",
				Email:    "invalid-email",
				Password: "password123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegisterRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAuthenticateRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     AuthenticateRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: AuthenticateRequest{
				Username: "testuser",
				Password: "password123",
			},
			wantErr: false,
		},
		{
			name: "missing username",
			req: AuthenticateRequest{
				Password: "password123",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			req: AuthenticateRequest{
				Username: "testuser",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuthenticateRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMFAChallengeRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     MFAChallengeRequest
		wantErr bool
	}{
		{
			name: "valid SMS request",
			req: MFAChallengeRequest{
				Username:      "testuser",
				SessionToken:  "session-token",
				MFACode:       "123456",
				ChallengeType: MFAChallengeTypeSMS,
			},
			wantErr: false,
		},
		{
			name: "valid TOTP request",
			req: MFAChallengeRequest{
				Username:      "testuser",
				SessionToken:  "session-token",
				MFACode:       "123456",
				ChallengeType: MFAChallengeTypeSoftwareToken,
			},
			wantErr: false,
		},
		{
			name: "missing username",
			req: MFAChallengeRequest{
				SessionToken:  "session-token",
				MFACode:       "123456",
				ChallengeType: MFAChallengeTypeSMS,
			},
			wantErr: true,
		},
		{
			name: "missing session token",
			req: MFAChallengeRequest{
				Username:      "testuser",
				MFACode:       "123456",
				ChallengeType: MFAChallengeTypeSMS,
			},
			wantErr: true,
		},
		{
			name: "missing mfa code",
			req: MFAChallengeRequest{
				Username:      "testuser",
				SessionToken:  "session-token",
				ChallengeType: MFAChallengeTypeSMS,
			},
			wantErr: true,
		},
		{
			name: "invalid challenge type",
			req: MFAChallengeRequest{
				Username:      "testuser",
				SessionToken:  "session-token",
				MFACode:       "123456",
				ChallengeType: MFAChallengeType("INVALID"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMFAChallengeRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestComputeSecretHash(t *testing.T) {
	clientID := "test-client-id"
	clientSecret := "test-secret"
	username := "testuser"

	hash := computeSecretHash(clientID, clientSecret, username)

	// Verificar que el hash no está vacío
	assert.NotEmpty(t, hash)

	// Verificar que el hash es diferente para diferentes inputs
	hash2 := computeSecretHash(clientID, clientSecret, "different-user")
	assert.NotEqual(t, hash, hash2)

	// Verificar que el hash es el mismo para los mismos inputs
	hash3 := computeSecretHash(clientID, clientSecret, username)
	assert.Equal(t, hash, hash3)

	// Verificar que sin secret retorna string vacío
	emptyHash := computeSecretHash(clientID, "", username)
	assert.Empty(t, emptyHash)
}

func TestGetStringClaim(t *testing.T) {
	claims := map[string]interface{}{
		"sub":    "user-id-123",
		"email":  "test@example.com",
		"number": 123,
	}

	assert.Equal(t, "user-id-123", getStringClaim(claims, "sub"))
	assert.Equal(t, "test@example.com", getStringClaim(claims, "email"))
	assert.Equal(t, "", getStringClaim(claims, "number")) // No es string
	assert.Equal(t, "", getStringClaim(claims, "missing"))
}

func TestGetBoolClaim(t *testing.T) {
	claims := map[string]interface{}{
		"verified":     true,
		"enabled":      false,
		"string_true":  "true",
		"string_false": "false",
		"number":       123,
	}

	assert.True(t, getBoolClaim(claims, "verified"))
	assert.False(t, getBoolClaim(claims, "enabled"))
	assert.True(t, getBoolClaim(claims, "string_true"))
	assert.False(t, getBoolClaim(claims, "string_false"))
	assert.False(t, getBoolClaim(claims, "number"))
	assert.False(t, getBoolClaim(claims, "missing"))
}

func TestGetStringSliceClaim(t *testing.T) {
	claims := map[string]interface{}{
		"groups": []interface{}{"admin", "user", "guest"},
		"mixed":  []interface{}{"string", 123, "another"},
		"number": 123,
	}

	result := getStringSliceClaim(claims, "groups")
	assert.Equal(t, []string{"admin", "user", "guest"}, result)

	result2 := getStringSliceClaim(claims, "mixed")
	assert.Equal(t, []string{"string", "another"}, result2) // Solo strings

	result3 := getStringSliceClaim(claims, "number")
	assert.Nil(t, result3)

	result4 := getStringSliceClaim(claims, "missing")
	assert.Nil(t, result4)
}

func TestGetFloat64Claim(t *testing.T) {
	claims := map[string]interface{}{
		"exp":    1234567890.0,
		"iat":    float64(1234567890),
		"string": "not-a-number",
	}

	assert.Equal(t, 1234567890.0, getFloat64Claim(claims, "exp"))
	assert.Equal(t, float64(1234567890), getFloat64Claim(claims, "iat"))
	assert.Equal(t, 0.0, getFloat64Claim(claims, "string"))
	assert.Equal(t, 0.0, getFloat64Claim(claims, "missing"))
}
