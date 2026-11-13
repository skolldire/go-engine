package config_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	c "github.com/skolldire/go-engine/config"
	envsource "github.com/skolldire/go-engine/config/env"
)

type sourceFunc func(context.Context) (c.Document, error)

func (f sourceFunc) Load(ctx context.Context) (c.Document, error) { return f(ctx) }

type resolverFunc func(context.Context, string) (string, error)

func (f resolverFunc) Resolve(ctx context.Context, ref string) (string, error) { return f(ctx, ref) }
func source(doc c.Document) c.Source {
	return sourceFunc(func(context.Context) (c.Document, error) { return doc, nil })
}
func load(t *testing.T, sources ...c.Source) *c.Snapshot {
	t.Helper()
	l, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	s, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func requireError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}

func TestConfigAndDefaults(t *testing.T) {
	if err := c.DefaultConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*c.Config){func(v *c.Config) { v.MaxSourceBytes = 0 }, func(v *c.Config) { v.MaxDepth = 0 }, func(v *c.Config) { v.MaxExpansionDepth = -1 }, func(v *c.Config) { v.RejectUnknown = false }} {
		cfg := c.DefaultConfig()
		change(&cfg)
		_, err := c.New(cfg, c.Dependencies{})
		requireError(t, err, c.ErrInvalidConfig)
	}
	if _, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{nil}}); !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
	l, err := c.New(c.DefaultConfig(), c.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	var ctx context.Context
	_, err = l.Load(ctx)
	requireError(t, err, c.ErrNilContext)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = l.Load(canceled)
	requireError(t, err, context.Canceled)
	s, err := l.Load(context.Background())
	if err != nil || !s.Has("") {
		t.Fatal(err)
	}
}
func TestMergeDefaultsIsolationAndRevision(t *testing.T) {
	base := c.Document{"service": map[string]any{"port": 8080, "enabled": true, "names": []any{"old"}, "labels": map[string]any{"base": "yes"}, "timeout": "1s"}}
	override := c.Document{"service": map[string]any{"port": 0, "enabled": false, "names": []any{}, "timeout": nil, "labels": map[string]any{"new": "yes"}}}
	sources := []c.Source{source(base), source(override)}
	l, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	sources[0] = nil
	s, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	base["service"].(map[string]any)["port"] = 999
	type service struct {
		Port    int
		Enabled bool
		Names   []string
		Labels  map[string]string
		Timeout time.Duration
	}
	seeded := service{Port: 42, Enabled: true, Names: []string{"seed"}, Labels: map[string]string{"seed": "yes"}, Timeout: 3 * time.Second}
	if err := s.Decode("service", &seeded); err != nil {
		t.Fatal(err)
	}
	if seeded.Port != 0 || seeded.Enabled || len(seeded.Names) != 0 || seeded.Timeout != 3*time.Second || len(seeded.Labels) != 3 {
		t.Fatalf("defaults: %+v", seeded)
	}
	if s.Has("service.timeout") || s.Has("missing") || s.Has("service..port") {
		t.Fatal("Has contract")
	}
	seeded.Labels["new"] = "mutated"
	var again service
	if err := s.Decode("service", &again); err != nil || again.Labels["new"] != "yes" {
		t.Fatal(err, again)
	}
	other, err := l.Load(context.Background())
	if err != nil || s.Revision() == other.Revision() || len(s.Revision()) != 32 {
		t.Fatal("revision", err)
	}
	requireError(t, s.Decode("missing", &again), c.ErrSectionMissing)
	requireError(t, s.Decode("service..port", &again), c.ErrInvalidConfig)
}
func TestShapesDocumentsAndLimits(t *testing.T) {
	for _, values := range [][2]any{{map[string]any{}, 1}, {[]any{}, 1}, {1, map[string]any{}}, {1, []any{}}} {
		l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": values[0]}), source(c.Document{"x": values[1]})}})
		_, err := l.Load(context.Background())
		requireError(t, err, c.ErrInvalidConfig)
		if !strings.Contains(err.Error(), "source[0]") || !strings.Contains(err.Error(), "source[1]") {
			t.Fatal(err)
		}
	}
	for _, value := range []any{math.NaN(), math.Inf(1), make(chan int), json.Number("01"), float32(1), map[int]int{1: 2}} {
		l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": value})}})
		_, err := l.Load(context.Background())
		requireError(t, err, c.ErrInvalidDocument)
	}
	cyclic := map[string]any{}
	cyclic["loop"] = cyclic
	cfg := c.DefaultConfig()
	cfg.MaxDepth = 4
	for _, doc := range []c.Document{{"x": cyclic}, {"x": strings.Repeat("x", 100)}} {
		cfg.MaxSourceBytes = 30
		l, _ := c.New(cfg, c.Dependencies{Sources: []c.Source{source(doc)}})
		_, err := l.Load(context.Background())
		requireError(t, err, c.ErrLimitExceeded)
	}
	for _, limit := range []int64{5, 6} {
		cfg := c.DefaultConfig()
		cfg.MaxSourceBytes = limit
		l, _ := c.New(cfg, c.Dependencies{Sources: []c.Source{source(c.Document{"x": "abc"})}})
		_, err := l.Load(context.Background())
		if limit == 5 {
			requireError(t, err, c.ErrLimitExceeded)
		} else if err != nil {
			t.Fatal(err)
		}
	}
}
func TestDecodeTypesNamesAndAtomicity(t *testing.T) {
	type Embedded struct {
		TLSMode string
		CAFile  string
	}
	type target struct {
		Embedded     `mapstructure:",squash"`
		MaxJWKSBytes int
		Port         uint16   `json:"wrong" yaml:"wrong" mapstructure:"port"`
		Renamed      bool     `mapstructure:"renamed"`
		Ignored      chan int `mapstructure:"-"`
		Prefix       netip.Prefix
		Wait         time.Duration
		Numbers      [2]int
		Small        float32
		Large        uint64
		Data         map[string][]int
	}
	s := load(t, source(c.Document{"tls_mode": "strict", "ca_file": "ca", "max_jwks_bytes": 42, "port": 65535, "renamed": true, "prefix": "192.0.2.0/24", "wait": "0s", "numbers": []any{1, 2}, "small": json.Number("0.5"), "large": uint64(math.MaxUint64), "data": map[string]any{"x": []any{1, 2}}}))
	var got target
	if err := s.Decode("", &got); err != nil {
		t.Fatal(err)
	}
	if got.TLSMode != "strict" || got.MaxJWKSBytes != 42 || got.Port != 65535 || got.Prefix.String() != "192.0.2.0/24" || got.Numbers != [2]int{1, 2} || got.Small != .5 || got.Large != math.MaxUint64 {
		t.Fatalf("decode: %+v", got)
	}
	bad := load(t, source(c.Document{"a": map[string]any{"new": []any{3}}, "z": "wrong"}))
	seeded := struct {
		A map[string][]int
		Z int
	}{map[string][]int{"old": {1, 2}}, 9}
	original := struct {
		A map[string][]int
		Z int
	}{map[string][]int{"old": {1, 2}}, 9}
	requireError(t, bad.Decode("", &seeded), c.ErrInvalidConfig)
	if !reflect.DeepEqual(seeded, original) {
		t.Fatal("partial mutation", seeded)
	}
	for _, input := range []any{nil, got, (*target)(nil)} {
		requireError(t, s.Decode("", input), c.ErrInvalidConfig)
	}
	duplicate := struct {
		A int `mapstructure:"x"`
		B int `mapstructure:"x"`
	}{1, 2}
	requireError(t, load(t, source(c.Document{})).Decode("", &duplicate), c.ErrInvalidConfig)
	unsupported := struct{ P *int }{}
	requireError(t, load(t, source(c.Document{})).Decode("", &unsupported), c.ErrInvalidConfig)
}
func TestStrictScalars(t *testing.T) {
	cases := []struct {
		name   string
		value  any
		target any
		valid  bool
	}{
		{"json_string_int", "8080", new(int), false}, {"uint_negative", -1, new(uint), false}, {"overflow", 256, new(uint8), false}, {"decimal_integer", json.Number("1.0"), new(int), false},
		{"float32_precision", json.Number("0.1"), new(float32), false}, {"float64", json.Number("0.1"), new(float64), true}, {"float_overflow", json.Number("1e999"), new(float64), false},
		{"duration_bare", 1, new(time.Duration), false}, {"duration_zero", "0", new(time.Duration), false}, {"duration", "2ms", new(time.Duration), true},
		{"array", []any{1}, new([2]int), false}, {"null_list", []any{nil}, new([]int), false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			s := load(t, source(c.Document{"v": test.value}))
			err := s.Decode("v", test.target)
			if (err == nil) != test.valid {
				t.Fatalf("%v", err)
			}
		})
	}
	for _, raw := range []string{"42", "+1", "0x10", "1_000", "01", "false", "False"} {
		src, err := envsource.New(envsource.Config{Bindings: map[string]string{"v": "X"}}, envsource.Dependencies{Env: func(string) (string, bool) { return raw, true }})
		if err != nil {
			t.Fatal(err)
		}
		s := load(t, src)
		if raw == "false" || raw == "False" {
			var v bool
			err = s.Decode("v", &v)
			if (err == nil) != (raw == "false") {
				t.Fatal(raw, err)
			}
		} else {
			var v int
			err = s.Decode("v", &v)
			if (err == nil) != (raw == "42") {
				t.Fatal(raw, err)
			}
		}
	}
}
func TestReferencesAndConfidentiality(t *testing.T) {
	values := map[string]string{"PORT": "8080", "EMPTY": "", "A": "${B}", "B": "${A}", "NESTED": "${MISSING:-value}", "secret:db/password": "sensitive"}
	calls := map[string]int{}
	resolver := resolverFunc(func(_ context.Context, key string) (string, error) {
		calls[key]++
		v, ok := values[key]
		if !ok {
			return "", c.ErrReferenceNotFound
		}
		return v, nil
	})
	for _, test := range []struct {
		input   string
		want    string
		failure error
	}{
		{"${PORT}", "8080", nil}, {"${EMPTY:-fallback}", "", nil}, {"${MISSING:-fallback}", "fallback", nil}, {"${NESTED}", "value", nil}, {"$${PORT}", "${PORT}", nil},
		{"${A}", "", c.ErrReferenceCycle}, {"${MISSING}", "", c.ErrUnresolvedReference}, {"${PORT", "", c.ErrInvalidConfig}, {"${bad name}", "", c.ErrInvalidConfig},
	} {
		l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": test.input})}, Resolver: resolver})
		s, err := l.Load(context.Background())
		if test.failure != nil {
			requireError(t, err, test.failure)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var v string
		if err := s.Decode("x", &v); err != nil || v != test.want {
			t.Fatal(v, err)
		}
	}
	calls = map[string]int{}
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": "${PORT}", "y": "${PORT}"})}, Resolver: resolver})
	s, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ X, Y int }
	if err = s.Decode("", &got); err != nil || got.X != 8080 || calls["PORT"] != 1 {
		t.Fatal(got, calls, err)
	}
	secret := load(t, source(c.Document{"x": "sensitive"}))
	var wrong struct{ X int }
	err = secret.Decode("", &wrong)
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal(err)
	}
	injected := errors.New("external failure")
	l, _ = c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{sourceFunc(func(context.Context) (c.Document, error) { return nil, injected })}})
	_, err = l.Load(context.Background())
	requireError(t, err, injected)
	l, _ = c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": "${PORT}"})}})
	_, err = l.Load(context.Background())
	requireError(t, err, c.ErrUnresolvedReference)
}
func TestLoadSerializationCancellationAndSnapshotConcurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		started := make(chan struct{}, 1)
		var calls atomic.Int32
		l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{sourceFunc(func(ctx context.Context) (c.Document, error) {
			calls.Add(1)
			started <- struct{}{}
			select {
			case <-gate:
				return c.Document{"x": 1}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})}})
		done := make(chan error, 1)
		go func() { _, err := l.Load(context.Background()); done <- err }()
		<-started
		ctx, cancel := context.WithCancel(context.Background())
		waiting := make(chan error, 1)
		go func() { _, err := l.Load(ctx); waiting <- err }()
		synctest.Wait()
		cancel()
		requireError(t, <-waiting, context.Canceled)
		if calls.Load() != 1 {
			t.Fatal("concurrent Source invocation")
		}
		close(gate)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
	s := load(t, source(c.Document{"x": 1}))
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 20 {
				var v struct{ X int }
				if err := s.Decode("", &v); err != nil || v.X != 1 || !s.Has("x") || s.Revision() == "" {
					t.Error("concurrent snapshot access", err)
				}
			}
		})
	}
	wg.Wait()
}
func TestDepthAndExpansionBounds(t *testing.T) {
	for _, depth := range []int{1, 2, 4, 8} {
		cfg := c.DefaultConfig()
		cfg.MaxDepth = depth
		doc := c.Document{"x": 1}
		l, _ := c.New(cfg, c.Dependencies{Sources: []c.Source{source(doc)}})
		_, err := l.Load(context.Background())
		if depth == 1 {
			requireError(t, err, c.ErrLimitExceeded)
		} else if err != nil {
			t.Fatal(err)
		}
	}
	cfg := c.DefaultConfig()
	cfg.MaxExpansionDepth = 2
	resolver := resolverFunc(func(_ context.Context, ref string) (string, error) { return "${" + ref + "X}", nil })
	l, _ := c.New(cfg, c.Dependencies{Sources: []c.Source{source(c.Document{"x": "${A}"})}, Resolver: resolver})
	_, err := l.Load(context.Background())
	requireError(t, err, c.ErrLimitExceeded)
	cfg.MaxSourceBytes = 20
	l, _ = c.New(cfg, c.Dependencies{Sources: []c.Source{source(c.Document{"x": "${A}"})}, Resolver: resolverFunc(func(context.Context, string) (string, error) { return strings.Repeat("x", 100), nil })})
	_, err = l.Load(context.Background())
	requireError(t, err, c.ErrLimitExceeded)
}
func ExampleSnapshot_Decode() {
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"service": map[string]any{"port": 8080}})}})
	s, _ := l.Load(context.Background())
	cfg := struct{ Port int }{Port: 80}
	_ = s.Decode("service", &cfg)
	fmt.Println(cfg.Port)
	// Output: 8080
}

