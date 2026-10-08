package cognito

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
)

func (c *Client) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) error {
	if req.Username == "" {
		return ErrMissingRequiredField
	}

	ctx, cancel := c.ContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.ForgotPasswordInput{
		ClientId: aws.String(c.config.ClientID),
		Username: aws.String(req.Username),
	}

	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		input.SecretHash = aws.String(secretHash)
	}

	_, err := c.Execute(ctx, "ForgotPassword", func(ctx context.Context) (any, error) {
		return c.cognitoClient.ForgotPassword(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	return nil
}

func (c *Client) ConfirmForgotPassword(ctx context.Context, req ConfirmForgotPasswordRequest) error {
	if req.Username == "" || req.ConfirmationCode == "" || req.NewPassword == "" {
		return ErrMissingRequiredField
	}

	ctx, cancel := c.ContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.ConfirmForgotPasswordInput{
		ClientId:         aws.String(c.config.ClientID),
		Username:         aws.String(req.Username),
		ConfirmationCode: aws.String(req.ConfirmationCode),
		Password:         aws.String(req.NewPassword),
	}

	if c.clientSecret != "" {
		secretHash := c.computeSecretHash(req.Username)
		input.SecretHash = aws.String(secretHash)
	}

	_, err := c.Execute(ctx, "ConfirmForgotPassword", func(ctx context.Context) (any, error) {
		return c.cognitoClient.ConfirmForgotPassword(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	return nil
}
