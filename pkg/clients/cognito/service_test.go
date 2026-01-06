package cognito

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
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
func (m *mockLogger) WrapError(err error, msg string) error {
	args := m.Called(err, msg)
	if args.Get(0) != nil {
		return args.Get(0).(error)
	}
	return err
}
func (m *mockLogger) WithField(key string, value interface{}) logger.Service  { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service { return m }
func (m *mockLogger) GetLogLevel() string                                     { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                          { return nil }

func TestNewClient(t *testing.T) {
	cfg := Config{
		Region:         "us-east-1",
		UserPoolID:     "us-east-1_TestPool123",
		ClientID:       "test-client-id",
		ClientSecret:   "",
		EnableLogging:  false,
		WithResilience: false,
	}
	log := &mockLogger{}

	client, err := NewClient(cfg, log)

	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.IsType(t, &Client{}, client)
}

func TestNewClient_WithSecret(t *testing.T) {
	cfg := Config{
		Region:         "us-east-1",
		UserPoolID:     "us-east-1_TestPool123",
		ClientID:       "test-client-id",
		ClientSecret:   "test-secret",
		EnableLogging:  false,
		WithResilience: false,
	}
	log := &mockLogger{}

	client, err := NewClient(cfg, log)

	assert.NoError(t, err)
	assert.NotNil(t, client)
	// Verificar que el cliente se creó correctamente
	// Nota: cfg.ClientSecret no se limpia porque se pasa por valor,
	// pero el secret se copió al campo privado del cliente
	cognitoClient := client.(*Client)
	assert.NotEmpty(t, cognitoClient.clientSecret) // Verificar que el secret está en el cliente
}

func TestNewClient_WithLogging(t *testing.T) {
	cfg := Config{
		Region:         "us-east-1",
		UserPoolID:     "us-east-1_TestPool123",
		ClientID:       "test-client-id",
		EnableLogging:  true,
		WithResilience: false,
	}
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	client, err := NewClient(cfg, log)

	assert.NoError(t, err)
	assert.NotNil(t, client)
	log.AssertExpectations(t)
}

func TestNewClient_WithResilience(t *testing.T) {
	cfg := Config{
		Region:         "us-east-1",
		UserPoolID:     "us-east-1_TestPool123",
		ClientID:       "test-client-id",
		EnableLogging:  false,
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

	client, err := NewClient(cfg, log)

	assert.NoError(t, err)
	assert.NotNil(t, client)
	cognitoClient := client.(*Client)
	assert.NotNil(t, cognitoClient.resilience)
}

func TestNewClient_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "missing region",
			cfg: Config{
				UserPoolID: "us-east-1_TestPool123",
				ClientID:   "test-client-id",
			},
		},
		{
			name: "missing user_pool_id",
			cfg: Config{
				Region:   "us-east-1",
				ClientID: "test-client-id",
			},
		},
		{
			name: "missing client_id",
			cfg: Config{
				Region:     "us-east-1",
				UserPoolID: "us-east-1_TestPool123",
			},
		},
		{
			name: "unresolved secret variable",
			cfg: Config{
				Region:       "us-east-1",
				UserPoolID:   "us-east-1_TestPool123",
				ClientID:     "test-client-id",
				ClientSecret: "${UNRESOLVED_VAR}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &mockLogger{}
			client, err := NewClient(tt.cfg, log)
			assert.Error(t, err)
			assert.Nil(t, client)
		})
	}
}

func TestClient_RegisterUser_InvalidRequest(t *testing.T) {
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
		name string
		req  RegisterUserRequest
	}{
		{
			name: "missing username",
			req: RegisterUserRequest{
				Email:    "test@example.com",
				Password: "password123",
			},
		},
		{
			name: "missing email",
			req: RegisterUserRequest{
				Username: "testuser",
				Password: "password123",
			},
		},
		{
			name: "missing password",
			req: RegisterUserRequest{
				Username: "testuser",
				Email:    "test@example.com",
			},
		},
		{
			name: "invalid email format",
			req: RegisterUserRequest{
				Username: "testuser",
				Email:    "invalid-email",
				Password: "password123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.RegisterUser(ctx, tt.req)
			assert.Error(t, err)
		})
	}
}

func TestClient_Authenticate_InvalidRequest(t *testing.T) {
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
		name string
		req  AuthenticateRequest
	}{
		{
			name: "missing username",
			req: AuthenticateRequest{
				Password: "password123",
			},
		},
		{
			name: "missing password",
			req: AuthenticateRequest{
				Username: "testuser",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Authenticate(ctx, tt.req)
			assert.Error(t, err)
		})
	}
}

func TestClient_ConfirmSignUp_InvalidRequest(t *testing.T) {
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
		name string
		req  ConfirmSignUpRequest
	}{
		{
			name: "missing username",
			req: ConfirmSignUpRequest{
				ConfirmationCode: "123456",
			},
		},
		{
			name: "missing confirmation code",
			req: ConfirmSignUpRequest{
				Username: "testuser",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.ConfirmSignUp(ctx, tt.req)
			assert.Error(t, err)
		})
	}
}

func TestClient_RespondToMFAChallenge_InvalidRequest(t *testing.T) {
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
		name string
		req  MFAChallengeRequest
	}{
		{
			name: "missing username",
			req: MFAChallengeRequest{
				SessionToken:  "session-token",
				MFACode:       "123456",
				ChallengeType: MFAChallengeTypeSMS,
			},
		},
		{
			name: "missing session token",
			req: MFAChallengeRequest{
				Username:      "testuser",
				MFACode:       "123456",
				ChallengeType: MFAChallengeTypeSMS,
			},
		},
		{
			name: "missing mfa code",
			req: MFAChallengeRequest{
				Username:      "testuser",
				SessionToken:  "session-token",
				ChallengeType: MFAChallengeTypeSMS,
			},
		},
		{
			name: "invalid challenge type",
			req: MFAChallengeRequest{
				Username:      "testuser",
				SessionToken:  "session-token",
				MFACode:       "123456",
				ChallengeType: MFAChallengeType("INVALID"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.RespondToMFAChallenge(ctx, tt.req)
			assert.Error(t, err)
		})
	}
}

