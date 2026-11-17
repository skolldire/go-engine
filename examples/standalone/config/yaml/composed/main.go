// Command composed loads settings from YAML, decodes them and validates
// the result: config/yaml reads, config decodes, validation checks.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing/fstest"

	"github.com/skolldire/go-engine/config"
	yamlsource "github.com/skolldire/go-engine/config/yaml"
	"github.com/skolldire/go-engine/validation"
)

type listener struct {
	Name string `mapstructure:"name" json:"name" validate:"required"`
	Port int    `mapstructure:"port" json:"port" validate:"gte=1,lte=65535"`
}

type settings struct {
	Service   string     `mapstructure:"service" json:"service" validate:"required"`
	AdminMail string     `mapstructure:"admin_mail" json:"admin_mail" validate:"required,email"`
	Listeners []listener `mapstructure:"listeners" json:"listeners" validate:"min=1,dive"`
}

func load(v *validation.Validator, text string) (settings, error) {
	cfg := yamlsource.DefaultConfig()
	cfg.Path = "app.yaml"
	source, err := yamlsource.New(cfg, yamlsource.Dependencies{FS: fstest.MapFS{"app.yaml": {Data: []byte(text)}}})
	if err != nil {
		return settings{}, err
	}
	loader, err := config.New(config.DefaultConfig(), config.Dependencies{Sources: []config.Source{source}})
	if err != nil {
		return settings{}, err
	}
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		return settings{}, err
	}
	var out settings
	if err := snapshot.Decode("", &out); err != nil {
		return settings{}, err
	}
	// config never validates business rules; the application does.
	return out, v.Struct(context.Background(), &out)
}

func run() error {
	v, err := validation.New(validation.DefaultConfig(), validation.Dependencies{})
	if err != nil {
		return err
	}
	got, err := load(v, "service: orders\nadmin_mail: ops@example.com\nlisteners:\n  - {name: public, port: 8080}\n  - {name: admin, port: 9090}\n")
	if err != nil || got.Service != "orders" || len(got.Listeners) != 2 || got.Listeners[1].Port != 9090 {
		return fmt.Errorf("valid settings: %+v, %w", got, err)
	}
	// Well-formed YAML with values that break the rules: the error names
	// the setting by its path.
	_, err = load(v, "service: orders\nadmin_mail: not-an-address\nlisteners:\n  - {name: public, port: 8080}\n  - {name: \"\", port: 70000}\n")
	var failed validation.Errors
	if !errors.As(err, &failed) || len(failed) != 3 {
		return fmt.Errorf("invalid settings: %w", err)
	}
	text := err.Error()
	for _, path := range []string{"admin_mail (email)", "listeners[1].name (required)", "listeners[1].port (lte=65535)"} {
		if !strings.Contains(text, path) {
			return fmt.Errorf("missing %q in %q", path, text)
		}
	}
	if strings.Contains(text, "not-an-address") || strings.Contains(text, "70000") {
		return errors.New("a setting's value reached the error text")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("config/yaml composed: YAML loaded, decoded and validated, failures named by path passed")
}
