package cognito

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

func (c *Client) RegisterUser(ctx context.Context, req RegisterUserRequest) (*User, error) {
	if err := validateRegisterRequest(req); err != nil {
		return nil, err
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	attributes := []types.AttributeType{
		{Name: aws.String("email"), Value: aws.String(req.Email)},
	}

	if req.PhoneNumber != "" {
		attributes = append(attributes, types.AttributeType{
			Name: aws.String("phone_number"), Value: aws.String(req.PhoneNumber),
		})
	}

	for key, value := range req.Attributes {
		attributes = append(attributes, types.AttributeType{
			Name: aws.String(key), Value: aws.String(value),
		})
	}

	input := &cognitoidentityprovider.SignUpInput{
		ClientId:       aws.String(c.config.ClientID),
		Username:       aws.String(req.Username),
		Password:       aws.String(req.Password),
		UserAttributes: attributes,
	}

	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		input.SecretHash = aws.String(secretHash)
	}

	var result *cognitoidentityprovider.SignUpOutput
	_, err := c.executeOperation(ctx, "RegisterUser", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.SignUp(ctx, input)
		return result, err
	})

	if err != nil {
		return nil, handleCognitoError(err)
	}

	if result.UserSub == nil {
		return nil, fmt.Errorf("%w: user sub is nil", ErrInvalidConfig)
	}

	user := &User{
		ID:         *result.UserSub,
		Username:   req.Username,
		Email:      req.Email,
		Status:     UserStatusUnconfirmed,
		Attributes: req.Attributes,
		CreatedAt:  time.Now(),
		Enabled:    true,
	}

	if c.logging {
		c.logger.Info(ctx, "User registered successfully",
			map[string]interface{}{
				"user_id":  user.ID,
				"username": user.Username,
				"email":    user.Email,
			})
	}

	return user, nil
}

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

// Authenticate autentica un usuario y obtiene tokens JWT.
// Si MFA está activado, retorna MFARequiredError (usar RespondToMFAChallenge)
func (c *Client) Authenticate(ctx context.Context, req AuthenticateRequest) (*AuthTokens, error) {
	if err := validateAuthenticateRequest(req); err != nil {
		return nil, err
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	authParams := map[string]string{
		"USERNAME": req.Username,
		"PASSWORD": req.Password,
	}

	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		authParams["SECRET_HASH"] = secretHash
	}

	input := &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeUserPasswordAuth,
		ClientId:       aws.String(c.config.ClientID),
		AuthParameters: authParams,
	}

	var result *cognitoidentityprovider.InitiateAuthOutput
	_, err := c.executeOperation(ctx, "Authenticate", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.InitiateAuth(ctx, input)
		return result, err
	})

	if err != nil {
		cognitoErr := handleCognitoError(err)
		if cognitoErr, ok := cognitoErr.(*CognitoError); ok && cognitoErr.Code == "MFARequired" {
			return nil, &MFARequiredError{
				Message:       "MFA authentication required",
				ChallengeType: MFAChallengeTypeSMS,
			}
		}
		return nil, cognitoErr
	}

	if result.ChallengeName != "" {
		challengeType := MFAChallengeTypeSMS
		if string(result.ChallengeName) == string(MFAChallengeTypeSoftwareToken) {
			challengeType = MFAChallengeTypeSoftwareToken
		}

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
			})
	}

	return tokens, nil
}
