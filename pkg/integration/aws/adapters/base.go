package adapters

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// baseAdapter routes requests to appropriate service adapter
type baseAdapter struct {
	config   aws.Config
	timeout  time.Duration
	retries  RetryPolicy
	adapters map[string]cloud.Client
}

// RetryPolicy is copied from aws package to avoid circular dependency
type RetryPolicy struct {
	Enabled         bool
	MaxAttempts     int
	RetriableErrors []string
}

// NewBaseAdapter creates a cloud.Client that routes requests to service-specific adapters.
// The returned adapter is configured with the provided AWS config, default timeout, and retry policy and delegates to adapters for: sqs, sns, lambda, s3, ses, and ssm.
func NewBaseAdapter(cfg aws.Config, timeout time.Duration, retries RetryPolicy) cloud.Client {
	adapter := &baseAdapter{
		config:   cfg,
		timeout:  timeout,
		retries:  retries,
		adapters: make(map[string]cloud.Client),
	}

	// Initialize service adapters
	adapter.adapters["sqs"] = newSQSAdapter(cfg, timeout, retries)
	adapter.adapters["sns"] = newSNSAdapter(cfg, timeout, retries)
	adapter.adapters["lambda"] = newLambdaAdapter(cfg, timeout, retries)
	adapter.adapters["s3"] = newS3Adapter(cfg, timeout, retries)
	adapter.adapters["ses"] = newSESAdapter(cfg, timeout, retries)
	adapter.adapters["ssm"] = newSSMAdapter(cfg, timeout, retries)

	return adapter
}

func (b *baseAdapter) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req == nil {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "request cannot be nil")
	}

	if req.Operation == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "operation is required")
	}

	// Extract service from operation (e.g., "sqs.send" -> "sqs")
	parts := strings.Split(req.Operation, ".")
	if len(parts) < 2 {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("invalid operation format: %s (expected 'service.operation')", req.Operation))
	}

	service := parts[0]
	adapter, ok := b.adapters[service]
	if !ok {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("unsupported service: %s", service))
	}

	// Apply per-request timeout if specified
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	} else if b.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.timeout)
		defer cancel()
	}

	return adapter.Do(ctx, req)
}
