// Command minimal loads layered YAML from an in-memory filesystem and
// decodes it over defaults.
package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing/fstest"
	"time"

	"github.com/skolldire/go-engine/config"
	yamlsource "github.com/skolldire/go-engine/config/yaml"
)

type database struct {
	Host    string        `mapstructure:"host"`
	Port    int           `mapstructure:"port"`
	Timeout time.Duration `mapstructure:"timeout"`
	SSL     bool          `mapstructure:"ssl"`
}

type settings struct {
	Name     string `mapstructure:"name"`
	LogLevel string `mapstructure:"log_level"`
	// Database is the block the anchor names; unknown keys are rejected,
	// so it is declared like any other setting.
	Database database          `mapstructure:"database"`
	Primary  database          `mapstructure:"primary"`
	Replica  database          `mapstructure:"replica"`
	Labels   map[string]string `mapstructure:"labels"`
	Features []string          `mapstructure:"features"`
}

func defaults() settings {
	return settings{Name: "unnamed", LogLevel: "info", Primary: database{Port: 5432, Timeout: 5 * time.Second}}
}

const base = `
name: orders
# An anchor names a block once; aliases and merge keys reuse it.
database: &db
  host: db.internal
  port: 5432
  timeout: 10s
primary: *db
replica:
  <<: *db
  host: replica.internal   # the explicit key wins over the merged one
labels:
  tier: backend
  owner: platform
features: [search, export]
`

const override = `
log_level: debug
primary:
  port: 0x1538             # 5432 written in hexadecimal: decoded as the number
  ssl: true
labels:
  owner: ~                 # null removes the key set by the base file
  region: eu
features: [search]         # lists replace, maps merge
`

func source(files fstest.MapFS, path string, required bool) (config.Source, error) {
	cfg := yamlsource.DefaultConfig()
	cfg.Path = path
	cfg.Required = required
	return yamlsource.New(cfg, yamlsource.Dependencies{FS: files})
}

func load(files fstest.MapFS, paths ...string) (settings, error) {
	sources := make([]config.Source, 0, len(paths))
	for _, path := range paths {
		// Only the first file must exist.
		s, err := source(files, path, len(sources) == 0)
		if err != nil {
			return settings{}, err
		}
		sources = append(sources, s)
	}
	loader, err := config.New(config.DefaultConfig(), config.Dependencies{Sources: sources})
	if err != nil {
		return settings{}, err
	}
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		return settings{}, err
	}
	out := defaults()
	if err := snapshot.Decode("", &out); err != nil {
		return settings{}, err
	}
	return out, nil
}

func run() error {
	files := fstest.MapFS{"base.yaml": {Data: []byte(base)}, "override.yaml": {Data: []byte(override)}}
	got, err := load(files, "base.yaml", "override.yaml", "local.yaml")
	if err != nil {
		return err
	}
	want := settings{
		Name: "orders", LogLevel: "debug",
		Database: database{Host: "db.internal", Port: 5432, Timeout: 10 * time.Second},
		Primary:  database{Host: "db.internal", Port: 5432, Timeout: 10 * time.Second, SSL: true},
		Replica:  database{Host: "replica.internal", Port: 5432, Timeout: 10 * time.Second},
		Labels:   map[string]string{"tier": "backend", "region": "eu"},
		Features: []string{"search"},
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("decoded %+v", got)
	}

	// The source is strict. A duplicate key is an error that names the
	// position and the key path, and never the value.
	files["broken.yaml"] = &fstest.MapFile{Data: []byte("primary:\n  password: hunter2\n  password: hunter3\n")}
	_, err = load(files, "broken.yaml")
	if !errors.Is(err, config.ErrInvalidConfig) || !strings.Contains(err.Error(), "primary.password") || !strings.Contains(err.Error(), "line 3") || strings.Contains(err.Error(), "hunter") {
		return fmt.Errorf("duplicate key: %w", err)
	}
	// A quoted number in a file is text, and text does not become a number.
	files["quoted.yaml"] = &fstest.MapFile{Data: []byte("primary:\n  port: \"5432\"\n")}
	if _, err = load(files, "quoted.yaml"); err == nil {
		return errors.New("a quoted number was converted")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("config/yaml minimal: base and override, aliases, merge keys, null removal and strict errors passed")
}
