package cognito

import (
	"context"
	"errors"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

// Config configuración del cliente Cognito
//
// SECURITY NOTE: client_secret debe cargarse desde variable de entorno
// usando sintaxis ${COGNITO_CLIENT_SECRET} en YAML. El secret se encapsula
// en el cliente y nunca se expone públicamente.
type Config struct {
	// AWS Configuration
	Region       string `mapstructure:"region" json:"region"`
	UserPoolID   string `mapstructure:"user_pool_id" json:"user_pool_id"`
	ClientID     string `mapstructure:"client_id" json:"client_id"`
	ClientSecret string `mapstructure:"client_secret" json:"-"` // Opcional, carga desde ${VAR_NAME}

	// JWT Configuration
	JWKSUrl         string        `mapstructure:"jwks_url" json:"jwks_url"` // Auto-generado si está vacío
	TokenExpiration time.Duration `mapstructure:"token_expiration" json:"token_expiration"`

	// Resilience
	Resilience resilience.Config `mapstructure:"resilience" json:"resilience"`

	// Timeouts
	Timeout      time.Duration `mapstructure:"timeout" json:"timeout"`
	MaxRetries   int           `mapstructure:"max_retries" json:"max_retries"`
	RetryBackoff time.Duration `mapstructure:"retry_backoff" json:"retry_backoff"`

	// Feature Flags
	EnableLogging  bool `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool `mapstructure:"with_resilience" json:"with_resilience"`
}

// User representa un usuario de Cognito
type User struct {
	ID            string            `json:"id"`
	Username      string            `json:"username"`
	Email         string            `json:"email"`
	EmailVerified bool              `json:"email_verified"`
	PhoneNumber   string            `json:"phone_number,omitempty"`
	PhoneVerified bool              `json:"phone_verified,omitempty"`
	Status        UserStatus        `json:"status"`
	Attributes    map[string]string `json:"attributes"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Enabled       bool              `json:"enabled"`
}

// UserStatus representa el estado de un usuario
type UserStatus string

const (
	UserStatusUnconfirmed         UserStatus = "UNCONFIRMED"
	UserStatusConfirmed           UserStatus = "CONFIRMED"
	UserStatusArchived            UserStatus = "ARCHIVED"
	UserStatusCompromised         UserStatus = "COMPROMISED"
	UserStatusUnknown             UserStatus = "UNKNOWN"
	UserStatusResetRequired       UserStatus = "RESET_REQUIRED"
	UserStatusForceChangePassword UserStatus = "FORCE_CHANGE_PASSWORD"
)

// AuthTokens representa los tokens de autenticación generados por Cognito
type AuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"` // "Bearer"
	ExpiresIn    int64  `json:"expires_in"` // Segundos
}

// TokenClaims representa los claims de un JWT token generado por Cognito
// Cognito genera y firma los tokens JWT automáticamente
// Este cliente solo valida y extrae los claims
type TokenClaims struct {
	Sub           string                 `json:"sub"` // User ID (Cognito User Sub)
	Email         string                 `json:"email"`
	EmailVerified bool                   `json:"email_verified"`
	Username      string                 `json:"cognito:username"`
	Groups        []string               `json:"cognito:groups"` // Grupos de Cognito
	CustomClaims  map[string]interface{} `json:"-"`              // Claims personalizados

	// Standard JWT Claims (generados por Cognito)
	Iss      string `json:"iss"`       // Issuer (Cognito User Pool URL)
	Aud      string `json:"aud"`       // Audience (Client ID)
	Exp      int64  `json:"exp"`       // Expiration
	Iat      int64  `json:"iat"`       // Issued At
	TokenUse string `json:"token_use"` // "id", "access", "refresh"
}

// MFAChallengeType representa el tipo de desafío MFA
type MFAChallengeType string

const (
	MFAChallengeTypeSMS           MFAChallengeType = "SMS_MFA"
	MFAChallengeTypeSoftwareToken MFAChallengeType = "SOFTWARE_TOKEN_MFA"
)

