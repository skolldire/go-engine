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
func InstanceNames(raw RawConfig) []string {
	if !raw.Exists() {
		return nil
	}
	entries, err := namedEntries(raw.Raw())
	if err != nil {
		return nil
	}
	return sortedKeys(entries)
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
