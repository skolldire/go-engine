package cognito

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/golang-jwt/jwt/v5"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	// DefaultTimeout es el timeout por defecto para operaciones Cognito
	DefaultTimeout = 30 * time.Second
)

// Client implementa Service usando AWS SDK v2
// El secret se almacena en un campo privado (no exportado)
type Client struct {
	// Config completo (pero ClientSecret se limpia después de inicialización)
	config Config

	// CRÍTICO: Campo privado (minúscula) - no exportado
	// Este es el único lugar donde se almacena el secret
	clientSecret string

	// Cliente AWS SDK
	cognitoClient *cognitoidentityprovider.Client

	// Cliente JWKS para validación de tokens
	jwksClient *JWKSClient

	// Logger y resiliencia
	logger     logger.Service
	resilience *resilience.Service
	logging    bool
}

// NewClient crea una nueva instancia del cliente Cognito
// CRÍTICO: Manejo seguro del secret
func NewClient(cfg Config, log logger.Service) (Service, error) {
	// Validar configuración
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid cognito config: %w", err)
	}

	// CRÍTICO: Copiar secret a campo privado ANTES de crear el cliente
	clientSecret := cfg.ClientSecret

	// CRÍTICO: Limpiar secret de Config para evitar exposición accidental
	cfg.ClientSecret = ""

	// Crear cliente AWS SDK
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	cognitoClient := cognitoidentityprovider.NewFromConfig(awsCfg)

	// Configurar JWKS URL si no se proporciona
	jwksURL := cfg.JWKSUrl
	if jwksURL == "" {
		jwksURL = fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s/.well-known/jwks.json",
			cfg.Region, cfg.UserPoolID)
	}

	// Crear cliente JWKS
	jwksClient := NewJWKSClient(jwksURL)

	// Crear servicio de resiliencia si está habilitado
	var resilienceSvc *resilience.Service
	if cfg.WithResilience {
		resilienceSvc = resilience.NewResilienceService(cfg.Resilience, log)
	}

	// Crear cliente con secret privado
	client := &Client{
		config:        cfg,           // Config sin secret (ya limpiado)
		clientSecret:  clientSecret,  // Secret en campo privado
		cognitoClient: cognitoClient,
		jwksClient:    jwksClient,
		logger:        log,
		resilience:    resilienceSvc,
		logging:       cfg.EnableLogging,
	}

	// Logging seguro: solo indicar si secret está presente, NO el valor
	if client.logging {
		logFields := map[string]interface{}{
			"user_pool_id": cfg.UserPoolID,
			"client_id":    cfg.ClientID,
			"region":       cfg.Region,
			"has_secret":   clientSecret != "", // Solo indicador, no el valor
		}
		if clientSecret != "" {
			log.Debug(context.Background(), "Cognito client initialized with client secret", logFields)
		} else {
			log.Debug(context.Background(), "Cognito client initialized without client secret", logFields)
		}
	}

	return client, nil
}

// ensureContextWithTimeout asegura que el contexto tenga un timeout
func (c *Client) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := c.config.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}

// executeOperation ejecuta una operación con logging y resiliencia
func (c *Client) executeOperation(ctx context.Context, operationName string,
	operation func() (interface{}, error)) (interface{}, error) {
	logFields := map[string]interface{}{
		"operation": operationName,
		"service":   "Cognito",
	}

	// Ejecutar con resiliencia si está habilitado
	if c.resilience != nil {
		return c.executeWithResilience(ctx, operationName, operation, logFields)
	}

	return c.executeWithLogging(ctx, operationName, operation, logFields)
}

// executeWithResilience ejecuta una operación con resiliencia
func (c *Client) executeWithResilience(ctx context.Context, operationName string,
	operation func() (interface{}, error), logFields map[string]interface{}) (interface{}, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("starting Cognito operation with resilience: %s", operationName), logFields)
	}

	result, err := c.resilience.Execute(ctx, operation)

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Cognito operation completed with resilience: %s", operationName), logFields)
	}

	return result, err
}

// executeWithLogging ejecuta una operación con logging
func (c *Client) executeWithLogging(ctx context.Context, operationName string,
	operation func() (interface{}, error), logFields map[string]interface{}) (interface{}, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("starting Cognito operation: %s", operationName), logFields)
	}

	result, err := operation()

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Cognito operation completed: %s", operationName), logFields)
	}

	return result, err
}

// computeSecretHash calcula el SecretHash necesario para operaciones con ClientSecret
// CRÍTICO: Método privado - solo usado internamente
func (c *Client) computeSecretHash(username string) string {
	return computeSecretHash(c.config.ClientID, c.clientSecret, username)
}

