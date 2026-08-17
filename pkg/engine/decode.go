package engine

import "fmt"

// DecodeNamed extracts one named instance from the multi-instance layout used
// throughout the configuration file:
//
//	sqs_clients:
//	  - orders:
//	      endpoint: "..."
//	  - billing:
//	      endpoint: "..."
//
// It is generic over the adapter's own Config type, which is what lets the core
// support named instances without knowing a single adapter type.
//
// A missing section or a missing instance is reported as an error: asking for
// "sqs:orders" when the file declares no such client is a configuration
// mistake, not a silent zero value.
func DecodeNamed[T any](raw RawConfig, instance string) (T, error) {
	var cfg T

	if !raw.Exists() {
		return cfg, fmt.Errorf("no configuration section found")
	}

	entries, err := namedEntries(raw.Raw())
	if err != nil {
		return cfg, err
	}

	value, ok := entries[instance]
	if !ok {
		return cfg, fmt.Errorf("instance %q not declared (available: %v)", instance, sortedKeys(entries))
	}

	if err := NewRawConfig(instance, value).Decode(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// InstanceNames lists the instances declared in a multi-instance section. It
// lets a preset register every client the file declares without the caller
// naming them one by one.
//
// A malformed section is an error, not an empty list. Returning nil silently
// meant a typo in the YAML produced a service that started with none of its
// clients and no indication why.
func InstanceNames(raw RawConfig) ([]string, error) {
	if !raw.Exists() {
		return nil, nil
	}
	entries, err := namedEntries(raw.Raw())
	if err != nil {
		return nil, fmt.Errorf("section %q: %w", raw.Key(), err)
	}
	return sortedKeys(entries), nil
}

// namedEntries flattens the section into instance name -> raw config. Both the
// list-of-maps form above and a plain map form are accepted, since YAML authors
// write both.
func namedEntries(value any) (map[string]any, error) {
	out := make(map[string]any)

	switch v := value.(type) {
	case []any:
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected a map of instance name to config, got %T", item)
			}
			for name, cfg := range m {
				out[name] = cfg
			}
		}
	case map[string]any:
		for name, cfg := range v {
			out[name] = cfg
		}
	default:
		return nil, fmt.Errorf("expected a list or map of named configs, got %T", value)
	}

	return out, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

// EachInstance calls add for every instance declared under key, reporting a
// malformed section instead of skipping it.
//
// Presets use it so registering "every queue in the YAML" stays one line per
// adapter rather than four lines of error plumbing repeated fifteen times.
func EachInstance(cfg *Config, key string, add func(name string)) error {
	names, err := InstanceNames(cfg.Section(key))
	if err != nil {
		return err
	}
	for _, name := range names {
		add(name)
	}
	return nil
}
