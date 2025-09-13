package simple

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

var ErrExecution = errors.New("error executing http request")

type client struct {
	baseURL    string
	httpClient *resty.Client
	logger     logger.Service
}

var _ Service = (*client)(nil)

func NewService(d Dependencies) Service {
	d.Config.applyDefaults()
	httpClient := newRestyClient(d.Config)

	return &client{
		baseURL:    d.Config.BaseURL,
		httpClient: httpClient,
		logger:     d.Logger,
	}
}

func newRestyClient(cfg Config) *resty.Client {
	return resty.New().
		SetBaseURL(cfg.BaseURL).
		SetTimeout(cfg.TimeOut*time.Second).
		SetRetryCount(*cfg.RetryCount).
		SetRetryWaitTime(*cfg.RetryWaitTime).
		SetRetryMaxWaitTime(*cfg.RetryMaxWaitTime).
		SetHeader("User-Agent", cfg.UserAgent)
}

func (c *client) Get(ctx context.Context, endpoint string, headers map[string]string, params map[string]string) (*resty.Response, error) {
	request := c.httpClient.R().SetContext(ctx)

	if len(headers) > 0 {
		request.SetHeaders(headers)
	}
	if len(params) > 0 {
		request.SetQueryParams(params)
	}

	return c.execute(ctx, "GET", endpoint, func() (*resty.Response, error) {
		return request.Get(endpoint)
	})
}

func (c *client) Post(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	request := c.httpClient.R().SetContext(ctx)

	if len(headers) > 0 {
		request.SetHeaders(headers)
	}
	if body != nil {
		request.SetBody(body)
	}

	return c.execute(ctx, "POST", endpoint, func() (*resty.Response, error) {
		return request.Post(endpoint)
	})
}

func (c *client) Put(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	request := c.httpClient.R().SetContext(ctx)

	if len(headers) > 0 {
		request.SetHeaders(headers)
	}
	if body != nil {
		request.SetBody(body)
	}

	return c.execute(ctx, "PUT", endpoint, func() (*resty.Response, error) {
		return request.Put(endpoint)
	})
}

func (c *client) Patch(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error) {
	request := c.httpClient.R().SetContext(ctx)

	if len(headers) > 0 {
		request.SetHeaders(headers)
	}
	if body != nil {
		request.SetBody(body)
	}

	return c.execute(ctx, "PATCH", endpoint, func() (*resty.Response, error) {
		return request.Patch(endpoint)
	})
}

func (c *client) Delete(ctx context.Context, endpoint string, headers map[string]string, params map[string]string) (*resty.Response, error) {
	request := c.httpClient.R().SetContext(ctx)

	if len(headers) > 0 {
		request.SetHeaders(headers)
	}
	if len(params) > 0 {
		request.SetQueryParams(params)
	}

	return c.execute(ctx, "DELETE", endpoint, func() (*resty.Response, error) {
		return request.Delete(endpoint)
	})
}

func (c *client) execute(
	ctx context.Context,
	method, endpoint string,
	requestFunc func() (*resty.Response, error),
) (*resty.Response, error) {
	return c.runFunction(ctx, method, endpoint, requestFunc)
}

func (c *client) runFunction(
	ctx context.Context,
	method, endpoint string,
	requestFunc func() (*resty.Response, error),
) (*resty.Response, error) {
	resp, err := requestFunc()
	if err != nil {
		c.logRequestError(ctx, method, endpoint, err)
		return nil, errors.Join(err, ErrExecution)
	}
	c.logRequestSuccess(ctx, method, endpoint, resp)
	return resp, nil
}

func (c *client) logRequestError(ctx context.Context, method, endpoint string, err error) {
	data := map[string]interface{}{"error": err.Error(), "endpoint": endpoint, "base_url": c.baseURL}
	c.logger.Debug(ctx, fmt.Sprintf("%s request error", method), data)
}

func (c *client) logRequestSuccess(ctx context.Context, method, endpoint string, resp *resty.Response) {
	data := map[string]interface{}{"request_url": resp.Request.URL, "endpoint": endpoint, "base_url": c.baseURL, "status_code": resp.StatusCode()}
	c.logger.Debug(ctx, fmt.Sprintf("%s request URL details", method), data)
}
