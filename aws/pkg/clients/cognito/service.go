package cognito

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	baseclient "github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	// DefaultTimeout es el timeout por defecto para operaciones Cognito
	DefaultTimeout = 30 * time.Second
)

// cognitoAPI abstrae las operaciones del SDK de Cognito usadas por el cliente.
// Lo satisface *cognitoidentityprovider.Client y permite inyectar mocks en tests.
type cognitoAPI interface {
	SignUp(context.Context, *cognitoidentityprovider.SignUpInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.SignUpOutput, error)
	ConfirmSignUp(context.Context, *cognitoidentityprovider.ConfirmSignUpInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ConfirmSignUpOutput, error)
	InitiateAuth(context.Context, *cognitoidentityprovider.InitiateAuthInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.InitiateAuthOutput, error)
	RespondToAuthChallenge(context.Context, *cognitoidentityprovider.RespondToAuthChallengeInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.RespondToAuthChallengeOutput, error)
	ForgotPassword(context.Context, *cognitoidentityprovider.ForgotPasswordInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ForgotPasswordOutput, error)
	ConfirmForgotPassword(context.Context, *cognitoidentityprovider.ConfirmForgotPasswordInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ConfirmForgotPasswordOutput, error)
	GetUser(context.Context, *cognitoidentityprovider.GetUserInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.GetUserOutput, error)
	GlobalSignOut(context.Context, *cognitoidentityprovider.GlobalSignOutInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.GlobalSignOutOutput, error)
	AssociateSoftwareToken(context.Context, *cognitoidentityprovider.AssociateSoftwareTokenInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AssociateSoftwareTokenOutput, error)
	VerifySoftwareToken(context.Context, *cognitoidentityprovider.VerifySoftwareTokenInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.VerifySoftwareTokenOutput, error)
	SetUserMFAPreference(context.Context, *cognitoidentityprovider.SetUserMFAPreferenceInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.SetUserMFAPreferenceOutput, error)
	AdminAddUserToGroup(context.Context, *cognitoidentityprovider.AdminAddUserToGroupInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminAddUserToGroupOutput, error)
	AdminRemoveUserFromGroup(context.Context, *cognitoidentityprovider.AdminRemoveUserFromGroupInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminRemoveUserFromGroupOutput, error)
	AdminListGroupsForUser(context.Context, *cognitoidentityprovider.AdminListGroupsForUserInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminListGroupsForUserOutput, error)
}

// Client implementa Service usando AWS SDK v2
type Client struct {
	config Config

	// CRÍTICO: Campo privado - único lugar donde se almacena el secret
	clientSecret string

	cognitoClient cognitoAPI
	jwksClient    *JWKSClient

	// Timeout handling, logging and resilience come from BaseClient's
	// middleware chain rather than from fields and conditionals here.
	*baseclient.BaseClient
}

// NewClient crea una nueva instancia del cliente Cognito
// CRÍTICO: Manejo seguro del secret - se copia a campo privado y se limpia de Config
func NewClient(ctx context.Context, cfg Config, log logger.Service) (Service, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid cognito config: %w", err)
	}

	clientSecret := cfg.ClientSecret
	cfg.ClientSecret = ""

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	cognitoClient := cognitoidentityprovider.NewFromConfig(awsCfg)

	jwksURL := cfg.JWKSUrl
	if jwksURL == "" {
		jwksURL = fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s/.well-known/jwks.json",
			cfg.Region, cfg.UserPoolID)
	}

	jwksClient := NewJWKSClient(jwksURL)

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &Client{
		config:        cfg,
		clientSecret:  clientSecret,
		cognitoClient: cognitoClient,
		jwksClient:    jwksClient,
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{
			EnableLogging:  cfg.EnableLogging,
			WithResilience: cfg.WithResilience,
			Resilience:     cfg.Resilience,
			Timeout:        timeout,
		}, log, "Cognito"),
	}, nil
}

func (c *Client) computeSecretHash(username string) string {
	return computeSecretHash(c.config.ClientID, c.clientSecret, username)
}
