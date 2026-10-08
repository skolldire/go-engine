package engine

import (
	"os"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
)

// envVarDecodeHook resolves ${VAR} and ${VAR:-default} placeholders in string
// values while a configuration section is decoded.
//
// It lives in the core rather than in the YAML loader because every provider
// decodes its own section and all of them must resolve placeholders the same
// way; duplicating this per provider is how configuration semantics drift.
func envVarDecodeHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, _ reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		str, ok := data.(string)
		if !ok {
			return data, nil
		}
		return resolveEnvValue(str), nil
	}
}

// resolveEnvValue expands a single ${VAR} or ${VAR:-default} placeholder.
// A value that is not a placeholder, or a placeholder for an unset variable
// with no default, is returned unchanged.
func resolveEnvValue(value string) string {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return value
	}

	trimmed := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
	name, fallback, hasFallback := strings.Cut(trimmed, ":-")

	if envValue := os.Getenv(name); envValue != "" {
		return envValue
	}
	if hasFallback {
		return fallback
	}
	return value
}
