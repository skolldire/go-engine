package viper_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	c "github.com/skolldire/go-engine/config"
	e "github.com/skolldire/go-engine/config/env"
	j "github.com/skolldire/go-engine/config/json"
	v "github.com/skolldire/go-engine/config/viper"
)

func load(t *testing.T, files fstest.MapFS, names ...string) *c.Snapshot {
	t.Helper()
	cfg := v.DefaultConfig()
	cfg.Files = names
	src, err := v.New(cfg, v.Dependencies{FS: files})
	if err != nil {
		t.Fatal(err)
	}
	loader, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{src}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func TestFormatsAndMerge(t *testing.T) {
	for extension, input := range map[string]string{"yaml": "app:\n  port: 8080\n  timeout: 2s\n", "yml": "app:\n  port: 8080\n  timeout: 2s\n", "json": `{"app":{"port":8080,"timeout":"2s"}}`, "toml": "[app]\nport=8080\ntimeout='2s'"} {
		t.Run(extension, func(t *testing.T) {
			path := "app." + extension
			s := load(t, fstest.MapFS{path: {Data: []byte(input)}}, path)
			var cfg struct {
				Port    int
				Timeout time.Duration
			}
			if err := s.Decode("app", &cfg); err != nil || cfg.Port != 8080 || cfg.Timeout != 2*time.Second {
				t.Fatal(cfg, err)
			}
		})
	}
	files := fstest.MapFS{"base.yaml": {Data: []byte("app:\n  port: 80\n  timeout: 2s\n  hosts: [a,b]\n")}, "override.yaml": {Data: []byte("app:\n  port: 8080\n  hosts: [c]\n")}}
	var cfg struct {
		Port    int
		Timeout time.Duration
		Hosts   []string
	}
	if err := load(t, files, "base.yaml", "override.yaml").Decode("app", &cfg); err != nil || cfg.Port != 8080 || cfg.Timeout != 2*time.Second || !reflect.DeepEqual(cfg.Hosts, []string{"c"}) {
		t.Fatal(cfg, err)
	}
}
func TestViperLowercasesDataKeys(t *testing.T) {
	s := load(t, fstest.MapFS{"app.yaml": {Data: []byte("hosts:\n  EXAMPLE.COM: yes\nrows:\n  - MiXeD: value\n")}}, "app.yaml")
	var cfg struct {
		Hosts map[string]map[string]string
		Rows  []map[string]string
	}
	if err := s.Decode("", &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Hosts["example"]["com"] != "yes" {
		t.Fatal(cfg)
	}
	if cfg.Rows[0]["mixed"] != "value" {
		t.Fatal(cfg)
	}
}
func TestViperMissingLimitsSyntaxAndConfig(t *testing.T) {
	for _, required := range []bool{true, false} {
		cfg := v.DefaultConfig()
		cfg.Files = []string{"missing.yaml"}
		cfg.Required = required
		src, err := v.New(cfg, v.Dependencies{FS: fstest.MapFS{}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = src.Load(context.Background())
		if required && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		if !required && err != nil {
			t.Fatal(err)
		}
	}
	cfg := v.DefaultConfig()
	cfg.Files = []string{"app.yaml"}
	cfg.MaxBytes = 4
	src, _ := v.New(cfg, v.Dependencies{FS: fstest.MapFS{"app.yaml": {Data: []byte("x: 1\n")}}})
	_, err := src.Load(context.Background())
	if !errors.Is(err, c.ErrLimitExceeded) {
		t.Fatal(err)
	}
	cfg.MaxBytes = 5
	src, _ = v.New(cfg, v.Dependencies{FS: fstest.MapFS{"app.yaml": {Data: []byte("x: 1\n")}}})
	if _, err := src.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg.MaxBytes = 1024
	for _, input := range []string{"password: [SUPER_SECRET", "password: \"SUPER_SECRET"} {
		src, _ = v.New(cfg, v.Dependencies{FS: fstest.MapFS{"app.yaml": {Data: []byte(input)}}})
		_, err = src.Load(context.Background())
		if !errors.Is(err, c.ErrInvalidConfig) || strings.Contains(err.Error(), "SUPER_SECRET") || !strings.Contains(err.Error(), "app.yaml") {
			t.Fatal(err)
		}
	}
	for _, files := range [][]string{nil, {"../app.yaml"}, {"app.ini"}, {"app"}} {
		cfg := v.DefaultConfig()
		cfg.Files = files
		if _, err := v.New(cfg, v.Dependencies{FS: fstest.MapFS{}}); !errors.Is(err, c.ErrInvalidConfig) {
			t.Fatal(files, err)
		}
	}
	cfg = v.DefaultConfig()
	cfg.Files = []string{"app.yaml"}
	if _, err := v.New(cfg, v.Dependencies{}); !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
	var nilContext context.Context
	_, err = src.Load(nilContext)
	if !errors.Is(err, c.ErrNilContext) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = src.Load(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestCompositionAndBorrowedFS(t *testing.T) {
	files := []string{"app.yaml", "prod.yaml"}
	cfg := v.DefaultConfig()
	cfg.Files = files
	source, err := v.New(cfg, v.Dependencies{FS: fstest.MapFS{"app.yaml": {Data: []byte("app:\n  port: 80\n  timeout: ${TIMEOUT}\n  name: prefix-${NAME}\n")}, "prod.yaml": {Data: []byte("app:\n  port: 8080\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	files[0] = "mutated"
	captured := map[string]string{"PORT": "0", "TIMEOUT": "3s", "NAME": ""}
	lookup := func(name string) (string, bool) { value, ok := captured[name]; return value, ok }
	resolver, _ := e.Resolver(lookup)
	env, _ := e.New(e.Config{Bindings: map[string]string{"app.port": "PORT"}}, e.Dependencies{Env: lookup})
	loader, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source, env}, Resolver: resolver})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				snapshot, err := loader.Load(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				target := struct {
					Port    int
					Timeout time.Duration
					Name    string
					Default int
				}{Default: 42}
				if err := snapshot.Decode("app", &target); err != nil || target.Port != 0 || target.Timeout != 3*time.Second || target.Name != "prefix-" || target.Default != 42 {
					t.Error(target, err)
					return
				}
			}
		})
	}
	wg.Wait()
	if fmt.Sprint(source) != "viper:app.yaml,prod.yaml" {
		t.Fatal(source)
	}
}

func TestNativeJSONPrecisionVersusExactSource(t *testing.T) {
	raw := `{"number":9007199254740993}`
	s := load(t, fstest.MapFS{"data.json": {Data: []byte(raw)}}, "data.json")
	var got struct{ Number float64 }
	if err := s.Decode("", &got); err != nil || got.Number != 9007199254740992 {
		t.Fatal(got, err)
	}
	cfg := j.DefaultConfig()
	cfg.Path = "data.json"
	src, err := j.New(cfg, j.Dependencies{FS: fstest.MapFS{"data.json": {Data: []byte(raw)}}})
	if err != nil {
		t.Fatal(err)
	}
	loader, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{src}})
	exact, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var exactNumber struct{ Number int64 }
	if err := exact.Decode("", &exactNumber); err != nil || exactNumber.Number != 9007199254740993 {
		t.Fatal(exactNumber, err)
	}
	literalCfg := j.DefaultConfig()
	literalCfg.Path = "literal.json"
	literal, _ := j.New(literalCfg, j.Dependencies{FS: fstest.MapFS{"literal.json": {Data: []byte(`{"hosts":{"EXAMPLE.COM":"value"}}`)}}})
	loader, _ = c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{literal}})
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var hosts struct{ Hosts map[string]string }
	if err := snapshot.Decode("", &hosts); err != nil || hosts.Hosts["EXAMPLE.COM"] != "value" {
		t.Fatal(hosts, err)
	}
}

type trackedFS struct {
	fs.FS
	opened, closed        atomic.Int32
	openError, closeError error
	afterRead             func()
}

func (f *trackedFS) Open(name string) (fs.File, error) {
	if f.openError != nil {
		return nil, f.openError
	}
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	f.opened.Add(1)
	return &trackedFile{File: file, owner: f}, nil
}

type trackedFile struct {
	fs.File
	owner *trackedFS
}

func (f *trackedFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if f.owner.afterRead != nil {
		f.owner.afterRead()
	}
	return n, err
}
func (f *trackedFile) Close() error {
	f.owner.closed.Add(1)
	err := f.File.Close()
	return errors.Join(err, f.owner.closeError)
}
func TestReaderOwnershipAndCancellation(t *testing.T) {
	files := &trackedFS{FS: fstest.MapFS{"app.yaml": {Data: []byte("value: ok\n")}}}
	cfg := v.DefaultConfig()
	cfg.Files = []string{"app.yaml"}
	source, err := v.New(cfg, v.Dependencies{FS: files})
	if err != nil {
		t.Fatal(err)
	}
	if files.opened.Load() != 0 {
		t.Fatal("New performed I/O")
	}
	for range 3 {
		if _, err := source.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if files.opened.Load() != 3 || files.closed.Load() != 3 {
		t.Fatal(files.opened.Load(), files.closed.Load())
	}
	closeFailure := errors.New("injected close failure")
	files.closeError = closeFailure
	doc, err := source.Load(context.Background())
	if !errors.Is(err, closeFailure) || doc != nil || files.closed.Load() != 4 {
		t.Fatal(doc, err)
	}
	files.closeError = nil
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	files.afterRead = cancel
	doc, err = source.Load(ctx)
	if !errors.Is(err, context.Canceled) || doc != nil || files.closed.Load() != 5 {
		t.Fatal(doc, err)
	}
	files.afterRead = nil
	files.openError = fs.ErrPermission
	cfg.Required = false
	source, err = v.New(cfg, v.Dependencies{FS: files})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.Load(context.Background())
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
}
