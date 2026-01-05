package rest

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// NewClient creates a REST Service configured from cfg and log.
// It constructs a resty HTTP client, applies cfg.TimeOut (or DefaultTimeout when cfg.TimeOut == 0) and sets the timeout if greater than zero.
// It builds a BaseConfig with logging and resilience settings taken from cfg, instantiates a restClient with the configured base URL and HTTP client, and returns it as a Service.
func NewClient(cfg Config, log logger.Service) Service {
	httpClient := resty.New()
	timeout := cfg.TimeOut
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	if timeout > 0 {
		httpClient.SetTimeout(timeout)
	}

	baseConfig := client.BaseConfig{
		EnableLogging:  cfg.EnableLogging,
		WithResilience: cfg.WithResilience,
		Resilience:     cfg.Resilience,
		Timeout:        timeout,
	}

	c := &restClient{
		BaseClient: client.NewBaseClientWithName(baseConfig, log, "REST"),
		baseURL:    cfg.BaseURL,
		httpClient: httpClient,
	}

	return c
}

func (c *restClient) executeRequest(ctx context.Context, operationName string, reqFunc func() (*resty.Response, error)) (*resty.Response, error) {
	result, err := c.Execute(ctx, operationName, func() (interface{}, error) {
		return c.processRequest(ctx, reqFunc)
	})

	if err != nil {
		return nil, err
	}

	resp, err := client.SafeTypeAssert[*resty.Response](result)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *restClient) processRequest(ctx context.Context, reqFunc func() (*resty.Response, error)) (*resty.Response, error) {
	resp, err := reqFunc()
	if err != nil {
		if c.IsLoggingEnabled() {
			c.GetLogger().Warn(ctx, "request_failed",
				map[string]interface{}{"event": "request_failed", "error": err.Error()})
		}
		return nil, err
	}

	if err := validateResponse(resp); err != nil {
		if c.IsLoggingEnabled() {
			c.GetLogger().Warn(ctx, "Error HTTP",
				map[string]interface{}{"event": "http_error",
					"status": resp.StatusCode(),
					"error":  err.Error()})
		}
		return nil, err
	}

	return resp, nil
}

func (c *restClient) Get(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "GET "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetHeaders(headers).
			Get(c.baseURL + endpoint)
	})
}

func (c *restClient) Post(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "POST "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Post(c.baseURL + endpoint)
	})
}

func (c *restClient) Put(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "PUT "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Put(c.baseURL + endpoint)
	})
}

func (c *restClient) Patch(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
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

// validateResponse checks the HTTP response and returns an error for non-2xx statuses.
// If resp is nil it returns the error "respuesta es nil". For 2xx status codes it
// returns nil. For other statuses it returns an error formatted as
// "HTTP <code>: <status> - <bodyPreview>", where <bodyPreview> is the response
// body truncated to 200 characters (appending "..." when truncated).
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