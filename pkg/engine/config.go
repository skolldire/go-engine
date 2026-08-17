package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skolldire/go-engine/pkg/health"
	"github.com/skolldire/go-engine/pkg/router"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/spf13/viper"
)

// Config is the core configuration: only the sections the core itself owns are
// typed here.
//
// Everything else lands in Components untouched. That single `,remain` field is
// the fix for the coupling the audit identified: the previous configuration
// struct named the concrete Config type of all fifteen adapters, so importing
// a YAML reader pulled in forty modules.
type Config struct {
	Router router.Config `mapstructure:"router"`
	Log    logger.Config `mapstructure:"log"`
	Health health.Config `mapstructure:"health"`

	// Components holds every section the core does not own, undecoded. Each
	// Provider decodes the section it declares via ConfigKey.
	Components map[string]any `mapstructure:",remain"`
}

// Section returns the raw configuration section named key.
func (c *Config) Section(key string) RawConfig {
	if c == nil || c.Components == nil {
		return NewMissingRawConfig(key)
	}
	value, ok := c.Components[key]
	if !ok {
		return NewMissingRawConfig(key)
	}
	return NewRawConfig(key, value)
}

// loadConfig reads and merges the configuration files under dir, then splits
// them into the typed core sections and the untouched remainder.
func loadConfig(dir string, files []string) (*Config, error) {
	if dir == "" {
		dir = defaultConfigDir()
	}
	if len(files) == 0 {
		files = []string{"application"}
	}

	merged := viper.New()
	loadedAny := false

	for _, name := range files {
		v := viper.New()
		v.AddConfigPath(dir)
		v.SetConfigName(name)

		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if ok := asConfigNotFound(err, &notFound); ok || os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read config %q in %q: %w", name, dir, err)
		}

		loadedAny = true
		if err := merged.MergeConfigMap(v.AllSettings()); err != nil {
			return nil, fmt.Errorf("merge config %q: %w", name, err)
		}
	}

	if !loadedAny {
		return nil, fmt.Errorf("no configuration file found in %q (looked for %s)",
			dir, strings.Join(files, ", "))
	}

	var cfg Config
	decoder, err := newDecoder(&cfg)
	if err != nil {
		return nil, fmt.Errorf("build config decoder: %w", err)
	}
	if err := decoder.Decode(merged.AllSettings()); err != nil {
		return nil, fmt.Errorf("map configuration: %w", err)
	}

	return &cfg, nil
}

// asConfigNotFound reports whether err is viper's "file not found" sentinel.
func asConfigNotFound(err error, target *viper.ConfigFileNotFoundError) bool {
	e, ok := err.(viper.ConfigFileNotFoundError) //nolint:errorlint // viper returns this value type unwrapped
	if ok {
		*target = e
	}
	return ok
}

// defaultConfigDir mirrors the historical lookup: CONF_DIR wins, then the
// conventional ./config directory.
func defaultConfigDir() string {
	if dir := os.Getenv("CONF_DIR"); dir != "" {
		return dir
	}
	return filepath.Clean("config")
}
