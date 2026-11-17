// Package resty adapts go-resty to an httpclient.Client.
//
// resty contributes its request builder and its JSON handling. Transport,
// origin rule, response limit, timeout and redirect policy all stay with the
// httpclient underneath, and so does the absence of retries.
package resty

import (
	"context"
	"fmt"
	"net/http"

	restyv2 "github.com/go-resty/resty/v2"

	"github.com/skolldire/go-engine/httpclient"
)

// Dependencies holds the borrowed httpclient. It is never closed here.
type Dependencies struct {
	HTTPClient *httpclient.Client
}

// Client wraps a resty client that is deliberately not exposed: handing it
// out would let any caller switch on resty's retry loop, and then two
// policies would each multiply the other's attempts.
type Client struct {
	resty *restyv2.Client
}

// New builds the adapter. There is no Config: everything configurable
// belongs to the httpclient.
func New(deps Dependencies) (*Client, error) {
	if deps.HTTPClient == nil || deps.HTTPClient.HTTPClient() == nil {
		return nil, &httpclient.ConfigError{Field: "HTTPClient", Reason: "is required and must come from httpclient.New"}
	}
	// The base URL has no accessor; an empty path yields it, and stays
	// correct if httpclient ever changes how it normalizes one.
	root, err := deps.HTTPClient.NewRequest(context.Background(), http.MethodGet, "", nil)
	if err != nil {
		return nil, &httpclient.ConfigError{Field: "HTTPClient", Reason: "cannot derive the base URL"}
	}
	// resty gets a copy of the shared client. Some of its request options
	// (digest authentication, for one) swap fields on the client they hold;
	// the copy keeps the same guarded transport and redirect policy while
	// leaving the one httpclient shares untouched.
	shared := *deps.HTTPClient.HTTPClient()
	r := restyv2.NewWithClient(&shared).
		SetBaseURL(root.URL.String()).
		SetRetryCount(0).
		SetPreRequestHook(refuseAcceptEncoding)
	return &Client{resty: r}, nil
}

// refuseAcceptEncoding keeps the response limit meaningful. With this header
// set by hand the transport stops decoding gzip and resty inflates the body
// itself, after the limit has already counted the compressed bytes: a small
// response could then expand without bound. Left alone, the transport
// negotiates gzip and the limit counts decoded bytes.
func refuseAcceptEncoding(_ *restyv2.Client, req *http.Request) error {
	if req.Header.Get("Accept-Encoding") != "" {
		return fmt.Errorf("%w: Accept-Encoding is negotiated by the transport", httpclient.ErrInvalidCall)
	}
	return nil
}

// R returns resty's request builder bound to ctx, for path and query
// parameters, typed results, multipart or streaming with
// SetDoNotParseResponse.
func (c *Client) R(ctx context.Context) *restyv2.Request {
	return c.resty.R().SetContext(ctx)
}

func (c *Client) send(ctx context.Context, method, path string, body any, headers map[string]string) (*restyv2.Response, error) {
	if ctx == nil {
		return nil, httpclient.ErrNilContext
	}
	req := c.R(ctx).SetHeaders(headers)
	if body != nil {
		req.SetBody(body)
	}
	return req.Execute(method, path)
}

// The verb helpers return the response with a nil error for any status; use
// CheckStatus to turn a non-2xx into an error.

func (c *Client) Get(ctx context.Context, path string, headers map[string]string) (*restyv2.Response, error) {
	return c.send(ctx, http.MethodGet, path, nil, headers)
}

func (c *Client) Post(ctx context.Context, path string, body any, headers map[string]string) (*restyv2.Response, error) {
	return c.send(ctx, http.MethodPost, path, body, headers)
}

func (c *Client) Put(ctx context.Context, path string, body any, headers map[string]string) (*restyv2.Response, error) {
	return c.send(ctx, http.MethodPut, path, body, headers)
}

func (c *Client) Patch(ctx context.Context, path string, body any, headers map[string]string) (*restyv2.Response, error) {
	return c.send(ctx, http.MethodPatch, path, body, headers)
}

func (c *Client) Delete(ctx context.Context, path string, headers map[string]string) (*restyv2.Response, error) {
	return c.send(ctx, http.MethodDelete, path, nil, headers)
}

// CheckStatus returns nil for 2xx and a *httpclient.StatusError otherwise,
// so one classifier serves both clients. The error names the status code
// only; the body resty buffered stays on resp.
func CheckStatus(resp *restyv2.Response) error {
	if resp == nil || resp.RawResponse == nil {
		return httpclient.ErrInvalidCall
	}
	return httpclient.CheckStatus(resp.RawResponse)
}
