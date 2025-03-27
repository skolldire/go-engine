package app

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/app/build"
	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/clients/sns"
	"github.com/skolldire/go-engine/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/skolldire/go-engine/pkg/database/dynamo"
	"github.com/skolldire/go-engine/pkg/database/gormsql"
	"github.com/skolldire/go-engine/pkg/database/redis"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"go.elastic.co/ecslogrus"
)

const IncorrectType = "Builder returned incorrect type"

type ClientInitializer struct {
	ctx       context.Context
	log       logger.Service
	awsConfig aws.Config
	errors    []error
}

func NewApp() *Engine {
	ctx := context.Background()
	builder := NewEngineBuilder()
	app := build.ApplyWithContext(ctx, builder)

	engine, ok := app.(*Engine)
	if !ok {
		panic(IncorrectType)
	}

	return engine
}

// BaseBuilder contiene la lógica compartida entre diferentes builders
type BaseBuilder struct {
	engine *Engine
	ctx    context.Context
	config *viper.Config
}

// EngineBuilder implementa build.Builder
type EngineBuilder struct {
	base BaseBuilder
}

// EngineBuilderWithMiddleware implementa build.BuilderWithMiddleware
type EngineBuilderWithMiddleware struct {
	base BaseBuilder
}

// EngineBuilderWithGracefulShutdown implementa build.BuilderWithGracefulShutdown
type EngineBuilderWithGracefulShutdown struct {
	base BaseBuilder
}

// NewEngineBuilder crea un nuevo builder estándar
func NewEngineBuilder() *EngineBuilder {
	return &EngineBuilder{
		base: BaseBuilder{
			engine: &Engine{
				errors: []error{},
				ctx:    context.Background(),
			},
		},
	}
}

// Funciones factory para crear builders específicos
func NewMiddlewareBuilder() *EngineBuilderWithMiddleware {
	return &EngineBuilderWithMiddleware{
		base: BaseBuilder{
			engine: &Engine{
				errors: []error{},
				ctx:    context.Background(),
			},
		},
	}
}

func NewGracefulShutdownBuilder() *EngineBuilderWithGracefulShutdown {
	return &EngineBuilderWithGracefulShutdown{
		base: BaseBuilder{
			engine: &Engine{
				errors: []error{},
				ctx:    context.Background(),
			},
		},
	}
}

// Implementaciones de EngineBuilder (build.Builder)

func (b *EngineBuilder) SetContext(ctx context.Context) build.Builder {
	b.base.ctx = ctx
	b.base.engine.ctx = ctx
	return b
}

func (b *EngineBuilder) LoadConfig() build.Builder {
	b.loadConfigImpl()
	return b
}

func (b *EngineBuilder) InitRepositories() build.Builder {
	b.initRepositoriesImpl()
	return b
}

func (b *EngineBuilder) InitUseCases() build.Builder {
	b.initUseCasesImpl()
	return b
}

func (b *EngineBuilder) InitHandlers() build.Builder {
	b.initHandlersImpl()
	return b
}

func (b *EngineBuilder) InitRoutes() build.Builder {
	b.initRoutesImpl()
	return b
}

func (b *EngineBuilder) Build() build.App {
	return b.base.engine
}

// Implementaciones de EngineBuilderWithMiddleware (build.BuilderWithMiddleware)

func (b *EngineBuilderWithMiddleware) LoadConfig() build.BuilderWithMiddleware {
	b.loadConfigImpl()
	return b
}

func (b *EngineBuilderWithMiddleware) InitMiddlewares() build.BuilderWithMiddleware {
	// Implementar middlewares aquí
	return b
}

func (b *EngineBuilderWithMiddleware) InitRepositories() build.BuilderWithMiddleware {
	b.initRepositoriesImpl()
	return b
}

func (b *EngineBuilderWithMiddleware) InitUseCases() build.BuilderWithMiddleware {
	b.initUseCasesImpl()
	return b
}

func (b *EngineBuilderWithMiddleware) InitHandlers() build.BuilderWithMiddleware {
	b.initHandlersImpl()
	return b
}

func (b *EngineBuilderWithMiddleware) InitRoutes() build.BuilderWithMiddleware {
	b.initRoutesImpl()
	return b
}

