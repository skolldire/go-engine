package rest

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

func NewClient(cfg Config, log logger.Service) Service {
	timeout := cfg.TimeOut
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	httpClient := resty.New().
		SetTimeout(timeout).
		SetBaseURL(cfg.BaseURL)

	retryCfg, breakerCfg := cfg.Retry, cfg.CircuitBreaker

	// The breaker is the one gap resty leaves. Installing it as the transport
	// puts it inside resty's retry loop, so it sees each attempt.
	if breakerCfg != nil {
		httpClient.SetTransport(newBreakerTransport(httpClient.GetClient().Transport, breakerCfg, log))
	}

	// Resty owns the retry loop, its exponential backoff and its jitter. All we
	// contribute is the policy: what may be replayed, and honouring Retry-After.
	if retryCfg != nil {
		httpClient.
			SetRetryCount(retryCfg.maxRetries()).
			SetRetryWaitTime(retryCfg.waitTime()).
			SetRetryMaxWaitTime(retryCfg.maxWaitTime()).
			SetRetryAfter(RetryAfter)

		if retryCfg.RetryNonIdempotent {
			httpClient.AddRetryCondition(AllowNonIdempotentRetryCondition)
		} else {
			httpClient.AddRetryCondition(ConservativeRetryCondition)
		}
	}

	c := &restClient{
		BaseClient: client.NewBaseClientWithName(client.BaseConfig{
			EnableLogging: cfg.EnableLogging,
			Timeout:       timeout,
		}, log, "REST"),
		baseURL:    cfg.BaseURL,
		httpClient: httpClient,
	}

	return c
}

// R returns a request builder bound to this client.
func (c *restClient) R(ctx context.Context) *resty.Request {
	return c.httpClient.R().SetContext(ctx)
}

func (c *restClient) executeRequest(ctx context.Context, operationName string, reqFunc func() (*resty.Response, error)) (*resty.Response, error) {
	// Logging, metrics and tracing come from the BaseClient middleware chain,
	// so this client carries no `if c.logging` of its own.
	var resp *resty.Response

	_, err := c.Execute(ctx, operationName, func(context.Context) (any, error) {
		r, reqErr := reqFunc()
		resp = r
		if reqErr != nil {
			return nil, reqErr
		}
		// The response travels with the error. Folding the status into a
		// formatted string and discarding the response is what stopped callers
		// from branching on the status code, and stopped any retry policy from
		// telling a retryable 503 from a permanent 400.
		return r, validateResponse(r)
	})

	return resp, err
}

func (c *restClient) Get(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "GET "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetHeaders(headers).
			Get(c.baseURL + endpoint)
	})
}

func (c *restClient) Post(ctx context.Context, endpoint string, body any, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "POST "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Post(c.baseURL + endpoint)
	})
}

func (c *restClient) Put(ctx context.Context, endpoint string, body any, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "PUT "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Put(c.baseURL + endpoint)
	})
}

func (c *restClient) Patch(ctx context.Context, endpoint string, body any, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "PATCH "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Patch(c.baseURL + endpoint)
	})
}

func (c *restClient) Delete(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "DELETE "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetHeaders(headers).
			Delete(c.baseURL + endpoint)
	})
}

func (c *restClient) WithLogging(enable bool) {
	c.SetLogging(enable)
}

func validateResponse(resp *resty.Response) error {
	if resp == nil {
		return errors.New("respuesta es nil")
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 299 {
		return nil
	}
	bodyPreview := ""
	if resp.Body() != nil && len(resp.Body()) > 0 {
		text := string(resp.Body())
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		bodyPreview = text
	}
	return fmt.Errorf("HTTP %d: %s - %s", resp.StatusCode(), resp.Status(), bodyPreview)
}
