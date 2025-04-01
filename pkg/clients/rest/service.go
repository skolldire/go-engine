package rest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

func NewClient(cfg Config, log logger.Service) Service {
	httpClient := resty.New()
	if cfg.TimeOut > 0 {
		httpClient.SetTimeout(cfg.TimeOut * time.Second)
	}

	c := &client{
		baseURL:    cfg.BaseURL,
		httpClient: httpClient,
		logger:     log,
		logging:    cfg.EnableLogging,
	}

	if cfg.WithResilience {
		resilienceService := resilience.NewResilienceService(cfg.Resilience, log)
		c.resilience = resilienceService
	}

	return c
}

func (c *client) executeRequest(ctx context.Context, operationName string, reqFunc func() (*resty.Response, error)) (*resty.Response, error) {
	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	logFields := map[string]interface{}{"operation": operationName}

	if c.resilience != nil {
		return c.executeWithResilience(ctx, operationName, reqFunc, logFields)
	}

	return c.executeDirectly(ctx, operationName, reqFunc, logFields)
}

func (c *client) executeWithResilience(ctx context.Context, operationName string, reqFunc func() (*resty.Response, error), logFields map[string]interface{}) (*resty.Response, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Iniciando petición HTTP con resiliencia: %s", operationName), logFields)
	}

	result, err := c.resilience.Execute(ctx, func() (interface{}, error) {
		return c.processRequest(ctx, reqFunc)
	})

	c.logCompletionStatus(ctx, operationName, err, true, logFields)

	if err != nil {
		return nil, err
	}

	return result.(*resty.Response), nil
}

func (c *client) executeDirectly(ctx context.Context, operationName string, reqFunc func() (*resty.Response, error), logFields map[string]interface{}) (*resty.Response, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Iniciando petición HTTP: %s", operationName), logFields)
	}

	resp, err := c.processRequest(ctx, reqFunc)

	c.logCompletionStatus(ctx, operationName, err, false, logFields)

	return resp, err
}

func (c *client) processRequest(ctx context.Context, reqFunc func() (*resty.Response, error)) (*resty.Response, error) {
	resp, err := reqFunc()
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

func (c *client) logCompletionStatus(ctx context.Context, operationName string, err error, withResilience bool, logFields map[string]interface{}) {
	if !c.logging {
		return
	}

	resilienceText := ""
	if withResilience {
		resilienceText = " con resiliencia"
	}

	if err != nil {
		c.logger.Error(ctx, fmt.Errorf("error en petición HTTP: %w", err), logFields)
	} else {
		c.logger.Debug(ctx, fmt.Sprintf("Petición HTTP completada%s: %s", resilienceText, operationName), logFields)
	}
}

func (c *client) Get(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "GET "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetHeaders(headers).
			Get(c.baseURL + endpoint)
	})
}

func (c *client) Post(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "POST "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Post(c.baseURL + endpoint)
	})
}

func (c *client) Put(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "PUT "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Put(c.baseURL + endpoint)
	})
}

func (c *client) Patch(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "PATCH "+endpoint, func() (*resty.Response, error) {
		return c.httpClient.R().
			SetBody(body).
			SetContext(ctx).
			SetHeaders(headers).
			Patch(c.baseURL + endpoint)
	})
}

func (c *client) Delete(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error) {
	return c.executeRequest(ctx, "DELETE "+endpoint, func() (*resty.Response, error) {
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
