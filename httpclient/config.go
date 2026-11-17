// Package httpclient is an outbound HTTP client with a bounded transport,
// a response size limit, explicit redirects and a single allowed origin.
//
// It deliberately carries no retry, breaker, rate limit, logging, metric or
// trace of its own: those are composed from outside, one policy owning the
// budget, so two layers never multiply each other's attempts.
package httpclient

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidConfig      = errors.New("invalid httpclient configuration")
	ErrInvalidCall        = errors.New("invalid httpclient call")
	ErrNilContext         = errors.New("nil httpclient context")
	ErrResponseTooLarge   = errors.New("httpclient response too large")
	ErrOriginNotAllowed   = errors.New("httpclient origin not allowed")
	ErrRedirectNotAllowed = errors.New("httpclient redirect not allowed")
)

// ConfigError names the rejected field with a fixed reason. It never carries
// the rejected value: a base or proxy URL may embed credentials.
type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "httpclient: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

// TransportConfig describes the transport the module builds for itself. It is
// a separate struct so that "the transport is borrowed" has one checkable
// meaning: this value is the zero value.
type TransportConfig struct {
	ConnectTimeout        time.Duration `mapstructure:"connect_timeout" json:"connect_timeout"`
	ResponseHeaderTimeout time.Duration `mapstructure:"response_header_timeout" json:"response_header_timeout"`
	IdleTimeout           time.Duration `mapstructure:"idle_timeout" json:"idle_timeout"`
	MaxIdleConns          int           `mapstructure:"max_idle_conns" json:"max_idle_conns"`
	MaxIdleConnsPerHost   int           `mapstructure:"max_idle_conns_per_host" json:"max_idle_conns_per_host"`
	MaxConnsPerHost       int           `mapstructure:"max_conns_per_host" json:"max_conns_per_host"`
	ProxyURL              string        `mapstructure:"proxy_url" json:"proxy_url"`
}

type Config struct {
	BaseURL          string          `mapstructure:"base_url" json:"base_url"`
	RequestTimeout   time.Duration   `mapstructure:"request_timeout" json:"request_timeout"`
	MaxResponseBytes int64           `mapstructure:"max_response_bytes" json:"max_response_bytes"`
	FollowRedirects  bool            `mapstructure:"follow_redirects" json:"follow_redirects"`
	MaxRedirects     int             `mapstructure:"max_redirects" json:"max_redirects"`
	Transport        TransportConfig `mapstructure:"transport" json:"transport"`
}

// Dependencies are caller-owned collaborators. RoundTripper is borrowed and
// never modified or closed; TLSConfig is cloned.
type Dependencies struct {
	RoundTripper  http.RoundTripper
	TLSConfig     *tls.Config
	WrapTransport func(http.RoundTripper) http.RoundTripper
}

