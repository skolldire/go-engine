package build

import "context"

func Apply(builder Builder) App {
	return builder.
		LoadConfig().
		InitRepositories().
		InitUseCases().
		InitHandlers().
		InitRoutes().
		Build()
}

func ApplyWithContext(ctx context.Context, builder Builder) App {
	if contextBuilder, ok := builder.(interface{ SetContext(context.Context) Builder }); ok {
		builder = contextBuilder.SetContext(ctx)
	}

	return Apply(builder)
}

func ApplyWithMiddleware(builder BuilderWithMiddleware) App {
	return builder.
		LoadConfig().
		InitMiddlewares().
		InitRepositories().
		InitUseCases().
		InitHandlers().
		InitRoutes().
		Build()
}

func ApplyWithGracefulShutdown(builder BuilderWithGracefulShutdown) App {
	return builder.
		LoadConfig().
		InitGracefulShutdown().
		InitRepositories().
		InitUseCases().
		InitHandlers().
		InitRoutes().
		Build()
}
