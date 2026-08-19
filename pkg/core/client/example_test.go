package client_test

import (
	"context"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// --- Custom client definition ---

// IRTScorerConfig holds the settings for the IRT scorer service.
type IRTScorerConfig struct {
	BaseURL string
	Timeout time.Duration
}

// IRTScoreResponse is the result of a scoring request.
type IRTScoreResponse struct {
	Theta float64
	SE    float64
}

// IRTScorerClient calls the external IRT scoring service.
// It embeds BaseClient to get timeout management, logging, and optional
// retry + circuit-breaker for free.
type IRTScorerClient struct {
	client.BaseClient
	baseURL string
}

// NewIRTScorerClient constructs the client. Pass a BaseConfig to control
// logging, timeout, and resilience settings.
func NewIRTScorerClient(cfg IRTScorerConfig, log logger.Service) *IRTScorerClient {
	return &IRTScorerClient{
		BaseClient: *client.NewBaseClientWithName(
			client.BaseConfig{
				EnableLogging:  true,
				WithResilience: false,
				Timeout:        cfg.Timeout,
			},
			log,
			"irt-scorer",
		),
		baseURL: cfg.BaseURL,
	}
}

// Score sends an item response vector to the IRT endpoint and returns theta + SE.
// It delegates execution to BaseClient.Execute so that timeout and logging are
// applied transparently.
func (c *IRTScorerClient) Score(ctx context.Context, responses []int) (*IRTScoreResponse, error) {
	raw, err := c.Execute(ctx, "irt-scorer.score", func(ctx context.Context) (any, error) {
		// In production this would call c.baseURL with responses.
		// Stubbed here to keep the example self-contained.
		return &IRTScoreResponse{Theta: 0.42, SE: 0.15}, nil
	})
	if err != nil {
		return nil, err
	}
	return client.SafeTypeAssert[*IRTScoreResponse](raw)
}

// --- Example function ---

// Example_customClient shows how to build a client on top of BaseClient and
// hand it to the engine.
//
// There is no generic "custom client" slot. A client joins an engine by
// implementing engine.Provider, the same interface every adapter in this
// repository uses, and is retrieved with engine.Get[T]. That is what keeps the
// core free of adapter imports: the engine never needs to know the concrete
// type, only that something claimed the configuration section.
func Example_customClient() {
	// 1. Build the custom client.
	log := &noopLogger{}
	scorer := NewIRTScorerClient(IRTScorerConfig{
		BaseURL: "https://irt.internal",
		Timeout: 5 * time.Second,
	}, log)

	// 2. Use it directly, or wrap it in an engine.Provider so the engine builds
	//    it from configuration and closes it in reverse order on shutdown:
	//
	//    type provider struct{ c *IRTScorerClient }
	//
	//    func (p *provider) Name() string      { return "irt-scorer" }
	//    func (p *provider) ConfigKey() string { return "irt_scorer" }
	//
	//    func (p *provider) Init(ctx context.Context, raw engine.RawConfig,
	//        deps engine.Deps) (any, error) {
	//
	//        var cfg IRTScorerConfig
	//        if err := raw.Decode(&cfg); err != nil {
	//            return nil, err
	//        }
	//        p.c = NewIRTScorerClient(cfg, deps.Logger)
	//        return p.c, nil
	//    }
	//
	//    func (p *provider) Close(ctx context.Context) error { return nil }
	//
	//    eng, _ := engine.New(ctx, engine.WithProvider(&provider{}))
	//
	//    Later, in a handler or use case:
	//
	//    scorer, err := engine.Get[*IRTScorerClient](eng, "irt-scorer")

	// 3. Call it.
	result, err := scorer.Score(context.Background(), []int{1, 0, 1, 1})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("theta=%.2f se=%.2f\n", result.Theta, result.SE)

	// Output:
	// theta=0.42 se=0.15
}

// noopLogger satisfies logger.Service with silent no-ops for the example.
type noopLogger struct{}

func (n *noopLogger) Debug(ctx context.Context, msg string, fields map[string]any)     {}
func (n *noopLogger) Info(ctx context.Context, msg string, fields map[string]any)      {}
func (n *noopLogger) Warn(ctx context.Context, msg string, fields map[string]any)      {}
func (n *noopLogger) Error(ctx context.Context, err error, fields map[string]any)      {}
func (n *noopLogger) FatalError(ctx context.Context, err error, fields map[string]any) {}
func (n *noopLogger) WrapError(err error, msg string) error                            { return err }
func (n *noopLogger) WithField(key string, value any) logger.Service                   { return n }
func (n *noopLogger) WithFields(fields map[string]any) logger.Service                  { return n }
func (n *noopLogger) GetLogLevel() string                                              { return "info" }
func (n *noopLogger) SetLogLevel(level string) error                                   { return nil }