// DefaultConfig leaves BaseURL, MaxResponseBytes and Transport.MaxConnsPerHost
// empty on purpose: no value of those is safe for every upstream, so the
// caller has to state them.
func DefaultConfig() Config {
	return Config{
		RequestTimeout: 10 * time.Second,
		Transport: TransportConfig{
			ConnectTimeout:        5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			IdleTimeout:           90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
}

// Validate checks fields in declaration order and completes nothing: a zero
// literal is rejected instead of being replaced by a default.
//
// A zero Transport is accepted here because it is how a borrowed RoundTripper
// is declared; New rejects it when no RoundTripper is supplied.
func (cfg Config) Validate() error {
	if _, _, err := parseBase(cfg.BaseURL); err != nil {
		return err
	}
	if cfg.RequestTimeout <= 0 {
		return &ConfigError{"RequestTimeout", "must be positive"}
	}
	if cfg.MaxResponseBytes <= 0 {
		return &ConfigError{"MaxResponseBytes", "must be positive"}
	}
	switch {
	case cfg.MaxRedirects < 0:
		return &ConfigError{"MaxRedirects", "must not be negative"}
	case cfg.FollowRedirects && cfg.MaxRedirects == 0:
		return &ConfigError{"MaxRedirects", "must be positive when FollowRedirects is set"}
	case !cfg.FollowRedirects && cfg.MaxRedirects != 0:
		return &ConfigError{"MaxRedirects", "requires FollowRedirects"}
	}
	if cfg.Transport == (TransportConfig{}) {
		return nil
	}
	return cfg.Transport.validate()
}

func (t TransportConfig) validate() error {
	for _, d := range []struct {
		field string
		value time.Duration
	}{
		{"Transport.ConnectTimeout", t.ConnectTimeout},
		{"Transport.ResponseHeaderTimeout", t.ResponseHeaderTimeout},
		{"Transport.IdleTimeout", t.IdleTimeout},
	} {
		if d.value <= 0 {
			return &ConfigError{d.field, "must be positive"}
		}
	}
	for _, n := range []struct {
		field string
		value int
	}{
		{"Transport.MaxIdleConns", t.MaxIdleConns},
		{"Transport.MaxIdleConnsPerHost", t.MaxIdleConnsPerHost},
		{"Transport.MaxConnsPerHost", t.MaxConnsPerHost},
	} {
		if n.value <= 0 {
			return &ConfigError{n.field, "must be positive"}
		}
	}
	_, err := t.proxy()
	return err
}

// proxy returns nil when no proxy is configured. The environment is never
// consulted: a proxy that appears through HTTP_PROXY would silently reroute
// every call of a library that promised one origin.
func (t TransportConfig) proxy() (*url.URL, error) {
	if t.ProxyURL == "" {
		return nil, nil
	}
	invalid := &ConfigError{"Transport.ProxyURL", "must be an http, https or socks5 URL with a host"}
	u, err := url.Parse(t.ProxyURL)
	if err != nil {
		return nil, invalid
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, invalid
	}
	if u.Hostname() == "" || !validPort(u.Port()) || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, invalid
	}
	return u, nil
}

// origin is the scheme, host and effective port a request may reach.
type origin struct {
	scheme, host string
	port         int
}

// originOf canonicalizes without IDNA or trailing-dot folding: two spellings
// of one host compare as different and fail closed.
func originOf(u *url.URL) (origin, bool) {
	scheme := strings.ToLower(u.Scheme)
	port := 0
	switch scheme {
	case "http":
		port = 80
	case "https":
		port = 443
	default:
		return origin{}, false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || !validPort(u.Port()) {
		return origin{}, false
	}
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}
	return origin{scheme, host, port}, true
}

func validPort(p string) bool {
	if p == "" {
		return true
	}
	n, err := strconv.Atoi(p)
	return err == nil && n > 0 && n <= 65535 && len(p) <= 5
}

// parseBase accepts scheme://host[:port][/clean/prefix] and nothing else, so
// that joining a relative path under it cannot be ambiguous.
func parseBase(raw string) (*url.URL, origin, error) {
	if raw == "" {
		return nil, origin{}, &ConfigError{"BaseURL", "is required"}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, origin{}, &ConfigError{"BaseURL", "is not a valid URL"}
	}
	o, ok := originOf(u)
	if !ok || u.Opaque != "" {
		return nil, origin{}, &ConfigError{"BaseURL", "must be an absolute http or https URL with a host"}
	}
	if u.User != nil {
		return nil, origin{}, &ConfigError{"BaseURL", "must not contain userinfo"}
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || strings.Contains(raw, "#") {
		return nil, origin{}, &ConfigError{"BaseURL", "must not contain a query or fragment"}
	}
	if !cleanPrefix(u) {
		return nil, origin{}, &ConfigError{"BaseURL", "path must be a plain prefix without dot, empty or encoded segments"}
	}
	base := &url.URL{Scheme: o.scheme, Host: strings.ToLower(u.Host), Path: strings.TrimSuffix(u.Path, "/")}
	return base, o, nil
}

func cleanPrefix(u *url.URL) bool {
	if u.RawPath != "" {
		return false
	}
	p := strings.TrimSuffix(u.Path, "/")
	if p == "" {
		return true
	}
	for _, r := range p {
		if r <= 0x20 || r == 0x7f || r == '\\' || r == '%' {
			return false
		}
	}
	for _, segment := range strings.Split(p[1:], "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return p[0] == '/'
}
