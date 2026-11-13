package env

import (
	"context"
	"strings"

	"github.com/skolldire/go-engine/config"
)

type environmentResolver struct{ lookup func(string) (string, bool) }

func Resolver(lookup func(string) (string, bool)) (config.Resolver, error) {
	if lookup == nil {
		return nil, &config.ConfigError{Field: "env", Reason: "lookup is required"}
	}
	return environmentResolver{lookup: lookup}, nil
}
func (r environmentResolver) Resolve(ctx context.Context, reference string) (string, error) {
	if ctx == nil {
		return "", config.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name := reference
	if scheme, remainder, hasScheme := strings.Cut(reference, ":"); hasScheme {
		if scheme != "env" {
			return "", &config.ConfigError{Field: "reference", Reason: "unsupported environment reference scheme"}
		}
		name = remainder
	}
	if name == "" {
		return "", &config.ConfigError{Field: "reference", Reason: "empty environment reference name"}
	}
	value, ok := r.lookup(name)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !ok {
		return "", config.ErrReferenceNotFound
	}
	return value, nil
}