// RegisterUserRequest representa la solicitud de registro
type RegisterUserRequest struct {
	Username          string            `json:"username"`
	Email             string            `json:"email"`
	Password          string            `json:"password"`
	PhoneNumber       string            `json:"phone_number,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
	TemporaryPassword bool              `json:"temporary_password,omitempty"`
}

// AuthenticateRequest representa la solicitud de autenticación
type AuthenticateRequest struct {
	Username string `json:"username"` // Email o username
	Password string `json:"password"`
}

// MFAChallengeRequest representa la solicitud de desafío MFA
type MFAChallengeRequest struct {
	Username      string           `json:"username"`      // Username requerido por Cognito
	SessionToken  string           `json:"session_token"` // Token de sesión de Cognito
	MFACode       string           `json:"mfa_code"`      // Código recibido (SMS o TOTP)
	ChallengeType MFAChallengeType `json:"challenge_type"`
}

// ConfirmSignUpRequest representa la solicitud de confirmación de registro
type ConfirmSignUpRequest struct {
	Username         string `json:"username"`
	ConfirmationCode string `json:"confirmation_code"`
}

// ForgotPasswordRequest representa la solicitud de recuperación de contraseña
type ForgotPasswordRequest struct {
	Username string `json:"username"`
}

// ConfirmForgotPasswordRequest representa la confirmación de recuperación
type ConfirmForgotPasswordRequest struct {
	Username         string `json:"username"`
	ConfirmationCode string `json:"confirmation_code"`
	NewPassword      string `json:"new_password"`
}

// RefreshTokenRequest representa la solicitud de renovación de token
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// SoftwareTokenAssociation representa la asociación de un token TOTP
type SoftwareTokenAssociation struct {
	SecretCode string `json:"secret_code"` // Código secreto para configuración manual
	QRCode     string `json:"qr_code"`     // Base64 del código QR (PNG)
	Session    string `json:"session"`     // Token de sesión para verificación
}

// MFAStatus representa el estado de MFA de un usuario
type MFAStatus struct {
	MFAEnabled      bool     `json:"mfa_enabled"`      // Si MFA está habilitado
	MFATypes        []string `json:"mfa_types"`        // Tipos configurados: ["SMS_MFA", "SOFTWARE_TOKEN_MFA"]
	PreferredMethod string   `json:"preferred_method"` // Método preferido: "SMS_MFA" o "SOFTWARE_TOKEN_MFA"
	SMSEnabled      bool     `json:"sms_enabled"`      // Si SMS MFA está habilitado
	TOTPEnabled     bool     `json:"totp_enabled"`     // Si TOTP está habilitado y configurado
}

// Errores del dominio
var (
	// Errores de autenticación
	ErrInvalidCredentials      = errors.New("invalid credentials")
	ErrUserNotFound            = errors.New("user not found")
	ErrUserAlreadyExists       = errors.New("user already exists")
	ErrUserNotConfirmed        = errors.New("user is not confirmed")
	ErrInvalidToken            = errors.New("invalid token")
	ErrExpiredToken            = errors.New("token expired")
	ErrInvalidConfirmationCode = errors.New("invalid confirmation code")
	ErrCodeExpired             = errors.New("confirmation code expired")
	ErrPasswordTooShort        = errors.New("password too short")
	ErrPasswordTooWeak         = errors.New("password does not meet requirements")
	ErrPasswordSameAsPrevious  = errors.New("new password must be different from previous")

	// Errores de autorización
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidAccessToken = errors.New("invalid access token")

	// Errores de configuración
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrUserPoolNotFound = errors.New("user pool not found")
	ErrClientNotFound   = errors.New("client not found")

	// Errores de red/resiliencia
	ErrServiceUnavailable = errors.New("cognito service unavailable")
	ErrTimeout            = errors.New("request timeout")
	ErrTooManyRequests    = errors.New("too many requests")

	// Errores de validación
	ErrInvalidEmail         = errors.New("invalid email format")
	ErrInvalidPhoneNumber   = errors.New("invalid phone number format")
	ErrInvalidUsername      = errors.New("invalid username format")
	ErrMissingRequiredField = errors.New("missing required field")

	// Errores específicos de MFA
	ErrMFAAlreadyEnabled        = errors.New("MFA already enabled")
	ErrMFAConfigurationRequired = errors.New("TOTP configuration required")
	ErrInvalidMFACode           = errors.New("invalid MFA code")
	ErrMFACodeMismatch          = errors.New("MFA code mismatch")
	ErrMFACodeExpired           = errors.New("MFA code expired")
)

// CognitoError representa un error específico de Cognito
type CognitoError struct {
	Code        string
	Message     string
	StatusCode  int
	OriginalErr error
}

func (e *CognitoError) Error() string {
	if e.OriginalErr != nil {
		return e.Message + ": " + e.OriginalErr.Error()
	}
	return e.Message
}

func (e *CognitoError) Unwrap() error {
	return e.OriginalErr
}

// MFARequiredError representa un error cuando MFA es requerido
// Mejora propuesta: Tipo de error específico para MFA
type MFARequiredError struct {
	SessionToken  string
	ChallengeType MFAChallengeType
	Message       string
}

func (e *MFARequiredError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "MFA required: " + string(e.ChallengeType)
}

// IsCognitoError verifica si un error es de tipo CognitoError
func IsCognitoError(err error) bool {
	_, ok := err.(*CognitoError)
	return ok
}

// IsMFARequiredError verifica si un error es de tipo MFARequiredError
func IsMFARequiredError(err error) bool {
	_, ok := err.(*MFARequiredError)
	return ok
}

// Service define la interfaz del cliente Cognito
//
// PRINCIPIO YAGNI (You Aren't Gonna Need It):
// Esta interfaz incluye solo los métodos esenciales para MVP 0 y MVP 1
type Service interface {
	// MVP 0 - Funcionalidades Críticas
	RegisterUser(ctx context.Context, req RegisterUserRequest) (*User, error)
	ConfirmSignUp(ctx context.Context, req ConfirmSignUpRequest) error
	Authenticate(ctx context.Context, req AuthenticateRequest) (*AuthTokens, error)
	ValidateToken(ctx context.Context, token string) (*TokenClaims, error)
	GetUserByAccessToken(ctx context.Context, accessToken string) (*User, error)

	// MVP 0 - MFA Support
	RespondToMFAChallenge(ctx context.Context, req MFAChallengeRequest) (*AuthTokens, error)

	// MVP 1 - Funcionalidades Adicionales
	RefreshToken(ctx context.Context, req RefreshTokenRequest) (*AuthTokens, error)
	ForgotPassword(ctx context.Context, req ForgotPasswordRequest) error
	ConfirmForgotPassword(ctx context.Context, req ConfirmForgotPasswordRequest) error

	// MVP 0 - Gestión Completa de MFA
	AssociateSoftwareToken(ctx context.Context, accessToken string) (*SoftwareTokenAssociation, error)
	VerifySoftwareToken(ctx context.Context, accessToken, userCode, session string) error
	SetUserMFAPreference(ctx context.Context, accessToken string, smsEnabled, totpEnabled bool) error
	GetUserMFAStatus(ctx context.Context, accessToken string) (*MFAStatus, error)

	// MVP 0 - Gestión de Sesiones
	SignOut(ctx context.Context, accessToken string) error
	GlobalSignOut(ctx context.Context, accessToken string) error
}
