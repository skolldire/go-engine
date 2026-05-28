package cognito

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	// DefaultTimeout es el timeout por defecto para operaciones Cognito
	DefaultTimeout = 30 * time.Second
)

// Client implementa Service usando AWS SDK v2
type Client struct {
	config Config

	// CRÍTICO: Campo privado - único lugar donde se almacena el secret
	clientSecret string

	cognitoClient *cognitoidentityprovider.Client
	jwksClient    *JWKSClient
	logger        logger.Service
	resilience    *resilience.Service
	logging       bool
}

// NewClient crea una nueva instancia del cliente Cognito
// CRÍTICO: Manejo seguro del secret - se copia a campo privado y se limpia de Config
func NewClient(cfg Config, log logger.Service) (Service, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid cognito config: %w", err)
	}

	clientSecret := cfg.ClientSecret
	cfg.ClientSecret = ""

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
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

	var resilienceSvc *resilience.Service
	if cfg.WithResilience {
		resilienceSvc = resilience.NewResilienceService(cfg.Resilience, log)
	}

	client := &Client{
		config:        cfg,
		clientSecret:  clientSecret,
		cognitoClient: cognitoClient,
		jwksClient:    jwksClient,
		logger:        log,
		resilience:    resilienceSvc,
		logging:       cfg.EnableLogging,
	}

	if client.logging {
		logFields := map[string]interface{}{
			"user_pool_id": cfg.UserPoolID,
			"client_id":    cfg.ClientID,
			"region":       cfg.Region,
			"has_secret":   clientSecret != "",
		}
		if clientSecret != "" {
			log.Debug(context.Background(), "Cognito client initialized with client secret", logFields)
		} else {
			log.Debug(context.Background(), "Cognito client initialized without client secret", logFields)
		}
	}

	return client, nil
}

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

func (c *Client) executeOperation(ctx context.Context, operationName string,
	operation func() (interface{}, error)) (interface{}, error) {
	logFields := map[string]interface{}{
		"operation": operationName,
		"service":   "Cognito",
	}

	if c.resilience != nil {
		return c.executeWithResilience(ctx, operationName, operation, logFields)
	}

	return c.executeWithLogging(ctx, operationName, operation, logFields)
}

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

func (c *Client) computeSecretHash(username string) string {
	return computeSecretHash(c.config.ClientID, c.clientSecret, username)
}