func (b *EngineBuilderWithMiddleware) Build() build.App {
	return b.base.engine
}

// Implementaciones de EngineBuilderWithGracefulShutdown (build.BuilderWithGracefulShutdown)

func (b *EngineBuilderWithGracefulShutdown) LoadConfig() build.BuilderWithGracefulShutdown {
	b.loadConfigImpl()
	return b
}

func (b *EngineBuilderWithGracefulShutdown) InitGracefulShutdown() build.BuilderWithGracefulShutdown {
	// Implementar cierre controlado aquí
	return b
}

func (b *EngineBuilderWithGracefulShutdown) InitRepositories() build.BuilderWithGracefulShutdown {
	b.initRepositoriesImpl()
	return b
}

func (b *EngineBuilderWithGracefulShutdown) InitUseCases() build.BuilderWithGracefulShutdown {
	b.initUseCasesImpl()
	return b
}

func (b *EngineBuilderWithGracefulShutdown) InitHandlers() build.BuilderWithGracefulShutdown {
	b.initHandlersImpl()
	return b
}

func (b *EngineBuilderWithGracefulShutdown) InitRoutes() build.BuilderWithGracefulShutdown {
	b.initRoutesImpl()
	return b
}

func (b *EngineBuilderWithGracefulShutdown) Build() build.App {
	return b.base.engine
}

// Métodos de implementación compartidos para todos los builders

func (b *BaseBuilder) loadConfigImpl() {
	tracer := logrus.New()
	tracer.SetOutput(os.Stdout)
	tracer.SetFormatter(&ecslogrus.Formatter{})
	tracer.Level = logrus.DebugLevel

	v := viper.NewService(tracer)
	c, err := v.Apply()
	if err != nil {
		tracer.Error(err)
		b.engine.errors = append(b.engine.errors, err)
		return
	}

	b.config = &c

	log := configLogLevel(c.Log, tracer)
	_ = log.SetLogLevel("trace")

	b.engine.Log = log
}

func (b *BaseBuilder) initRepositoriesImpl() {
	if b.config == nil || len(b.engine.errors) > 0 {
		return
	}

	awsConfig, err := config.LoadDefaultConfig(b.ctx,
		config.WithRegion(b.config.Aws.Region),
	)
	if err != nil {
		b.engine.errors = append(b.engine.errors, err)
		return
	}

	initializer := &ClientInitializer{
		ctx:       b.ctx,
		log:       b.engine.Log,
		awsConfig: awsConfig,
	}

	b.engine.RestClients = initializer.createHttpClients(b.config.Rest)
	b.engine.SQSClient = initializer.createSQSClient(b.config.SQS)
	b.engine.SNSClient = initializer.createSNSClient(b.config.SNS)
	b.engine.DynamoDBClient = initializer.createDynamoClient(b.config.Dynamo)
	b.engine.RedisClient = initializer.createRedisClient(b.config.Redis)
	b.engine.SqlConnection = initializer.createSQLClient(b.config.DataBaseSql)

	b.engine.RepositoriesConfig = b.config.Repositories

	if len(initializer.errors) > 0 {
		b.engine.errors = append(b.engine.errors, initializer.errors...)
	}
}

func (b *BaseBuilder) initUseCasesImpl() {
	if b.config == nil || len(b.engine.errors) > 0 {
		return
	}

	b.engine.UsesCasesConfig = b.config.Cases
}

func (b *BaseBuilder) initHandlersImpl() {
	if b.config == nil || len(b.engine.errors) > 0 {
		return
	}

	b.engine.HandlerConfig = b.config.Endpoints
	b.engine.BatchConfig = b.config.Processors
}

func (b *BaseBuilder) initRoutesImpl() {
	if b.config == nil || len(b.engine.errors) > 0 {
		return
	}

	b.engine.App = router.NewService(b.config.Router, router.WithLogger(b.engine.Log))

	_ = b.engine.Log.SetLogLevel(b.config.Log.Level)

}

func (b *EngineBuilderWithMiddleware) loadConfigImpl() {
	b.base.loadConfigImpl()
}

func (b *EngineBuilderWithMiddleware) initRepositoriesImpl() {
	b.base.initRepositoriesImpl()
}

func (b *EngineBuilderWithMiddleware) initUseCasesImpl() {
	b.base.initUseCasesImpl()
}

