package mongodb

import (
	"context"
	"errors"
	"time"

	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	DefaultTimeout = 30 * time.Second
)

var (
	ErrConnection   = errors.New("mongodb connection error")
	ErrNotFound     = errors.New("document not found")
	ErrInvalidInput = errors.New("invalid input")
)

type Config struct {
	URI            string            `mapstructure:"uri" json:"uri"`
	Database       string            `mapstructure:"database" json:"database"`
	Timeout        time.Duration     `mapstructure:"timeout" json:"timeout"`
	MaxPoolSize    uint64            `mapstructure:"max_pool_size" json:"max_pool_size"`
	MinPoolSize    uint64            `mapstructure:"min_pool_size" json:"min_pool_size"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
}

type Service interface {
	// GetDatabase returns the MongoDB database instance.
	GetDatabase() *mongo.Database

	// GetCollection returns a collection by name.
	GetCollection(name string) *mongo.Collection

	// Ping checks the connection to MongoDB.
	Ping(ctx context.Context) error

	// Disconnect closes the MongoDB connection.
	// Should be called when done using the client.
	Disconnect(ctx context.Context) error

	// EnableLogging enables or disables logging for this client.
	EnableLogging(enable bool)
}

type MongoDBClient struct {
	*client.BaseClient
	client   *mongo.Client
	database *mongo.Database
	dbName   string
}