type mutatingText struct{ Values []string }

func (m *mutatingText) UnmarshalText([]byte) error {
	if len(m.Values) > 0 {
		m.Values[0] = "mutated"
	}
	m.Values = append(m.Values, "new")
	return errors.New("private input")
}
func TestTextUnmarshalerDoesNotMutateSeedOnFailure(t *testing.T) {
	s := load(t, source(c.Document{"x": "secret"}))
	target := struct{ X mutatingText }{mutatingText{[]string{"original"}}}
	err := s.Decode("", &target)
	if !errors.Is(err, c.ErrInvalidConfig) || target.X.Values[0] != "original" || len(target.X.Values) != 1 || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private input") {
		t.Fatal(target, err)
	}
}
func TestExactFloat32ExponentNormalization(t *testing.T) {
	for _, test := range []struct {
		raw   string
		valid bool
	}{{"0e-99999999", true}, {"1e-99999999", false}, {"0.500000000000000000000000000", true}, {"100000000000000000000000000000000000000000000000000000e-53", true}, {"1.0000000000000000000000000000000000000000000000000000001", false}} {
		s := load(t, source(c.Document{"v": json.Number(test.raw)}))
		var v float32
		err := s.Decode("v", &v)
		if (err == nil) != test.valid {
			t.Fatal(test.raw, err)
		}
	}
}

