package comparison

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/skolldire/go-engine/pkg/engine"

	c "github.com/skolldire/go-engine/config"
	envsource "github.com/skolldire/go-engine/config/env"
	vsource "github.com/skolldire/go-engine/config/viper"
	sdk "github.com/spf13/viper"
)

type comparisonConfig struct {
	ServiceName  string
	OtelEndpoint string
	Timeout      time.Duration `mapstructure:"timeout"`
	Hosts        []string      `mapstructure:"hosts"`
	Port         int
	Enabled      bool
	Optional     int
}

func comparisonLegacy(t testing.TB, body string, target *comparisonConfig) error {
	t.Helper()
	source := sdk.New()
	source.SetConfigType("yaml")
	if err := source.ReadConfig(strings.NewReader(body)); err != nil {
		return err
	}
	return engine.NewRawConfig("comparison", source.AllSettings()).Decode(target)
}
func comparisonSnapshot(t testing.TB, body string, lookup func(string) (string, bool)) *c.Snapshot {
	t.Helper()
	cfg := vsource.DefaultConfig()
	cfg.Files = []string{"app.yaml"}
	source, err := vsource.New(cfg, vsource.Dependencies{FS: fstest.MapFS{"app.yaml": {Data: []byte(body)}}})
	if err != nil {
		t.Fatal(err)
	}
	deps := c.Dependencies{Sources: []c.Source{source}}
	if lookup != nil {
		deps.Resolver, err = envsource.Resolver(lookup)
		if err != nil {
			t.Fatal(err)
		}
	}
	loader, err := c.New(c.DefaultConfig(), deps)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func TestModularConfigLegacyFixtures(t *testing.T) {
	for _, body := range []string{"timeout: 30s\nhosts: [a.example.com,b.example.com]\n", "service_name: svc\notel_endpoint: localhost:4317\n"} {
		old := comparisonConfig{Optional: 42}
		fresh := old
		if err := comparisonLegacy(t, body, &old); err != nil {
			t.Fatal(err)
		}
		if err := comparisonSnapshot(t, body, nil).Decode("", &fresh); err != nil || !reflect.DeepEqual(old, fresh) {
			t.Fatal(old, fresh, err)
		}
	}
}
func TestModularConfigDocumentedDifferences(t *testing.T) {
	for _, body := range []string{"port: '8080'\n", "enabled: '1'\n", "hosts: a.example.com,b.example.com\n"} {
		var old, fresh comparisonConfig
		if err := comparisonLegacy(t, body, &old); err != nil {
			t.Fatal(err)
		}
		if err := comparisonSnapshot(t, body, nil).Decode("", &fresh); err == nil {
			t.Fatal("weak conversion retained")
		}
	}
	t.Setenv("CONFIG_COMPARISON_EMPTY", "")
	t.Setenv("CONFIG_COMPARISON_ABSENT", "")
	for _, test := range []struct {
		body string
		want string
	}{{"service_name: ${CONFIG_COMPARISON_EMPTY:-fallback}\n", ""}, {"service_name: prefix-${CONFIG_COMPARISON_EMPTY:-fallback}\n", "prefix-"}} {
		var fresh, old comparisonConfig
		if err := comparisonLegacy(t, test.body, &old); err != nil {
			t.Fatal(err)
		}
		if old.ServiceName == test.want {
			t.Fatal("expected documented env difference")
		}
		lookup := func(name string) (string, bool) {
			if name == "CONFIG_COMPARISON_EMPTY" {
				return "", true
			}
			return "", false
		}
		if err := comparisonSnapshot(t, test.body, lookup).Decode("", &fresh); err != nil || fresh.ServiceName != test.want {
			t.Fatal(fresh, err)
		}
	}
	snapshot := comparisonSnapshot(t, "service_name: svc\n", nil)
	fresh := comparisonConfig{Optional: 42}
	if err := snapshot.Decode("missing", &fresh); err == nil || fresh.Optional != 42 {
		t.Fatal(fresh, err)
	}
	if err := comparisonSnapshot(t, "unknown_key: secret\n", nil).Decode("", &fresh); err == nil {
		t.Fatal("unknown field ignored")
	}
}
func BenchmarkConfigDecoderComparison(b *testing.B) {
	body := "service_name: svc\notel_endpoint: localhost:4317\ntimeout: 30s\nhosts: [a,b]\nport: 8080\nenabled: true\n"
	source := sdk.New()
	source.SetConfigType("yaml")
	if err := source.ReadConfig(strings.NewReader(body)); err != nil {
		b.Fatal(err)
	}
	data := source.AllSettings()
	snapshot := comparisonSnapshot(b, body, nil)
	b.Run("engine", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			target := comparisonConfig{Optional: 42}
			if err := engine.NewRawConfig("comparison", data).Decode(&target); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("config", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			target := comparisonConfig{Optional: 42}
			if err := snapshot.Decode("", &target); err != nil {
				b.Fatal(err)
			}
		}
	})
}
