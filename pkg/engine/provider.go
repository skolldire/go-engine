// Package engine is the dependency-inverted core of go-engine.
//
// The rule this package exists to enforce: the core knows no adapter, and every
// adapter knows the core. Nothing under pkg/engine may import a provider,
// an AWS SDK, a database driver or a broker client — `make lint-arch` fails the
// build if it does. That is what lets an application that only wants an HTTP
// router avoid paying for the whole ecosystem.
//
// A component is contributed by implementing Provider in its own package and
// handing it to New via WithProvider.
package engine

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// RawConfig is a configuration section the core has not decoded and whose type
// it does not know. Each Provider decodes its own section into its own type,
// which is what keeps concrete adapter types out of the core configuration
// struct.
type RawConfig interface {
	// Decode maps the section onto target, which must be a non-nil pointer.
	Decode(target any) error
	// Raw returns the undecoded value, for providers that need to inspect the
	// shape before decoding.
	Raw() any
	// Exists reports whether the section was present in the configuration file.
	Exists() bool
	// Key is the section name, so errors can name the offending YAML block.
	Key() string
}

// Provider builds and owns the lifecycle of one component.
//
// Implementations live in the adapter's own package (for example
// provider/sqs), never in the core. Adding a new adapter therefore requires no
// change to any file under pkg/engine.
type Provider interface {
	// Name identifies this instance, e.g. "sqs:orders". It must be unique
	// within an engine and is the key used to retrieve the component.
	Name() string

	// ConfigKey is the top-level configuration section this provider consumes,
	// e.g. "sqs_clients".
	ConfigKey() string

	// Init decodes the provider's own section and builds the component. The
	// returned value is stored in the engine and retrieved by Name.
	Init(ctx context.Context, raw RawConfig, deps Deps) (any, error)

	// Close releases the component. The core calls it in LIFO order during
	// shutdown. Returning nil is correct for components that own nothing.
	Close(ctx context.Context) error
}

// Deps is everything the core hands a provider. It is deliberately small: a
// growing Deps is the first sign that the core is learning about adapters.
type Deps struct {
	// Logger is the engine logger. Never nil.
	//
	// This is the full logger.Service, not a narrowed view. A narrow interface
	// bought no decoupling — the core already depends on the logger package for
	// Config and NewService — while forcing all sixteen providers to type-assert
	// their way back to it, each with an error path that could never fire.
	Logger logger.Service

	// Telemetry records spans and metrics. Never nil; it is a no-op when the
	// application configured no telemetry, so providers need no nil checks.
	Telemetry Telemetry

	// Health registers dependency checks. Never nil; registering against an
	// engine built without health support is silently ignored.
	Health HealthRegistrar

	// Section reads any top-level configuration section by name, for the cases
	// where a provider legitimately depends on shared settings it does not own
	// — several AWS adapters reading the same `aws:` block, for instance. The
	// core still knows nothing about what those sections contain.
	Section func(key string) RawConfig

	// Resource memoizes a shared resource for the lifetime of the engine,
	// keyed by an arbitrary string. Providers that need an expensive object in
	// common — several AWS providers sharing one resolved credential chain —
	// build it through Resource so it is created at most once, even when
	// several providers race. build is invoked at most once per key per engine.
	Resource func(key string, build func() (any, error)) (any, error)
}

// Telemetry is the core's view of observability. It is intentionally narrower
// than any OpenTelemetry type: the core must not depend on the OTel SDK, so the
// otel provider adapts its own implementation onto this interface.
type Telemetry interface {
	Span(ctx context.Context, name string, fn func(context.Context) error) error
	Counter(ctx context.Context, name string, value int64)
	Histogram(ctx context.Context, name string, value float64)
}

// HealthRegistrar accepts dependency health checks contributed by providers.
type HealthRegistrar interface {
	RegisterCheck(name string, check func(ctx context.Context) error)
}
