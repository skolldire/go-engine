package cognito

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
)

// AddUserToGroup agrega un usuario a un grupo del User Pool.
// Mapea AdminAddUserToGroup; el UserPoolID se toma de la Config del cliente.
// El grupo de Cognito modela el rol del usuario.
func (c *Client) AddUserToGroup(ctx context.Context, username, group string) error {
	if username == "" {
		return ErrMissingRequiredField
	}
	if group == "" {
		return ErrMissingRequiredField
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.AdminAddUserToGroupInput{
		UserPoolId: aws.String(c.config.UserPoolID),
		Username:   aws.String(username),
		GroupName:  aws.String(group),
	}

	_, err := c.executeOperation(ctx, "AddUserToGroup", func() (interface{}, error) {
		return c.cognitoClient.AdminAddUserToGroup(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	if c.logging {
		// No se registra el username: puede ser PII (email/teléfono).
		// group es el nombre del rol, no es PII.
		c.logger.Info(ctx, "User added to group successfully",
			map[string]interface{}{
				"group": group,
			})
	}

	return nil
}

// RemoveUserFromGroup quita un usuario de un grupo del User Pool.
// Mapea AdminRemoveUserFromGroup; el UserPoolID se toma de la Config del cliente.
func (c *Client) RemoveUserFromGroup(ctx context.Context, username, group string) error {
	if username == "" {
		return ErrMissingRequiredField
	}
	if group == "" {
		return ErrMissingRequiredField
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.AdminRemoveUserFromGroupInput{
		UserPoolId: aws.String(c.config.UserPoolID),
		Username:   aws.String(username),
		GroupName:  aws.String(group),
	}

	_, err := c.executeOperation(ctx, "RemoveUserFromGroup", func() (interface{}, error) {
		return c.cognitoClient.AdminRemoveUserFromGroup(ctx, input)
	})

	if err != nil {
		return handleCognitoError(err)
	}

	if c.logging {
		// No se registra el username: puede ser PII (email/teléfono).
		c.logger.Info(ctx, "User removed from group successfully",
			map[string]interface{}{
				"group": group,
			})
	}

	return nil
}

// ListGroupsForUser lista los nombres de los grupos a los que pertenece un usuario.
// Mapea AdminListGroupsForUser; el UserPoolID se toma de la Config del cliente.
// Maneja la paginación de Cognito de forma transparente.
func (c *Client) ListGroupsForUser(ctx context.Context, username string) ([]string, error) {
	if username == "" {
		return nil, ErrMissingRequiredField
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	var groups []string
	var nextToken *string

	for {
		input := &cognitoidentityprovider.AdminListGroupsForUserInput{
			UserPoolId: aws.String(c.config.UserPoolID),
			Username:   aws.String(username),
			NextToken:  nextToken,
		}

		result, err := c.executeOperation(ctx, "ListGroupsForUser", func() (interface{}, error) {
			return c.cognitoClient.AdminListGroupsForUser(ctx, input)
		})

		if err != nil {
			return nil, handleCognitoError(err)
		}

		output, ok := result.(*cognitoidentityprovider.AdminListGroupsForUserOutput)
		if !ok || output == nil {
			// No enmascarar un fallo real como éxito: la operación terminó sin
			// error pero la respuesta no tiene la forma esperada.
			return nil, fmt.Errorf("%w: AdminListGroupsForUser returned %T", ErrUnexpectedResponse, result)
		}

		for _, g := range output.Groups {
			if g.GroupName != nil {
				groups = append(groups, *g.GroupName)
			}
		}

		if output.NextToken == nil || *output.NextToken == "" {
			break
		}
		nextToken = output.NextToken
	}

	if c.logging {
		// No se registra el username: puede ser PII (email/teléfono).
		c.logger.Info(ctx, "Listed groups for user successfully",
			map[string]interface{}{
				"count": len(groups),
			})
	}

	return groups, nil
}
