package mongodb

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

func NewClient(ctx context.Context, cfg Config, log logger.Service) (Service, error) {
	if cfg.URI == "" {
		return nil, fmt.Errorf("%w: URI is required", ErrConnection)
	}

	if !strings.HasPrefix(cfg.URI, "mongodb://") && !strings.HasPrefix(cfg.URI, "mongodb+srv://") {
		return nil, fmt.Errorf("%w: invalid MongoDB URI format, must start with mongodb:// or mongodb+srv://", ErrConnection)
	}

	if cfg.Database == "" {
		return nil, fmt.Errorf("%w: database name is required", ErrConnection)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	// Use provided context or create one with timeout
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	clientOptions := options.Client().ApplyURI(cfg.URI)
	
	if cfg.MaxPoolSize > 0 {
		clientOptions.SetMaxPoolSize(cfg.MaxPoolSize)
	}
	if cfg.MinPoolSize > 0 {
		clientOptions.SetMinPoolSize(cfg.MinPoolSize)
	}

	mongoClient, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	baseConfig := client.BaseConfig{
		EnableLogging:  cfg.EnableLogging,
		WithResilience: cfg.WithResilience,
		Resilience:     cfg.Resilience,
		Timeout:        timeout,
	}

	c := &MongoDBClient{
		BaseClient: client.NewBaseClientWithName(baseConfig, log, "MongoDB"),
		client:     mongoClient,
		database:   mongoClient.Database(cfg.Database),
		dbName:     cfg.Database,
	}

	if err := c.Ping(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	if c.IsLoggingEnabled() {
		// Redact credentials from URI before logging
		redactedURI := redactMongoURI(cfg.URI)
		log.Debug(ctx, "MongoDB connection established successfully",
			map[string]interface{}{
				"database": cfg.Database,
				"uri":      redactedURI,
			})
	}

	return c, nil
}

func (c *MongoDBClient) GetDatabase() *mongo.Database {
	return c.database
}

func (c *MongoDBClient) GetCollection(name string) *mongo.Collection {
	return c.database.Collection(name)
}

func (c *MongoDBClient) Ping(ctx context.Context) error {
	_, err := c.Execute(ctx, "Ping", func() (interface{}, error) {
		return nil, c.client.Ping(ctx, nil)
	})
	return err
}

func (c *MongoDBClient) Disconnect(ctx context.Context) error {
	_, err := c.Execute(ctx, "Disconnect", func() (interface{}, error) {
		return nil, c.client.Disconnect(ctx)
	})
	return err
}

// redactMongoURI removes userinfo (credentials) from MongoDB URI for safe logging
func redactMongoURI(uri string) string {
	// Parse URI and remove userinfo if present
	// Format: mongodb://[username:password@]host[:port][/database]
	if strings.Contains(uri, "@") {
		parts := strings.SplitN(uri, "@", 2)
		if len(parts) == 2 {
			// Extract scheme and remove userinfo
			schemeAndHost := parts[1]
			if strings.HasPrefix(uri, "mongodb://") {
				return "mongodb://***:***@" + schemeAndHost
			} else if strings.HasPrefix(uri, "mongodb+srv://") {
				return "mongodb+srv://***:***@" + schemeAndHost
			}
		}
	}
	return uri
}

func (c *MongoDBClient) EnableLogging(enable bool) {
	c.SetLogging(enable)
}

