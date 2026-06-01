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
	raw, err := c.Execute(ctx, "irt-scorer.score", func() (interface{}, error) {
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

// Example_customClient shows how to build a custom client using BaseClient,
// inject it into the engine via WithCustomClient, and retrieve it with
// GetCustomClient + SafeTypeAssert.
//
// In a real service the engine would be built with app.NewAppBuilder();
// here we demonstrate the pattern with the client in isolation.
func Example_customClient() {
	// 1. Build the custom client.
	log := &noopLogger{}
	scorer := NewIRTScorerClient(IRTScorerConfig{
		BaseURL: "https://irt.internal",
		Timeout: 5 * time.Second,
	}, log)

	// 2. Use it directly (or inject into the AppBuilder via WithCustomClient).
	//
	//    engine, _ := app.NewAppBuilder().
	//        WithDynamicConfig().
	//        WithCustomClient("irt-scorer", scorer).
	//        WithRouter().
	//        Build()
	//
	//    Later, in a handler or use-case:
	//
	//    raw    := engine.GetCustomClient("irt-scorer")
	//    scorer, err := client.SafeTypeAssert[*IRTScorerClient](raw)

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

func (n *noopLogger) Debug(ctx context.Context, msg string, fields map[string]interface{})     {}
func (n *noopLogger) Info(ctx context.Context, msg string, fields map[string]interface{})      {}
func (n *noopLogger) Warn(ctx context.Context, msg string, fields map[string]interface{})      {}
func (n *noopLogger) Error(ctx context.Context, err error, fields map[string]interface{})      {}
func (n *noopLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (n *noopLogger) WrapError(err error, msg string) error                                    { return err }
func (n *noopLogger) WithField(key string, value interface{}) logger.Service                   { return n }
func (n *noopLogger) WithFields(fields map[string]interface{}) logger.Service                  { return n }
func (n *noopLogger) GetLogLevel() string                                                      { return "info" }
func (n *noopLogger) SetLogLevel(level string) error                                           { return nil }