type textList []string

func (v *textList) UnmarshalText(input []byte) error {
	*v = strings.Split(string(input), ",")
	return nil
}
func TestSupportedTypeMatrixAndUnsupportedContainers(t *testing.T) {
	type allTypes struct {
		I       int
		I8      int8
		I16     int16
		I32     int32
		I64     int64
		U       uint
		U8      uint8
		U16     uint16
		U32     uint32
		U64     uint64
		PtrSize uintptr
		F32     float32
		F64     float64
		Text    textList
		Rows    []map[string]int
		Maps    map[string]struct{ A int }
	}
	doc := c.Document{"i": int(-1), "i8": int8(-2), "i16": int16(-3), "i32": int32(-4), "i64": int64(-5), "u": uint(1), "u8": uint8(2), "u16": uint16(3), "u32": uint32(4), "u64": uint64(5), "ptr_size": uintptr(6), "f32": float64(.5), "f64": float64(.1), "text": "a,b", "rows": []any{map[string]any{"a": 1}}, "maps": map[string]any{"row": map[string]any{"a": 2}}}
	var got allTypes
	if err := load(t, source(doc)).Decode("", &got); err != nil || got.I64 != -5 || got.U64 != 5 || got.PtrSize != 6 || got.Rows[0]["a"] != 1 || got.Maps["row"].A != 2 || len(got.Text) != 2 {
		t.Fatal(got, err)
	}
	for _, target := range []any{&map[int]string{}, &[]any{}, &map[string]chan int{}, &struct{ F func() }{}, &[]map[string]any{}, new(complex128)} {
		requireError(t, load(t, source(c.Document{})).Decode("", target), c.ErrInvalidConfig)
	}
}
func TestReferenceGrammarAndResolverErrors(t *testing.T) {
	for _, ref := range []string{"${:name}", "${HTTP:abc}", "${secret:}", "${bad~:name}", "${a:b:c}"} {
		l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": ref})}, Resolver: resolverFunc(func(context.Context, string) (string, error) {
			t.Error("invalid reference reached resolver")
			return "", nil
		})})
		_, err := l.Load(context.Background())
		requireError(t, err, c.ErrInvalidConfig)
	}
	failure := errors.New("injected resolver error")
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": "${secret:db/password}"})}, Resolver: resolverFunc(func(_ context.Context, name string) (string, error) {
		if name != "secret:db/password" {
			t.Fatal(name)
		}
		return "", failure
	})})
	s, err := l.Load(context.Background())
	if s != nil {
		t.Fatal("partial snapshot")
	}
	requireError(t, err, failure)
}
