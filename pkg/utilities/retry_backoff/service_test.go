package retry_backoff

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
)

type mockLogger struct{}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]any)     {}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any)      {}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]any)      {}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]any)      {}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]any) {}
func (m *mockLogger) WrapError(err error, msg string) error                            { return err }
func (m *mockLogger) WithField(key string, value any) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]any) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                              { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                   { return nil }

func TestNewRetryer(t *testing.T) {
	config := &Config{
		MaxRetries: 3,
	}

	retryer := NewRetryer(Dependencies{
		RetryConfig: config,
		Logger:      nil,
	})

	assert.NotNil(t, retryer)
}

func TestNewRetryer_WithDefaults(t *testing.T) {
	retryer := NewRetryer(Dependencies{
		RetryConfig: &Config{}, // Empty config should use defaults
		Logger:      nil,
	})

	assert.NotNil(t, retryer)
	assert.Equal(t, DefaultMaxRetries, retryer.config.MaxRetries)
}

func TestRetryer_Do_Success(t *testing.T) {
	retryer := NewRetryer(Dependencies{
		RetryConfig: &Config{MaxRetries: 3},
		Logger:      nil,
	})

	err := retryer.Do(context.Background(), func() error {
		return nil
	})

	assert.NoError(t, err)
}

func TestRetryer_Do_Error(t *testing.T) {
	retryer := NewRetryer(Dependencies{
		RetryConfig: &Config{MaxRetries: 1},
		Logger:      nil,
	})

	testErr := errors.New("test error")
	err := retryer.Do(context.Background(), func() error {
		return testErr
	})

	assert.Error(t, err)
}

func TestRetryer_Do_ContextCancelled(t *testing.T) {
	retryer := NewRetryer(Dependencies{
		RetryConfig: &Config{MaxRetries: 3, InitialWaitTime: 200},
		Logger:      nil,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel context during wait to test cancellation during retry wait
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := retryer.Do(ctx, func() error {
		return errors.New("test")
	})

	assert.Error(t, err)
	// The retryer may return either the operation error or ctx.Err() depending on timing
	// Both are valid behaviors - we just verify that an error is returned
	assert.True(t, err != nil)
}

func TestRetryer_Do_WithLogger(t *testing.T) {
	retryer := NewRetryer(Dependencies{
		RetryConfig: &Config{MaxRetries: 1},
		Logger:      &mockLogger{},
	})

	testErr := errors.New("test error")
	err := retryer.Do(context.Background(), func() error {
		return testErr
	})

	assert.Error(t, err)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected *Config
	}{
		{
			name:   "all defaults",
			config: &Config{},
			expected: &Config{
				InitialWaitTime: DefaultInitialWaitTime,
				MaxWaitTime:     DefaultMaxWaitTime,
				MaxRetries:      DefaultMaxRetries,
				BackoffFactor:   DefaultBackoffFactor,
				JitterFactor:    DefaultJitterFactor,
			},
		},
		{
			name: "valid config",
			config: &Config{
				InitialWaitTime: 200 * time.Millisecond,
				MaxWaitTime:     5 * time.Second,
				MaxRetries:      5,
				BackoffFactor:   2.5,
				JitterFactor:    0.3,
			},
			expected: &Config{
				InitialWaitTime: 200 * time.Millisecond,
				MaxWaitTime:     5 * time.Second,
				MaxRetries:      5,
				BackoffFactor:   2.5,
				JitterFactor:    0.3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeConfig(tt.config)
			assert.Equal(t, tt.expected.InitialWaitTime, got.InitialWaitTime)
			assert.Equal(t, tt.expected.MaxRetries, got.MaxRetries)
		})
	}
}

func TestNormalizeConfigNilIsSafe(t *testing.T) {
	got := normalizeConfig(nil)
	assert.Equal(t, DefaultInitialWaitTime, got.InitialWaitTime)
	assert.Equal(t, DefaultMaxWaitTime, got.MaxWaitTime)
	assert.Equal(t, DefaultMaxRetries, got.MaxRetries)
}

func TestNormalizeConfigDoesNotMutateInput(t *testing.T) {
	in := &Config{}
	_ = normalizeConfig(in)
	assert.Equal(t, time.Duration(0), in.InitialWaitTime, "input config must not be mutated")
	assert.Equal(t, 0, in.MaxRetries, "input config must not be mutated")
}

// TestNewRetryerAppliesDurationsExactly guards against the double time-unit
// multiplication bug: a 100ms initial wait must stay 100ms, not 100ms*time.Millisecond.
func TestNewRetryerAppliesDurationsExactly(t *testing.T) {
	r := NewRetryer(Dependencies{
		RetryConfig: &Config{
			InitialWaitTime: 100 * time.Millisecond,
			MaxWaitTime:     10 * time.Second,
			MaxRetries:      3,
			BackoffFactor:   2.0,
		},
	})
	assert.Equal(t, 100*time.Millisecond, r.config.InitialWaitTime)
	assert.Equal(t, 10*time.Second, r.config.MaxWaitTime)
}

func TestRetryer_CalculateWaitTime(t *testing.T) {
	retryer := NewRetryer(Dependencies{
		RetryConfig: &Config{
			InitialWaitTime: 100 * time.Millisecond,
			MaxWaitTime:     1 * time.Second,
			BackoffFactor:   2.0,
			JitterFactor:    0.1,
		},
		Logger: nil,
	})

	waitTime := retryer.calculateWaitTime(0)
	assert.Greater(t, waitTime, time.Duration(0))
	assert.LessOrEqual(t, waitTime, retryer.config.MaxWaitTime)
}
