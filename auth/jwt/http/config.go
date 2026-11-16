package http

import (
	"context"
	nethttp "net/http"
	"path"
	"reflect"
	"strings"

	"github.com/skolldire/go-engine/auth/jwt"
)

type Config struct {
	Header         string   `mapstructure:"header" json:"header"`
	SkipPaths      []string `mapstructure:"skip_paths" json:"skip_paths"`
	RequiredGroups []string `mapstructure:"required_groups" json:"required_groups"`
}
type Verifier interface {
	Verify(context.Context, string) (jwt.Claims, error)
}
type ErrorWriter func(nethttp.ResponseWriter, *nethttp.Request, int, error)
type Dependencies struct {
	Verifier Verifier
	OnError  ErrorWriter
}

func DefaultConfig() Config { return Config{Header: "Authorization"} }
func headerToken(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)
		if !valid {
			return false
		}
	}
	return true
}
func canonicalPath(s string) bool {
	if !strings.HasPrefix(s, "/") || strings.ContainsAny(s, "\\%") || strings.Contains(s, "//") {
		return false
	}
	clean := path.Clean(s)
	if strings.HasSuffix(s, "/") && clean != "/" {
		clean += "/"
	}
	return clean == s
}
func (c Config) Validate() error {
	if !headerToken(c.Header) {
		return &jwt.ConfigError{Field: "Header", Reason: "invalid HTTP header name"}
	}
	for _, p := range c.SkipPaths {
		if !canonicalPath(p) {
			return &jwt.ConfigError{Field: "SkipPaths", Reason: "requires canonical absolute paths"}
		}
	}
	for _, g := range c.RequiredGroups {
		if g == "" {
			return &jwt.ConfigError{Field: "RequiredGroups", Reason: "empty group"}
		}
	}
	return nil
}
func nilPointer(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
