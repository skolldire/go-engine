package env

import (
	"context"
	"sort"
	"strings"

	"github.com/skolldire/go-engine/config"
	"github.com/skolldire/go-engine/config/internal/paths"
	"github.com/skolldire/go-engine/config/internal/textvalue"
)

type Config struct {
	Bindings map[string]string
	Required []string
}
type Dependencies struct{ Env func(string) (string, bool) }
type Source struct {
	bindings map[string]string
	required map[string]bool
	lookup   func(string) (string, bool)
}

func DefaultConfig() Config { return Config{} }
func (c Config) Validate() error {
	if len(c.Bindings) == 0 {
		return &config.ConfigError{Field: "bindings", Reason: "must be nonempty"}
	}
	keys := make([]string, 0, len(c.Bindings))
	for path, variable := range c.Bindings {
		if !paths.Valid(path) || variable == "" {
			return &config.ConfigError{Field: "bindings", Reason: "invalid path or empty variable name"}
		}
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		parts := strings.Split(path, ".")
		for i := 1; i < len(parts); i++ {
			if _, exists := c.Bindings[strings.Join(parts[:i], ".")]; exists {
				return &config.ConfigError{Field: path, Reason: "binding conflicts with parent path"}
			}
		}
	}

	for _, path := range c.Required {
		if _, ok := c.Bindings[path]; !ok {
			return &config.ConfigError{Field: path, Reason: "required path has no binding"}
		}
	}
	return nil
}
func New(cfg Config, deps Dependencies) (*Source, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Env == nil {
		return nil, &config.ConfigError{Field: "env", Reason: "lookup is required"}
	}
	source := &Source{bindings: map[string]string{}, required: map[string]bool{}, lookup: deps.Env}
	for path, name := range cfg.Bindings {
		source.bindings[path] = name
	}
	for _, path := range cfg.Required {
		source.required[path] = true
	}
	return source, nil
}
func (s *Source) String() string { return "env" }
func (s *Source) Load(ctx context.Context) (config.Document, error) {
	if ctx == nil {
		return nil, config.ErrNilContext
	}
	if s == nil || s.lookup == nil {
		return nil, &config.ConfigError{Field: "env", Reason: "source must be initialized"}
	}
	result := config.Document{}
	keys := make([]string, 0, len(s.bindings))
	for path := range s.bindings {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, present := s.lookup(s.bindings[path])
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !present {
			if s.required[path] {
				return nil, &config.ConfigError{Field: path, Reason: "required binding absent from env"}
			}
			continue
		}
		current := map[string]any(result)
		parts := strings.Split(path, ".")
		for _, part := range parts[:len(parts)-1] {
			next, ok := current[part]
			if !ok {
				next = map[string]any{}
				current[part] = next
			}
			current = next.(map[string]any)
		}
		current[parts[len(parts)-1]] = textvalue.Value(value)
	}
	return result, nil
}
