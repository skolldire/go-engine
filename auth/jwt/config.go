package jwt

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalidConfig     = errors.New("invalid JWT configuration")
	ErrInvalidCall       = errors.New("invalid JWT call")
	ErrNilContext        = errors.New("nil JWT context")
	ErrInvalidToken      = errors.New("invalid token")
	ErrExpiredToken      = fmt.Errorf("%w: expired token", ErrInvalidToken)
	ErrUnknownKey        = fmt.Errorf("%w: unknown key", ErrInvalidToken)
	ErrKeySetUnavailable = errors.New("key set unavailable")
	ErrForbidden         = errors.New("forbidden")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "jwt: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	Issuer         string        `mapstructure:"issuer" json:"issuer"`
	Audience       string        `mapstructure:"audience" json:"audience"`
	AudienceClaim  string        `mapstructure:"audience_claim" json:"audience_claim"`
	GroupsClaim    string        `mapstructure:"groups_claim" json:"groups_claim"`
	JWKSURL        string        `mapstructure:"jwks_url" json:"jwks_url"`
	Algorithms     []string      `mapstructure:"algorithms" json:"algorithms"`
	CacheTTL       time.Duration `mapstructure:"cache_ttl" json:"cache_ttl"`
	RefreshTimeout time.Duration `mapstructure:"refresh_timeout" json:"refresh_timeout"`
	ClockSkew      time.Duration `mapstructure:"clock_skew" json:"clock_skew"`
	StaleGrace     time.Duration `mapstructure:"stale_grace" json:"stale_grace"`
	MaxTokenBytes  int           `mapstructure:"max_token_bytes" json:"max_token_bytes"`
	MaxJWKSBytes   int           `mapstructure:"max_jwks_bytes" json:"max_jwks_bytes"`
}
type Dependencies struct{ HTTPClient *http.Client }
type Claims struct {
	Subject   string
	Issuer    string
	Audience  []string
	ID        string
	Expiry    time.Time
	NotBefore time.Time
	IssuedAt  time.Time
	Groups    []string
	Custom    map[string]any
}

func DefaultConfig() Config {
	return Config{AudienceClaim: "aud", GroupsClaim: "groups", Algorithms: []string{"RS256"}, CacheTTL: time.Hour, RefreshTimeout: 5 * time.Second, ClockSkew: 30 * time.Second, MaxTokenBytes: 16 << 10, MaxJWKSBytes: 1 << 20}
}
func allowedAlgorithm(s string) bool {
	switch s {
	case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384":
		return true
	}
	return false
}
func validEndpoint(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	h := u.Hostname()
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip, e := netip.ParseAddr(h)
	return e == nil && ip.IsLoopback()
}
func (c Config) Validate() error {
	checks := []struct {
		field string
		bad   bool
	}{{"Issuer", c.Issuer == ""}, {"Audience", c.Audience == ""}, {"AudienceClaim", c.AudienceClaim == ""}, {"GroupsClaim", c.GroupsClaim == ""}, {"JWKSURL", !validEndpoint(c.JWKSURL)}}
	for _, x := range checks {
		if x.bad {
			return &ConfigError{x.field, "required or invalid value"}
		}
	}
	if len(c.Algorithms) == 0 {
		return &ConfigError{"Algorithms", "is required"}
	}
	for _, a := range c.Algorithms {
		if !allowedAlgorithm(a) {
			return &ConfigError{"Algorithms", "unsupported algorithm"}
		}
	}
	for _, x := range []struct {
		field string
		bad   bool
	}{{"CacheTTL", c.CacheTTL <= 0}, {"RefreshTimeout", c.RefreshTimeout <= 0}, {"ClockSkew", c.ClockSkew < 0}, {"StaleGrace", c.StaleGrace < 0}, {"MaxTokenBytes", c.MaxTokenBytes <= 0}, {"MaxJWKSBytes", c.MaxJWKSBytes <= 0}} {
		if x.bad {
			return &ConfigError{x.field, "invalid duration or size"}
		}
	}
	return nil
}
func RequireGroup(c Claims, group string) error {
	if group == "" {
		return fmt.Errorf("%w: group", ErrInvalidCall)
	}
	for _, g := range c.Groups {
		if g == group {
			return nil
		}
	}
	return ErrForbidden
}