// RegisterUser registra un nuevo usuario en Cognito
func (c *Client) RegisterUser(ctx context.Context, req RegisterUserRequest) (*User, error) {
	// Validar request
	if err := validateRegisterRequest(req); err != nil {
		return nil, err
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	// Preparar atributos
	attributes := []types.AttributeType{
		{Name: aws.String("email"), Value: aws.String(req.Email)},
	}

	if req.PhoneNumber != "" {
		attributes = append(attributes, types.AttributeType{
			Name: aws.String("phone_number"), Value: aws.String(req.PhoneNumber),
		})
	}

	// Agregar atributos personalizados
	for key, value := range req.Attributes {
		attributes = append(attributes, types.AttributeType{
			Name: aws.String(key), Value: aws.String(value),
		})
	}

	// Preparar input
	input := &cognitoidentityprovider.SignUpInput{
		ClientId:      aws.String(c.config.ClientID),
		Username:      aws.String(req.Username),
		Password:      aws.String(req.Password),
		UserAttributes: attributes,
	}

	// Agregar SecretHash si ClientSecret está configurado
	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		input.SecretHash = aws.String(secretHash)
	}

	// Ejecutar con resiliencia
	var result *cognitoidentityprovider.SignUpOutput
	_, err := c.executeOperation(ctx, "RegisterUser", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.SignUp(ctx, input)
		return result, err
	})

	if err != nil {
		return nil, handleCognitoError(err)
	}

	// Convertir a User
	user := &User{
		ID:          *result.UserSub,
		Username:    req.Username,
		Email:       req.Email,
		Status:      UserStatusUnconfirmed,
		Attributes:  req.Attributes,
		CreatedAt:   time.Now(),
		Enabled:     true,
	}

	if c.logging {
		c.logger.Info(ctx, "User registered successfully",
			map[string]interface{}{
				"user_id":  user.ID,
				"username": user.Username,
				"email":    user.Email,
				// NO loggear: password, secret, hash
			})
	}

	return user, nil
}

