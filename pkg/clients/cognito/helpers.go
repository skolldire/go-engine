package cognito

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

// validateConfig valida la configuración del cliente Cognito
func validateConfig(cfg Config) error {
	if cfg.Region == "" {
		return fmt.Errorf("%w: region is required", ErrInvalidConfig)
	}
	if cfg.UserPoolID == "" {
		return fmt.Errorf("%w: user_pool_id is required", ErrInvalidConfig)
	}
	if cfg.ClientID == "" {
		return fmt.Errorf("%w: client_id is required", ErrInvalidConfig)
	}

	// Validar que si ClientSecret está configurado en YAML pero la variable no existe
	// (es decir, todavía tiene el formato ${VAR_NAME} sin resolver)
	if strings.HasPrefix(cfg.ClientSecret, "${") && strings.HasSuffix(cfg.ClientSecret, "}") {
		// La variable de entorno no se resolvió
		return fmt.Errorf("%w: client_secret environment variable not set or not resolved", ErrInvalidConfig)
	}

	return nil
}

// computeSecretHash calcula el SecretHash necesario para operaciones con ClientSecret
// CRÍTICO: Esta función debe ser privada y solo usarse internamente
// No expone el secret ni el hash calculado
// Fórmula: HMAC_SHA256(clientSecret, username + clientID) -> Base64
func computeSecretHash(clientID, clientSecret, username string) string {
	if clientSecret == "" {
		return ""
	}

	message := username + clientID
	mac := hmac.New(sha256.New, []byte(clientSecret))
	mac.Write([]byte(message))
	hash := mac.Sum(nil)

	return base64.StdEncoding.EncodeToString(hash)
}

// validateRegisterRequest valida el request de registro
func validateRegisterRequest(req RegisterUserRequest) error {
	if req.Username == "" {
		return fmt.Errorf("%w: username", ErrMissingRequiredField)
	}
	if req.Email == "" {
		return fmt.Errorf("%w: email", ErrMissingRequiredField)
	}
	if req.Password == "" {
		return fmt.Errorf("%w: password", ErrMissingRequiredField)
	}

	// Validar formato de email básico
	if !strings.Contains(req.Email, "@") {
		return ErrInvalidEmail
	}

	return nil
}

// validateAuthenticateRequest valida el request de autenticación
func validateAuthenticateRequest(req AuthenticateRequest) error {
	if req.Username == "" {
		return fmt.Errorf("%w: username", ErrMissingRequiredField)
	}
	if req.Password == "" {
		return fmt.Errorf("%w: password", ErrMissingRequiredField)
	}
	return nil
}

// validateMFAChallengeRequest valida el request de desafío MFA
func validateMFAChallengeRequest(req MFAChallengeRequest) error {
	if req.Username == "" {
		return fmt.Errorf("%w: username", ErrMissingRequiredField)
	}
	if req.SessionToken == "" {
		return fmt.Errorf("%w: session_token", ErrMissingRequiredField)
	}
	if req.MFACode == "" {
		return fmt.Errorf("%w: mfa_code", ErrMissingRequiredField)
	}
	if req.ChallengeType != MFAChallengeTypeSMS && req.ChallengeType != MFAChallengeTypeSoftwareToken {
		return fmt.Errorf("invalid challenge type: %s", req.ChallengeType)
	}
	return nil
}

// getStringClaim extrae un claim string de los claims del token
func getStringClaim(claims map[string]interface{}, key string) string {
	if val, ok := claims[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getBoolClaim extrae un claim bool de los claims del token
func getBoolClaim(claims map[string]interface{}, key string) bool {
	if val, ok := claims[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
		// Cognito a veces retorna "true"/"false" como strings
		if str, ok := val.(string); ok {
			return str == "true"
		}
	}
	return false
}

// getStringSliceClaim extrae un claim []string de los claims del token
func getStringSliceClaim(claims map[string]interface{}, key string) []string {
	if val, ok := claims[key]; ok {
		if slice, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}

// getFloat64Claim extrae un claim float64 de los claims del token
func getFloat64Claim(claims map[string]interface{}, key string) float64 {
	if val, ok := claims[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
	}
	return 0
}

// handleCognitoError maneja errores de Cognito y los convierte a errores tipados
// Retorna CognitoError o MFARequiredError según el caso
// Usa errors.As para type assertions robustas en lugar de string matching
func handleCognitoError(err error) error {
	if err == nil {
		return nil
	}

	// Usar errors.As para type assertions robustas
	var ae *types.NotAuthorizedException
	if errors.As(err, &ae) {
		// Verificar si es un error de MFA requerido
		if ae.Message != nil && (strings.Contains(*ae.Message, "MFA") || strings.Contains(*ae.Message, "challenge")) {
			// El SessionToken generalmente viene en el resultado de InitiateAuth, no en el error
			// Se manejará en el método Authenticate cuando se detecte el ChallengeName
			return &CognitoError{
				Code:        "MFARequired",
				Message:     "MFA authentication required",
				StatusCode:  401,
				OriginalErr: err,
			}
		}
		return &CognitoError{
			Code:        "NotAuthorized",
			Message:     "authentication failed",
			StatusCode:  401,
			OriginalErr: err,
		}
	}

	var ip *types.InvalidParameterException
	if errors.As(err, &ip) {
		return &CognitoError{
			Code:        "InvalidParameter",
			Message:     "invalid parameter provided",
			StatusCode:  400,
			OriginalErr: err,
		}
	}

	var rnf *types.ResourceNotFoundException
	if errors.As(err, &rnf) {
		return &CognitoError{
			Code:        "ResourceNotFound",
			Message:     "resource not found",
			StatusCode:  404,
			OriginalErr: err,
		}
	}

	var uee *types.UsernameExistsException
	if errors.As(err, &uee) {
		return &CognitoError{
			Code:        "UsernameExists",
			Message:     "username already exists",
			StatusCode:  400,
			OriginalErr: ErrUserAlreadyExists,
		}
	}

	var unf *types.UserNotFoundException
	if errors.As(err, &unf) {
		return &CognitoError{
			Code:        "UserNotFound",
			Message:     "user not found",
			StatusCode:  404,
			OriginalErr: ErrUserNotFound,
		}
	}

	var cm *types.CodeMismatchException
	if errors.As(err, &cm) {
		return &CognitoError{
			Code:        "CodeMismatch",
			Message:     "confirmation code does not match",
			StatusCode:  400,
			OriginalErr: ErrInvalidConfirmationCode,
		}
	}

	var ec *types.ExpiredCodeException
	if errors.As(err, &ec) {
		return &CognitoError{
			Code:        "ExpiredCode",
			Message:     "confirmation code has expired",
			StatusCode:  400,
			OriginalErr: ErrCodeExpired,
		}
	}

	var le *types.LimitExceededException
	if errors.As(err, &le) {
		return &CognitoError{
			Code:        "LimitExceeded",
			Message:     "rate limit exceeded",
			StatusCode:  429,
			OriginalErr: err,
		}
	}

	var tmr *types.TooManyRequestsException
	if errors.As(err, &tmr) {
		return &CognitoError{
			Code:        "TooManyRequests",
			Message:     "too many requests",
			StatusCode:  429,
			OriginalErr: ErrTooManyRequests,
		}
	}

	// Retornar error original si no se puede mapear
	return err
}

// maskEmail enmascara el email para logging, ocultando la parte local antes del @
// Ejemplo: "user@example.com" -> "****@example.com"
func maskEmail(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "****"
	}
	return "****@" + parts[1]
}
