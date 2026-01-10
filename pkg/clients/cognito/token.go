package cognito

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/golang-jwt/jwt/v5"
)

// ValidateToken valida un token JWT generado por Cognito usando JWKS
func (c *Client) ValidateToken(ctx context.Context, token string) (*TokenClaims, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}

	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v (expected RSA)", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("kid not found in token header")
		}

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

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	expectedIssuer := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", c.config.Region, c.config.UserPoolID)
	iss, ok := claims["iss"].(string)
	if !ok || iss != expectedIssuer {
		if !ok || !strings.HasPrefix(iss, "https://cognito-idp.") || !strings.Contains(iss, "/"+c.config.UserPoolID) {
			return nil, fmt.Errorf("%w: issuer mismatch (expected %s, got %s)", ErrInvalidToken, expectedIssuer, iss)
		}
	}

	var audMatch bool
	var audValue string

	if audStr, ok := claims["aud"].(string); ok {
		audMatch = audStr == c.config.ClientID
		audValue = audStr
	} else if audSlice, ok := claims["aud"].([]interface{}); ok {
		for _, v := range audSlice {
			if audStr, ok := v.(string); ok && audStr == c.config.ClientID {
				audMatch = true
				audValue = audStr
				break
			}
		}
	}

	if !audMatch {
		if clientID, ok := claims["client_id"].(string); ok {
			audMatch = clientID == c.config.ClientID
			audValue = clientID
		}
	}

	if !audMatch {
		return nil, fmt.Errorf("%w: audience mismatch (expected %s, got %s)", ErrInvalidToken, c.config.ClientID, audValue)
	}

	exp, ok := claims["exp"].(float64)
	if !ok || time.Now().Unix() > int64(exp) {
		return nil, ErrExpiredToken
	}

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

	if result == nil {
		return nil, fmt.Errorf("get user result is nil")
	}

	user := &User{
		ID:         "",
		Username:   "",
		Enabled:    true,
		Attributes: make(map[string]string),
		Status:     UserStatusConfirmed,
	}

	if result.Username != nil {
		user.ID = *result.Username
		user.Username = *result.Username
	}

	for _, attr := range result.UserAttributes {
		if attr.Name == nil || attr.Value == nil {
			continue
		}

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

	return user, nil
}

func (c *Client) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*AuthTokens, error) {
	if req.RefreshToken == "" {
		return nil, ErrMissingRequiredField
	}

	if c.clientSecret != "" && req.Username == "" {
		return nil, fmt.Errorf("%w: username required when client secret is configured", ErrMissingRequiredField)
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	authParams := map[string]string{
		"REFRESH_TOKEN": req.RefreshToken,
	}

	if c.clientSecret != "" && req.Username != "" {
		secretHash := c.computeSecretHash(req.Username)
		authParams["SECRET_HASH"] = secretHash
	}

	input := &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeRefreshTokenAuth,
		ClientId:       aws.String(c.config.ClientID),
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

	if result == nil || result.AuthenticationResult == nil {
		return nil, fmt.Errorf("authentication result is nil")
	}

	safeString := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	if result.AuthenticationResult.AccessToken == nil {
		return nil, fmt.Errorf("access token is nil")
	}
	if result.AuthenticationResult.IdToken == nil {
		return nil, fmt.Errorf("id token is nil")
	}

	tokens := &AuthTokens{
		AccessToken:  safeString(result.AuthenticationResult.AccessToken),
		RefreshToken: safeString(result.AuthenticationResult.RefreshToken),
		IDToken:      safeString(result.AuthenticationResult.IdToken),
		TokenType:    "Bearer",
		ExpiresIn:    int64(result.AuthenticationResult.ExpiresIn),
	}

	return tokens, nil
}
