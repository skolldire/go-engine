package rest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

func NewClient(cfg Config, logger logger.Service) Service {
	httpClient := resty.New()
	if cfg.TimeOut > 0 {
		httpClient.SetTimeout(cfg.TimeOut * time.Second)
	}

	c := &client{
		baseURL:    cfg.BaseURL,
		httpClient: httpClient,
		logger:     logger,
		logging:    cfg.EnableLogging,
	}

	if cfg.RetryConfig != nil {
		c.retryer = retry_backoff.NewRetryer(retry_backoff.Dependencies{
			RetryConfig: cfg.RetryConfig,
			Logger:      logger,
		})
	}

	if cfg.CircuitBreakerCfg != nil {
		c.circuitBreaker = circuit_breaker.NewCircuitBreaker(circuit_breaker.Dependencies{
			Config: cfg.CircuitBreakerCfg,
			Log:    logger,
		})
	}

	return c
}

func (c *client) executeRequest(ctx context.Context, reqFunc func(ctx context.Context) (*resty.Response, error)) (*resty.Response, error) {
	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	if c.circuitBreaker != nil {
		return c.executeWithCircuitBreaker(ctx, reqFunc)
	}

	return c.executeWithRetry(ctx, reqFunc)
}

func (c *client) executeWithCircuitBreaker(ctx context.Context, reqFunc func(ctx context.Context) (*resty.Response, error)) (*resty.Response, error) {
	result, err := c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return c.executeWithRetry(ctx, reqFunc)
	})

	if err != nil {
		if errors.Is(err, circuit_breaker.ErrCircuitOpen) {
			c.logCircuitBreakerOpen(ctx, err)
		}
		return nil, err
	}

	return result.(*resty.Response), nil
}

func (c *client) executeWithRetry(ctx context.Context, reqFunc func(ctx context.Context) (*resty.Response, error)) (*resty.Response, error) {
	if c.retryer == nil {
		return c.executeHTTP(ctx, reqFunc)
	}

	var response *resty.Response
	err := c.retryer.Do(ctx, func() error {
		resp, err := c.executeHTTP(ctx, reqFunc)
		if err == nil {
			response = resp
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (c *client) executeHTTP(ctx context.Context, reqFunc func(ctx context.Context) (*resty.Response, error)) (*resty.Response, error) {
	resp, err := reqFunc(ctx)
	if err != nil {
		c.logRequestFailure(ctx, err)
		return nil, err
	}

	if err := validateResponse(resp); err != nil {
		c.logHttpError(ctx, resp, err)
		return nil, err
	}

	return resp, nil
}

func (c *client) Get(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, func(ctx context.Context) (*resty.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetHeaders(headers).
			Get(c.baseURL + endpoint)
	})
}

func (c *client) Post(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, func(ctx context.Context) (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Post(c.baseURL + endpoint)
	})
}

func (c *client) Put(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, func(ctx context.Context) (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Put(c.baseURL + endpoint)
	})
}

func (c *client) Patch(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, func(ctx context.Context) (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Patch(c.baseURL + endpoint)
	})
}

func (c *client) Delete(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, func(ctx context.Context) (*resty.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetHeaders(headers).
			Delete(c.baseURL + endpoint)
	})
}

func (c *client) WithLogging(enable bool) {
	c.logging = enable
}

func (c *client) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		timeout := time.Until(deadline)
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithTimeout(ctx, DefaultTimeout)
}

func (c *client) logRequestFailure(ctx context.Context, err error) {
	if c.logging {
		c.logger.Warn(ctx, "request_failed",
			map[string]interface{}{"event": "request_failed", "error": err.Error()})
	}
}

func (c *client) logHttpError(ctx context.Context, resp *resty.Response, err error) {
	if c.logging {
		c.logger.Warn(ctx, "Error HTTP",
			map[string]interface{}{"event": "http_error",
				"status": resp.StatusCode(),
				"error":  err.Error()})
	}
}

func (c *client) logCircuitBreakerOpen(ctx context.Context, err error) {
	if c.logging {
		c.logger.Error(ctx, err,
			map[string]interface{}{"event": "circuit_breaker_open",
				"error": err.Error()})
	}
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
