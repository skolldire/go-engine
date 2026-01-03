package app

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"go.elastic.co/ecslogrus"
)

type AppBuilder struct {
	engine *Engine
	errors []error
}

func NewAppBuilder() *AppBuilder {
	return &AppBuilder{
		engine: &Engine{
			errors: []error{},
			ctx:    context.Background(),
		},
		errors: []error{},
	}
}

func (b *AppBuilder) WithContext(ctx context.Context) *AppBuilder {
	if ctx == nil {
		b.addError(fmt.Errorf("context cannot be nil"))
		return b
	}
	b.engine.ctx = ctx
	return b
}

func (b *AppBuilder) WithConfigs() *AppBuilder {
	app := &App{Engine: b.engine}
	result := app.GetConfigs()
	b.engine = result.Engine
	b.errors = append(b.errors, b.engine.errors...)
	return b
}

func (b *AppBuilder) WithDynamicConfig() *AppBuilder {
	tracer := logrus.New()
	tracer.SetOutput(os.Stdout)
	tracer.SetFormatter(&ecslogrus.Formatter{})
	tracer.Level = logrus.DebugLevel

	v := viper.NewService(tracer)

	tempLog := logger.NewService(logger.Config{
		Level: "debug",
	}, tracer)

	dynamicConfig, err := v.ApplyDynamic(tempLog)
	if err != nil {
		b.addError(err)
		return b
	}

	config := dynamicConfig.Get().(*viper.Config)
	b.engine.Conf = config

	log := setLogLevel(config.Log, tracer)
	_ = log.SetLogLevel("trace")
	b.engine.Log = log

	ctx := b.engine.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := dynamicConfig.StartWatching(ctx); err != nil {
		b.engine.Log.Warn(ctx, "failed to start config watchers: "+err.Error(), nil)
	}

	return b
}

func (b *AppBuilder) WithInitialization() *AppBuilder {
	if len(b.errors) > 0 {
		return b
	}
	app := &App{Engine: b.engine}
	result := app.Init()
	b.engine = result.Engine
	b.errors = append(b.errors, b.engine.errors...)
	return b
}

func (b *AppBuilder) WithRouter() *AppBuilder {
	if len(b.errors) > 0 {
		return b
	}
	app := &App{Engine: b.engine}
	result := app.InitializeRouter()
	b.engine = result.Engine
	b.errors = append(b.errors, b.engine.errors...)
	return b
}

func (b *AppBuilder) WithMiddleware(middleware func(router.Service)) *AppBuilder {
	if b.engine.Router == nil {
		b.addError(fmt.Errorf("router not initialized, call WithRouter first"))
		return b
	}
	if middleware != nil {
		middleware(b.engine.Router)
	}
	return b
}

func (b *AppBuilder) WithGracefulShutdown() *AppBuilder {
	return b
}

func (b *AppBuilder) WithCustomClient(name string, client interface{}) *AppBuilder {
	if name == "" {
		b.addError(fmt.Errorf("client name cannot be empty"))
		return b
	}
	if client == nil {
		b.addError(fmt.Errorf("client cannot be nil"))
		return b
	}
	return b
}

func (b *AppBuilder) Build() (*Engine, error) {
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("build errors: %v", b.errors)
	}

	if b.engine.Router == nil {
		b.addError(fmt.Errorf("router not initialized"))
	}

	if len(b.errors) > 0 {
		return nil, fmt.Errorf("build errors: %v", b.errors)
	}

	return b.engine, nil
}

func (b *AppBuilder) GetErrors() []error {
	return b.errors
}

func (b *AppBuilder) addError(err error) {
	if err != nil {
		b.errors = append(b.errors, err)
	}
}

func (b *AppBuilder) SetLogger(log logger.Service) *AppBuilder {
	if log == nil {
		b.addError(fmt.Errorf("logger cannot be nil"))
		return b
	}
	b.engine.Log = log
	return b
}

