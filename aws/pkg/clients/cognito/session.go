package cognito

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
)

// SignOut cierra la sesión del usuario invalidando el AccessToken actual
// Nota: AWS Cognito solo proporciona GlobalSignOut API, que invalida todos los tokens
// del usuario. Este método usa GlobalSignOut pero con semántica de cierre de sesión local
func (c *Client) SignOut(ctx context.Context, accessToken string) error {
	if accessToken == "" {
		return ErrInvalidAccessToken
	}

	_, err := c.ValidateToken(ctx, accessToken)
	if err != nil {
		return ErrInvalidToken
	}

	ctx, cancel := c.ContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.GlobalSignOutInput{
		AccessToken: aws.String(accessToken),
	}

	_, err = c.Execute(ctx, "SignOut", func(ctx context.Context) (any, error) {
		return c.cognitoClient.GlobalSignOut(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	return nil
}

// GlobalSignOut cierra todas las sesiones del usuario en todos los dispositivos
// Invalida todos los tokens de acceso y refresh del usuario en todos los dispositivos
func (c *Client) GlobalSignOut(ctx context.Context, accessToken string) error {
	if accessToken == "" {
		return ErrInvalidAccessToken
	}

	_, err := c.ValidateToken(ctx, accessToken)
	if err != nil {
		return ErrInvalidToken
	}

	ctx, cancel := c.ContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.GlobalSignOutInput{
		AccessToken: aws.String(accessToken),
	}

	_, err = c.Execute(ctx, "GlobalSignOut", func(ctx context.Context) (any, error) {
		return c.cognitoClient.GlobalSignOut(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	return nil
}