func (b *EngineBuilderWithMiddleware) initHandlersImpl() {
	b.base.initHandlersImpl()
}

func (b *EngineBuilderWithMiddleware) initRoutesImpl() {
	b.base.initRoutesImpl()
}

func (b *EngineBuilder) loadConfigImpl() {
	b.base.loadConfigImpl()
}

func (b *EngineBuilder) initRepositoriesImpl() {
	b.base.initRepositoriesImpl()
}

func (b *EngineBuilder) initUseCasesImpl() {
	b.base.initUseCasesImpl()
}

func (b *EngineBuilder) initHandlersImpl() {
	b.base.initHandlersImpl()
}

func (b *EngineBuilder) initRoutesImpl() {
	b.base.initRoutesImpl()
}

func (b *EngineBuilderWithGracefulShutdown) loadConfigImpl() {
	b.base.loadConfigImpl()
}

func (b *EngineBuilderWithGracefulShutdown) initRepositoriesImpl() {
	b.base.initRepositoriesImpl()
}

func (b *EngineBuilderWithGracefulShutdown) initUseCasesImpl() {
	b.base.initUseCasesImpl()
}

func (b *EngineBuilderWithGracefulShutdown) initHandlersImpl() {
	b.base.initHandlersImpl()
}

func (b *EngineBuilderWithGracefulShutdown) initRoutesImpl() {
	b.base.initRoutesImpl()
}

// Funciones de ayuda para crear aplicaciones con diferentes builders

func NewAppWithMiddleware() *Engine {
	builder := NewMiddlewareBuilder()
	app := build.ApplyWithMiddleware(builder)

	engine, ok := app.(*Engine)
	if !ok {
		panic(IncorrectType)
	}

	return engine
}

func NewAppWithGracefulShutdown() *Engine {
	builder := NewGracefulShutdownBuilder()
	app := build.ApplyWithGracefulShutdown(builder)

	engine, ok := app.(*Engine)
	if !ok {
		panic(IncorrectType)
	}

	return engine
}

func configLogLevel(c logger.Config, l *logrus.Logger) logger.Service {
	return logger.NewService(logger.Config{
		Level: c.Level,
		Path:  c.Path,
	}, l)
}

// Métodos para ClientInitializer
func (i *ClientInitializer) recordError(err error) {
	i.errors = append(i.errors, err)
}

func (i *ClientInitializer) createHttpClients(configs []map[string]rest.Config) map[string]rest.Service {
	httpClients := make(map[string]rest.Service)
	for _, v := range configs {
		for k, cfg := range v {
			httpClients[k] = rest.NewClient(cfg, i.log)
		}
	}
	return httpClients
}

func (i *ClientInitializer) createSQSClient(cfg *sqs.Config) sqs.Service {
	if cfg == nil {
		return nil
	}
	return sqs.NewClient(i.awsConfig, *cfg, i.log)
}

func (i *ClientInitializer) createSNSClient(cfg *sns.Config) sns.Service {
	if cfg == nil {
		return nil
	}
	return sns.NewClient(i.awsConfig, *cfg, i.log)
}

func (i *ClientInitializer) createDynamoClient(cfg *dynamo.Config) dynamo.Service {
	if cfg == nil {
		return nil
	}
	return dynamo.NewClient(i.awsConfig, *cfg, i.log)
}

func (i *ClientInitializer) createRedisClient(cfg *redis.Config) *redis.RedisClient {
	if cfg == nil {
		return nil
	}

	client, err := redis.NewClient(*cfg, i.log)
	if err != nil {
		i.recordError(err)
		return nil
	}

	return client
}

func (i *ClientInitializer) createSQLClient(cfg *gormsql.Config) *gormsql.DBClient {
	if cfg == nil {
		return nil
	}

	client, err := gormsql.NewClient(*cfg, i.log)
	if err != nil {
		i.recordError(err)
		return nil
	}

	return client
}

// Verificación de implementación de interfaces
var (
	_ build.Builder                     = (*EngineBuilder)(nil)
	_ build.BuilderWithMiddleware       = (*EngineBuilderWithMiddleware)(nil)
	_ build.BuilderWithGracefulShutdown = (*EngineBuilderWithGracefulShutdown)(nil)
)
