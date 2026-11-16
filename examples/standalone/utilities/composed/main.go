package main

import (
	"context"
	"fmt"
	"os"
	"testing/fstest"

	"github.com/skolldire/go-engine/config"
	configviper "github.com/skolldire/go-engine/config/viper"
	"github.com/skolldire/go-engine/utilities/profile"
)

type toolConfig struct {
	Name    string `mapstructure:"name"`
	Workers int    `mapstructure:"workers"`
	Enabled bool   `mapstructure:"enabled"`
}

func run(scope string) error {
	p, err := profile.ParseScope(scope)
	if err != nil {
		return err
	}
	files := fstest.MapFS{"base.yaml": {Data: []byte("tool:\n  name: base\n  workers: 1\n")}}
	for _, known := range []profile.Profile{profile.Local, profile.Test, profile.Stage, profile.Prod} {
		files[known.String()+".yaml"] = &fstest.MapFile{Data: []byte("tool:\n  name: " + known.String() + "\n  workers: 7\n")}
	}
	sourceCfg := configviper.DefaultConfig()
	sourceCfg.Files = []string{"base.yaml", p.String() + ".yaml"}
	source, err := configviper.New(sourceCfg, configviper.Dependencies{FS: files})
	if err != nil {
		return err
	}
	loader, err := config.New(config.DefaultConfig(), config.Dependencies{Sources: []config.Source{source}})
	if err != nil {
		return err
	}
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		return err
	}
	target := toolConfig{Name: "default", Workers: 2, Enabled: true}
	if err = snapshot.Decode("tool", &target); err != nil {
		return err
	}
	if target.Name != p.String() || target.Workers != 7 || !target.Enabled {
		return fmt.Errorf("profile configuration precedence mismatch")
	}
	return nil
}
func main() {
	if err := run(os.Getenv("SCOPE")); err != nil {
		panic(err)
	}
	fmt.Println("utilities composed: explicit SCOPE, profile and YAML precedence verified")
}