func TestClient_ValidateToken_InvalidToken(t *testing.T) {
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
		name  string
		token string
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "invalid token format",
			token: "not.a.valid.jwt.token",
		},
		{
			name:  "malformed token",
			token: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ValidateToken(ctx, tt.token)
			assert.Error(t, err)
		})
	}
}

func TestClient_GetUserByAccessToken_InvalidToken(t *testing.T) {
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

	_, err := client.GetUserByAccessToken(ctx, "")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidAccessToken, err)
}

func TestClient_RefreshToken_InvalidRequest(t *testing.T) {
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

	_, err := client.RefreshToken(ctx, RefreshTokenRequest{
		RefreshToken: "",
	})
	assert.Error(t, err)
}

func TestClient_ForgotPassword_InvalidRequest(t *testing.T) {
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

	err := client.ForgotPassword(ctx, ForgotPasswordRequest{
		Username: "",
	})
	assert.Error(t, err)
}

func TestClient_ConfirmForgotPassword_InvalidRequest(t *testing.T) {
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
		name string
		req  ConfirmForgotPasswordRequest
	}{
		{
			name: "missing username",
			req: ConfirmForgotPasswordRequest{
				ConfirmationCode: "123456",
				NewPassword:      "newpass123",
			},
		},
		{
			name: "missing confirmation code",
			req: ConfirmForgotPasswordRequest{
				Username:    "testuser",
				NewPassword: "newpass123",
			},
		},
		{
			name: "missing new password",
			req: ConfirmForgotPasswordRequest{
				Username:         "testuser",
				ConfirmationCode: "123456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.ConfirmForgotPassword(ctx, tt.req)
			assert.Error(t, err)
		})
	}
}

func TestHandleCognitoError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode string
		expectedType string
	}{
		{
			name:         "NotAuthorizedException",
			err:          &types.NotAuthorizedException{Message: aws.String("Not authorized")},
			expectedCode: "NotAuthorized",
			expectedType: "*cognito.CognitoError",
		},
		{
			name:         "InvalidParameterException",
			err:          &types.InvalidParameterException{Message: aws.String("Invalid parameter")},
			expectedCode: "InvalidParameter",
			expectedType: "*cognito.CognitoError",
		},
		{
			name:         "ResourceNotFoundException",
			err:          &types.ResourceNotFoundException{Message: aws.String("Not found")},
			expectedCode: "ResourceNotFound",
			expectedType: "*cognito.CognitoError",
		},
		{
			name:         "UsernameExistsException",
			err:          &types.UsernameExistsException{Message: aws.String("Username exists")},
			expectedCode: "UsernameExists",
			expectedType: "*cognito.CognitoError",
		},
		{
			name:         "CodeMismatchException",
			err:          &types.CodeMismatchException{Message: aws.String("Code mismatch")},
			expectedCode: "CodeMismatch",
			expectedType: "*cognito.CognitoError",
		},
		{
			name:         "ExpiredCodeException",
			err:          &types.ExpiredCodeException{Message: aws.String("Code expired")},
			expectedCode: "ExpiredCode",
			expectedType: "*cognito.CognitoError",
		},
		{
			name:         "LimitExceededException",
			err:          &types.LimitExceededException{Message: aws.String("Limit exceeded")},
			expectedCode: "LimitExceeded",
			expectedType: "*cognito.CognitoError",
		},
		{
			name:         "TooManyRequestsException",
			err:          &types.TooManyRequestsException{Message: aws.String("Too many requests")},
			expectedCode: "TooManyRequests",
			expectedType: "*cognito.CognitoError",
		},
		{
			name:         "generic error",
			err:          errors.New("generic error"),
			expectedType: "*errors.errorString",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleCognitoError(tt.err)
			assert.NotNil(t, result)

			if tt.expectedCode != "" {
				cognitoErr, ok := result.(*CognitoError)
				assert.True(t, ok, "expected CognitoError")
				assert.Equal(t, tt.expectedCode, cognitoErr.Code)
			}
		})
	}
}

func TestIsCognitoError(t *testing.T) {
	cognitoErr := &CognitoError{
		Code:    "TestCode",
		Message: "Test message",
	}
	genericErr := errors.New("generic error")

	assert.True(t, IsCognitoError(cognitoErr))
	assert.False(t, IsCognitoError(genericErr))
}

func TestIsMFARequiredError(t *testing.T) {
	mfaErr := &MFARequiredError{
		SessionToken:  "session-token",
		ChallengeType: MFAChallengeTypeSMS,
	}
	genericErr := errors.New("generic error")

	assert.True(t, IsMFARequiredError(mfaErr))
	assert.False(t, IsMFARequiredError(genericErr))
}

func TestMFARequiredError_Error(t *testing.T) {
	err := &MFARequiredError{
		SessionToken:  "session-token",
		ChallengeType: MFAChallengeTypeSMS,
		Message:       "Custom message",
	}

	assert.Contains(t, err.Error(), "Custom message")

	err2 := &MFARequiredError{
		ChallengeType: MFAChallengeTypeSoftwareToken,
	}

	assert.Contains(t, err2.Error(), "SOFTWARE_TOKEN_MFA")
}
