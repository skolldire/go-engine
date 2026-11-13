package config_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	ms "github.com/go-viper/mapstructure/v2"
	c "github.com/skolldire/go-engine/config"
	e "github.com/skolldire/go-engine/config/env"
)

func TestMapstructureRangeCharacterization(t *testing.T) {
	for _, input := range []any{256, 1.5} {
		var out uint8
		decoder, err := ms.NewDecoder(&ms.DecoderConfig{Result: &out, WeaklyTypedInput: false})
		if err != nil {
			t.Fatal(err)
		}
		if err := decoder.Decode(input); err != nil {
			t.Fatal("update characterization", err)
		}
		var strict uint8
		requireError(t, load(t, source(c.Document{"value": input})).Decode("value", &strict), c.ErrInvalidConfig)
	}
}
func TestLegacyNameMatchingAndAmbiguity(t *testing.T) {
	for _, key := range []string{"ServiceName", "service_name", "SERVICE_NAME", "servicename"} {
		target := struct{ ServiceName string }{}
		if err := load(t, source(c.Document{key: "ok"})).Decode("", &target); err != nil || target.ServiceName != "ok" {
			t.Fatal(key, target, err)
		}
	}
	target := struct {
		Port int `mapstructure:"listen_port" config:"wrong"`
	}{42}
	if err := load(t, source(c.Document{"listen_port": 8080})).Decode("", &target); err != nil || target.Port != 8080 {
		t.Fatal(target, err)
	}
	duplicate := struct {
		FooBar int
		Other  int `mapstructure:"foo_bar"`
	}{1, 2}
	requireError(t, load(t, source(c.Document{})).Decode("", &duplicate), c.ErrInvalidConfig)
	requireError(t, load(t, source(c.Document{"port": 1, "PORT": 2})).Decode("", &struct{ Port int }{}), c.ErrInvalidConfig)
}
func TestNestedMapDefaultsAndListReplacement(t *testing.T) {
	type item struct {
		A int
		B int
	}
	target := struct {
		Rows   map[string]item
		Nested map[string]map[string]int
		List   []item
	}{Rows: map[string]item{"x": {A: 1, B: 2}, "untouched": {A: 7}}, Nested: map[string]map[string]int{"x": {"seed": 1}}, List: []item{{A: 9, B: 9}, {A: 8}}}
	s := load(t, source(c.Document{"rows": map[string]any{"x": map[string]any{"b": 3}}, "nested": map[string]any{"x": map[string]any{"new": 2}}, "list": []any{map[string]any{"b": 4}}}))
	if err := s.Decode("", &target); err != nil {
		t.Fatal(err)
	}
	if target.Rows["x"] != (item{1, 3}) || target.Rows["untouched"].A != 7 || target.Nested["x"]["seed"] != 1 || len(target.List) != 1 || target.List[0] != (item{B: 4}) {
		t.Fatalf("seed merge: %+v", target)
	}
}
func TestTextOriginListsAndFileRejection(t *testing.T) {
	for _, raw := range []string{"1,2", "1,,2", ",1", "1,", "+1,2", "0x1,2"} {
		src, _ := e.New(e.Config{Bindings: map[string]string{"values": "X"}}, e.Dependencies{Env: func(string) (string, bool) { return raw, true }})
		var target struct{ Values []int }
		err := load(t, src).Decode("", &target)
		if (err == nil) != (raw == "1,2") {
			t.Fatal(raw, err)
		}
		if err == nil && !reflect.DeepEqual(target.Values, []int{1, 2}) {
			t.Fatal(target)
		}
	}
	var file struct{ Values []string }
	requireError(t, load(t, source(c.Document{"values": "a,b"})).Decode("", &file), c.ErrInvalidConfig)
	var str string
	requireError(t, load(t, source(c.Document{"n": 42})).Decode("n", &str), c.ErrInvalidConfig)
}
func TestMapstructureDiagnosticsDoNotExposeValues(t *testing.T) {
	target := struct {
		Accounts map[string]struct{ Tokens []int }
	}{}
	s := load(t, source(c.Document{"accounts": map[string]any{"one": map[string]any{"tokens": []any{1, "ultra-private"}}}}))
	err := s.Decode("", &target)
	requireError(t, err, c.ErrInvalidConfig)
	var ce *c.ConfigError
	if !errors.As(err, &ce) || ce.Field != "accounts.one.tokens[1]" || strings.Contains(err.Error(), "ultra-private") {
		t.Fatal(err)
	}
	var unsafe *ms.DecodeError
	if errors.As(err, &unsafe) {
		t.Fatal("SDK error with raw input escaped")
	}
}
func TestResolverAPI(t *testing.T) {
	if _, err := e.Resolver(nil); !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
	r, err := e.Resolver(func(name string) (string, bool) {
		if name == "EMPTY" {
			return "", true
		}
		if name == "TIMEOUT" {
			return "2s", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"EMPTY", "env:EMPTY"} {
		value, err := r.Resolve(context.Background(), name)
		if err != nil || value != "" {
			t.Fatal(value, err)
		}
	}
	_, err = r.Resolve(context.Background(), "MISSING")
	requireError(t, err, c.ErrReferenceNotFound)
	_, err = r.Resolve(context.Background(), "secret:EMPTY")
	if err == nil || errors.Is(err, c.ErrReferenceNotFound) {
		t.Fatal(err)
	}
	var nilContext context.Context
	_, err = r.Resolve(nilContext, "EMPTY")
	requireError(t, err, c.ErrNilContext)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = r.Resolve(ctx, "EMPTY")
	requireError(t, err, context.Canceled)
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"timeout": "${env:TIMEOUT}", "values": "${NOT_HERE:-1,2}"})}, Resolver: r})
	s, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var target struct {
		Timeout time.Duration
		Values  []int
	}
	if err = s.Decode("", &target); err != nil || target.Timeout != 2*time.Second || len(target.Values) != 2 {
		t.Fatal(target, err)
	}
	l, _ = c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"value": "${secret:EMPTY:-fallback}"})}, Resolver: r})
	_, err = l.Load(context.Background())
	if err == nil || errors.Is(err, c.ErrUnresolvedReference) {
		t.Fatal("unknown scheme hidden by fallback", err)
	}
}

func FuzzTextConversions(f *testing.F) {
	f.Add("1,2", uint8(0))
	f.Add("true", uint8(1))
	f.Add("256", uint8(2))
	f.Add("0.1", uint8(3))
	f.Add("0s", uint8(4))
	f.Fuzz(func(t *testing.T, raw string, kind uint8) {
		if len(raw) > 2048 {
			return
		}
		src, err := e.New(e.Config{Bindings: map[string]string{"value": "X"}}, e.Dependencies{Env: func(string) (string, bool) { return raw, true }})
		if err != nil {
			t.Fatal(err)
		}
		loader, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{src}})
		if err != nil {
			t.Fatal(err)
		}
		s, err := loader.Load(context.Background())
		if err != nil {
			return
		}
		var target any
		switch kind % 5 {
		case 0:
			target = &struct{ Value []int }{Value: []int{42}}
		case 1:
			target = &struct{ Value bool }{Value: true}
		case 2:
			target = &struct{ Value uint8 }{Value: 42}
		case 3:
			target = &struct{ Value float32 }{Value: .5}
		case 4:
			target = &struct{ Value time.Duration }{Value: time.Second}
		}
		before := fmt.Sprintf("%#v", target)
		err = s.Decode("", target)
		if err != nil && fmt.Sprintf("%#v", target) != before {
			t.Fatal("failed text conversion mutated target")
		}
	})
}
