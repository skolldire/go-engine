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

func NewClient(cfg Config, log logger.Service) (Service, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
		log.Debug(ctx, "MongoDB connection established successfully",
			map[string]interface{}{
				"database": cfg.Database,
				"uri":      cfg.URI,
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

func (c *MongoDBClient) EnableLogging(enable bool) {
	c.SetLogging(enable)
}