// ConfirmSignUp confirma el registro de usuario con código de verificación
func (c *Client) ConfirmSignUp(ctx context.Context, req ConfirmSignUpRequest) error {
	if req.Username == "" || req.ConfirmationCode == "" {
		return ErrMissingRequiredField
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.ConfirmSignUpInput{
		ClientId:         aws.String(c.config.ClientID),
		Username:         aws.String(req.Username),
		ConfirmationCode: aws.String(req.ConfirmationCode),
	}

	// Agregar SecretHash si ClientSecret está configurado
	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		input.SecretHash = aws.String(secretHash)
	}

	_, err := c.executeOperation(ctx, "ConfirmSignUp", func() (interface{}, error) {
		return c.cognitoClient.ConfirmSignUp(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	if c.logging {
		c.logger.Info(ctx, "User signup confirmed successfully",
			map[string]interface{}{
				"username": req.Username,
			})
	}

	return nil
}

// Authenticate autentica un usuario y obtiene tokens JWT
// Si MFA está activado, retorna MFARequiredError (usar RespondToMFAChallenge)
func (c *Client) Authenticate(ctx context.Context, req AuthenticateRequest) (*AuthTokens, error) {
	// Validar request
	if err := validateAuthenticateRequest(req); err != nil {
		return nil, err
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	// Preparar parámetros de autenticación
	authParams := map[string]string{
		"USERNAME": req.Username,
		"PASSWORD": req.Password,
	}

	// Agregar SecretHash si ClientSecret está configurado
	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		authParams["SECRET_HASH"] = secretHash
	}

	input := &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow:      types.AuthFlowTypeUserPasswordAuth,
		ClientId:      aws.String(c.config.ClientID),
		AuthParameters: authParams,
	}

	var result *cognitoidentityprovider.InitiateAuthOutput
	_, err := c.executeOperation(ctx, "Authenticate", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.InitiateAuth(ctx, input)
		return result, err
	})

	if err != nil {
		// Verificar si es error de MFA requerido
		cognitoErr := handleCognitoError(err)
		if cognitoErr, ok := cognitoErr.(*CognitoError); ok && cognitoErr.Code == "MFARequired" {
			// Extraer SessionToken del resultado si está disponible
			// Nota: AWS SDK puede retornar el ChallengeParameters en el error
			// Por ahora retornamos un error genérico que será manejado por el cliente
			return nil, &MFARequiredError{
				Message:       "MFA authentication required",
				ChallengeType: MFAChallengeTypeSMS, // Por defecto, se puede detectar del error
			}
		}
		return nil, cognitoErr
	}

	// Verificar si hay un challenge (MFA requerido)
	if result.ChallengeName != "" {
		challengeType := MFAChallengeTypeSMS
		if string(result.ChallengeName) == string(MFAChallengeTypeSoftwareToken) {
			challengeType = MFAChallengeTypeSoftwareToken
		}

		// Extraer SessionToken
		sessionToken := ""
		if result.Session != nil {
			sessionToken = *result.Session
		}

		return nil, &MFARequiredError{
			SessionToken:  sessionToken,
			ChallengeType: challengeType,
			Message:       fmt.Sprintf("MFA required: %s", challengeType),
		}
	}

	// Extraer tokens
	if result.AuthenticationResult == nil {
		return nil, fmt.Errorf("authentication result is nil")
	}

	tokens := &AuthTokens{
		AccessToken:  *result.AuthenticationResult.AccessToken,
		RefreshToken: *result.AuthenticationResult.RefreshToken,
		IDToken:      *result.AuthenticationResult.IdToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(result.AuthenticationResult.ExpiresIn),
	}

	if c.logging {
		c.logger.Info(ctx, "User authenticated successfully",
			map[string]interface{}{
				"username": req.Username,
				// NO loggear: password, secret, tokens completos
			})
	}

	return tokens, nil
}

// ValidateToken valida un token JWT generado por Cognito
// Valida firma usando JWKS de Cognito y extrae claims
// Mejora propuesta: Validación estricta de audience
func (c *Client) ValidateToken(ctx context.Context, token string) (*TokenClaims, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}

	// Parsear token y validar firma con JWKS de Cognito
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// Cognito siempre usa RSA para firmar tokens
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v (expected RSA)", token.Header["alg"])
		}

		// Obtener kid (Key ID) del header del token
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("kid not found in token header")
		}

		// Obtener clave pública desde JWKS endpoint de Cognito
		key, err := c.jwksClient.GetKey(ctx, kid)
		if err != nil {
			return nil, fmt.Errorf("failed to get public key from Cognito JWKS: %w", err)
		}

		return key, nil
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !parsedToken.Valid {
		return nil, ErrInvalidToken
	}

	// Extraer claims del token
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Validar issuer (debe ser el User Pool de Cognito)
	iss, ok := claims["iss"].(string)
	if !ok || !strings.Contains(iss, c.config.UserPoolID) {
		return nil, fmt.Errorf("%w: issuer mismatch", ErrInvalidToken)
	}

	// Mejora propuesta: Validación estricta de audience
	aud, ok := claims["aud"].(string)
	if !ok || aud != c.config.ClientID {
		return nil, fmt.Errorf("%w: audience mismatch (expected %s, got %s)", ErrInvalidToken, c.config.ClientID, aud)
	}

	// Validar expiración (Cognito maneja la expiración automáticamente)
	exp, ok := claims["exp"].(float64)
	if !ok || time.Now().Unix() > int64(exp) {
		return nil, ErrExpiredToken
	}

	// Convertir claims a TokenClaims
	tokenClaims := &TokenClaims{
		Sub:           getStringClaim(claims, "sub"),
		Email:         getStringClaim(claims, "email"),
		EmailVerified: getBoolClaim(claims, "email_verified"),
		Username:      getStringClaim(claims, "cognito:username"),
		Groups:        getStringSliceClaim(claims, "cognito:groups"),
		Iss:           getStringClaim(claims, "iss"),
		Aud:           getStringClaim(claims, "aud"),
		Exp:           int64(exp),
		Iat:           int64(getFloat64Claim(claims, "iat")),
		TokenUse:      getStringClaim(claims, "token_use"),
		CustomClaims:  make(map[string]interface{}),
	}

	return tokenClaims, nil
}

// GetUserByAccessToken obtiene información del usuario desde access token
func (c *Client) GetUserByAccessToken(ctx context.Context, accessToken string) (*User, error) {
	if accessToken == "" {
		return nil, ErrInvalidAccessToken
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.GetUserInput{
		AccessToken: aws.String(accessToken),
	}

	var result *cognitoidentityprovider.GetUserOutput
	_, err := c.executeOperation(ctx, "GetUserByAccessToken", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.GetUser(ctx, input)
		return result, err
	})

	if err != nil {
		return nil, handleCognitoError(err)
	}

	// Convertir respuesta AWS a User
	user := &User{
		ID:         *result.Username,
		Username:   *result.Username,
		Enabled:    true, // Por defecto, se puede obtener de otros atributos si es necesario
		Attributes: make(map[string]string),
		Status:     UserStatusConfirmed, // Por defecto
	}

	// Extraer atributos
	for _, attr := range result.UserAttributes {
		if attr.Name != nil && attr.Value != nil {
			switch *attr.Name {
			case "sub":
				user.ID = *attr.Value
			case "email":
				user.Email = *attr.Value
			case "email_verified":
				user.EmailVerified = *attr.Value == "true"
			case "phone_number":
				user.PhoneNumber = *attr.Value
			case "phone_number_verified":
				user.PhoneVerified = *attr.Value == "true"
			default:
				user.Attributes[*attr.Name] = *attr.Value
			}
		}
	}

	return user, nil
}

