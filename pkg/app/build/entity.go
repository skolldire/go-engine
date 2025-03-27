package build

type App interface {
	Run() error
}

type Builder interface {
	LoadConfig() Builder
	InitRepositories() Builder
	InitUseCases() Builder
	InitHandlers() Builder
	InitRoutes() Builder
	Build() App
}

type BuilderWithMiddleware interface {
	LoadConfig() BuilderWithMiddleware
	InitMiddlewares() BuilderWithMiddleware
	InitRepositories() BuilderWithMiddleware
	InitUseCases() BuilderWithMiddleware
	InitHandlers() BuilderWithMiddleware
	InitRoutes() BuilderWithMiddleware
	Build() App
}

type BuilderWithGracefulShutdown interface {
	LoadConfig() BuilderWithGracefulShutdown
	InitGracefulShutdown() BuilderWithGracefulShutdown
	InitRepositories() BuilderWithGracefulShutdown
	InitUseCases() BuilderWithGracefulShutdown
	InitHandlers() BuilderWithGracefulShutdown
	InitRoutes() BuilderWithGracefulShutdown
	Build() App
}
