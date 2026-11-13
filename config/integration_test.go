package config_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	c "github.com/skolldire/go-engine/config"
	e "github.com/skolldire/go-engine/config/env"
	j "github.com/skolldire/go-engine/config/json"
)

func TestIntegrationSourcesResolutionAndDefaults(t *testing.T) {
	files := fstest.MapFS{"base": {Data: []byte(`{"app":{"port":80,"enabled":true,"token":"${TOKEN}"}}`)}, "override": {Data: []byte(`{"app":{"port":8080,"enabled":false}}`)}}
	var sources []c.Source
	for _, path := range []string{"base", "override"} {
		cfg := j.DefaultConfig()
		cfg.Path = path
		src, err := j.New(cfg, j.Dependencies{FS: files})
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, src)
	}
	captured := map[string]string{"PORT": "0", "TOKEN": ""}
	lookup := func(name string) (string, bool) { v, ok := captured[name]; return v, ok }
	env, err := e.New(e.Config{Bindings: map[string]string{"app.port": "PORT"}}, e.Dependencies{Env: lookup})
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, env)
	l, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: sources, Resolver: resolverFunc(func(_ context.Context, name string) (string, error) {
		v, ok := lookup(name)
		if !ok {
			return "", c.ErrReferenceNotFound
		}
		return v, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	s, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := struct {
		Port    int
		Enabled bool
		Token   string
		Missing int
	}{1, true, "seed", 42}
	if err := s.Decode("app", &target); err != nil || target.Port != 0 || target.Enabled || target.Token != "" || target.Missing != 42 {
		t.Fatal(target, err)
	}
}

type cancelingReader struct {
	cancel context.CancelFunc
	read   bool
}

func (r *cancelingReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		p[0] = '{'
		r.cancel()
		return 1, nil
	}
	return 0, io.EOF
}
func TestIntegrationJSONCooperativeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := 0
	cfg := j.DefaultConfig()
	cfg.Path = "cancel"
	src, err := j.New(cfg, j.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) {
		return &countedReader{Reader: &cancelingReader{cancel: cancel}, closed: &closed}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Load(ctx)
	if !errors.Is(err, context.Canceled) || closed != 1 {
		t.Fatal(err, closed)
	}
	opened := 0
	closed = 0
	src, err = j.New(cfg, j.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) {
		opened++
		return &countedReader{Reader: strings.NewReader(`{}`), closed: &closed}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := src.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if opened != 3 || closed != 3 {
		t.Fatal(opened, closed)
	}
}

type countedReader struct {
	io.Reader
	closed *int
}

func (r *countedReader) Close() error { *r.closed++; return nil }
