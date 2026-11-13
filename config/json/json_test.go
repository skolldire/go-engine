package json_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	c "github.com/skolldire/go-engine/config"
	j "github.com/skolldire/go-engine/config/json"
)

func TestStrictJSONAndRawLimits(t *testing.T) {
	cases := []struct {
		name, input string
		valid       bool
	}{
		{"valid", `{"x":9007199254740993}`, true}, {"empty", `{}`, true}, {"nested", `{"x":[{"a":1},[],{}]}`, true},
		{"duplicate", `{"password":"secret","password":"other"}`, false}, {"nested_duplicate", `{"x":{"a":1,"a":2}}`, false}, {"escaped_duplicate", `{"a":1,"\u0061":2}`, false},
		{"extra", `{} {}`, false}, {"extra_array", `{} []`, false}, {"extra_scalar", `{} 1`, false}, {"truncated", `{"x":`, false}, {"root_list", `[]`, false}, {"root_null", `null`, false},
		{"invalid_utf8", "{\"x\":\"\xff\"}", false},
		{"unpaired_surrogate", `{"x":"\ud800"}`, false},
		{"surrogate_pair", `{"x":"\ud83d\ude00"}`, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cfg := j.DefaultConfig()
			cfg.Path = "app.json"
			src, err := j.New(cfg, j.Dependencies{FS: fstest.MapFS{"app.json": {Data: []byte(test.input)}}})
			if err != nil {
				t.Fatal(err)
			}
			document, err := src.Load(context.Background())
			if (err == nil) != test.valid {
				t.Fatal(err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("value leaked", err)
			}
			if test.name == "valid" {
				l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{src}})
				s, err := l.Load(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				var got struct{ X int64 }
				if err = s.Decode("", &got); err != nil || got.X != 9007199254740993 {
					t.Fatal(document, got, err)
				}
			}
		})
	}
	for _, limit := range []int64{1, 2, 3} {
		cfg := j.DefaultConfig()
		cfg.Path = "app"
		cfg.MaxBytes = limit
		src, _ := j.New(cfg, j.Dependencies{FS: fstest.MapFS{"app": {Data: []byte("{} ")}}})
		_, err := src.Load(context.Background())
		if limit < 3 && !errors.Is(err, c.ErrLimitExceeded) {
			t.Fatal(limit, err)
		}
		if limit == 3 && err != nil {
			t.Fatal(err)
		}
	}
}

type reader struct {
	io.Reader
	closes  int
	failure error
}

func (r *reader) Close() error { r.closes++; return r.failure }

type brokenReader struct{ failure error }

func (r brokenReader) Read([]byte) (int, error) { return 0, r.failure }
func TestReaderOwnershipAndInjectedErrors(t *testing.T) {
	injected := errors.New("external I/O failure")
	for _, input := range []string{`{}`, `{"bad":`} {
		r := &reader{Reader: strings.NewReader(input)}
		cfg := j.DefaultConfig()
		cfg.Path = "provided"
		src, _ := j.New(cfg, j.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return r, nil }})
		_, _ = src.Load(context.Background())
		if r.closes != 1 {
			t.Fatal(r.closes)
		}
	}
	for _, closeFailure := range []bool{false, true} {
		r := &reader{Reader: brokenReader{injected}}
		if closeFailure {
			r.Reader = strings.NewReader("{}")
			r.failure = injected
		}
		cfg := j.DefaultConfig()
		cfg.Path = "injected"
		src, _ := j.New(cfg, j.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return r, nil }})
		_, err := src.Load(context.Background())
		if !errors.Is(err, injected) || r.closes != 1 {
			t.Fatal(err, r.closes)
		}
	}
	cfg := j.DefaultConfig()
	cfg.Path = "missing"
	cfg.Required = false
	src, _ := j.New(cfg, j.Dependencies{FS: fstest.MapFS{}})
	if _, err := src.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	src, _ = j.New(cfg, j.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return nil, fs.ErrPermission }})
	_, err := src.Load(context.Background())
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	src, _ = j.New(cfg, j.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return nil, nil }})
	_, err = src.Load(context.Background())
	if !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
	var nilCtx context.Context
	_, err = src.Load(nilCtx)
	if !errors.Is(err, c.ErrNilContext) {
		t.Fatal(err)
	}
}
func TestJSONConstruction(t *testing.T) {
	cfg := j.DefaultConfig()
	cfg.Path = "../escape"
	if _, err := j.New(cfg, j.Dependencies{FS: fstest.MapFS{}}); !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
	cfg.Path = "app"
	for _, deps := range []j.Dependencies{{}, {FS: fstest.MapFS{}, Reader: func(context.Context) (io.ReadCloser, error) { return nil, nil }}} {
		if _, err := j.New(cfg, deps); !errors.Is(err, c.ErrInvalidConfig) {
			t.Fatal(err)
		}
	}
	cfg.MaxBytes = 0
	if _, err := j.New(cfg, j.Dependencies{FS: fstest.MapFS{}}); !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
}

func TestTypedNilReaderRejected(t *testing.T) {
	cfg := j.DefaultConfig()
	cfg.Path = "nil"
	src, err := j.New(cfg, j.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { var value *reader; return value, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Load(context.Background()); !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
}