// RespondToMFAChallenge completa desafío MFA y obtiene tokens JWT
func (c *Client) RespondToMFAChallenge(ctx context.Context, req MFAChallengeRequest) (*AuthTokens, error) {
	// Validar request
	if err := validateMFAChallengeRequest(req); err != nil {
		return nil, err
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	// Preparar parámetros
	challengeParams := map[string]string{
		"SOFTWARE_TOKEN_MFA_CODE": req.MFACode,
	}

	if req.ChallengeType == MFAChallengeTypeSMS {
		challengeParams = map[string]string{
			"SMS_MFA_CODE": req.MFACode,
		}
	}

	input := &cognitoidentityprovider.RespondToAuthChallengeInput{
		ClientId:          aws.String(c.config.ClientID),
		ChallengeName:    types.ChallengeNameType(string(req.ChallengeType)),
		Session:           aws.String(req.SessionToken),
		ChallengeResponses: challengeParams,
	}

	var result *cognitoidentityprovider.RespondToAuthChallengeOutput
	_, err := c.executeOperation(ctx, "RespondToMFAChallenge", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.RespondToAuthChallenge(ctx, input)
		return result, err
	})

	if err != nil {
		return nil, handleCognitoError(err)
	}

	// Extraer tokens
	if result.AuthenticationResult == nil {
		return nil, fmt.Errorf("authentication result is nil")
	}

	tokens := &AuthTokens{
		AccessToken:  *result.AuthenticationResult.AccessToken,
		RefreshToken: *result.AuthenticationResult.RefreshToken,
		IDToken:      *result.AuthenticationResult.IdToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(result.AuthenticationResult.ExpiresIn),
	}

	if c.logging {
		c.logger.Info(ctx, "MFA challenge completed successfully",
			map[string]interface{}{
				"challenge_type": req.ChallengeType,
				// NO loggear: mfa_code, tokens completos
			})
	}

	return tokens, nil
}

// RefreshToken renueva tokens expirados usando refresh token
func (c *Client) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*AuthTokens, error) {
	if req.RefreshToken == "" {
		return nil, ErrMissingRequiredField
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	// Preparar parámetros
	authParams := map[string]string{
		"REFRESH_TOKEN": req.RefreshToken,
	}

	input := &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow:      types.AuthFlowTypeRefreshTokenAuth,
		ClientId:      aws.String(c.config.ClientID),
		AuthParameters: authParams,
	}

	var result *cognitoidentityprovider.InitiateAuthOutput
	_, err := c.executeOperation(ctx, "RefreshToken", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.InitiateAuth(ctx, input)
		return result, err
	})

	if err != nil {
		return nil, handleCognitoError(err)
	}

	// Extraer tokens
	if result.AuthenticationResult == nil {
		return nil, fmt.Errorf("authentication result is nil")
	}

	tokens := &AuthTokens{
		AccessToken:  *result.AuthenticationResult.AccessToken,
		RefreshToken: *result.AuthenticationResult.RefreshToken,
		IDToken:      *result.AuthenticationResult.IdToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(result.AuthenticationResult.ExpiresIn),
	}

	return tokens, nil
}

// ForgotPassword inicia proceso de recuperación de contraseña
func (c *Client) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) error {
	if req.Username == "" {
		return ErrMissingRequiredField
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.ForgotPasswordInput{
		ClientId: aws.String(c.config.ClientID),
		Username: aws.String(req.Username),
	}

	// Agregar SecretHash si ClientSecret está configurado
	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		input.SecretHash = aws.String(secretHash)
	}

	_, err := c.executeOperation(ctx, "ForgotPassword", func() (interface{}, error) {
		return c.cognitoClient.ForgotPassword(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	if c.logging {
		c.logger.Info(ctx, "Password recovery initiated",
			map[string]interface{}{
				"username": req.Username,
			})
	}

	return nil
}

// ConfirmForgotPassword confirma recuperación de contraseña
func (c *Client) ConfirmForgotPassword(ctx context.Context, req ConfirmForgotPasswordRequest) error {
	if req.Username == "" || req.ConfirmationCode == "" || req.NewPassword == "" {
		return ErrMissingRequiredField
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.ConfirmForgotPasswordInput{
		ClientId:         aws.String(c.config.ClientID),
		Username:         aws.String(req.Username),
		ConfirmationCode: aws.String(req.ConfirmationCode),
		Password:         aws.String(req.NewPassword),
	}

	// Agregar SecretHash si ClientSecret está configurado
	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		input.SecretHash = aws.String(secretHash)
	}

	_, err := c.executeOperation(ctx, "ConfirmForgotPassword", func() (interface{}, error) {
		return c.cognitoClient.ConfirmForgotPassword(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	if c.logging {
		c.logger.Info(ctx, "Password recovery confirmed",
			map[string]interface{}{
				"username": req.Username,
			})
	}

	return nil
}
