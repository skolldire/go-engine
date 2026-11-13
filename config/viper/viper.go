package viper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"strings"

	"github.com/skolldire/go-engine/config"
	sdk "github.com/spf13/viper"
)

type Config struct {
	Files    []string
	MaxBytes int64
	Required bool
}
type Dependencies struct{ FS fs.FS }
type source struct {
	cfg Config
	fs  fs.FS
}

func DefaultConfig() Config { return Config{MaxBytes: 1 << 20, Required: true} }
func (c Config) Validate() error {
	if len(c.Files) == 0 {
		return &config.ConfigError{Field: "files", Reason: "must be nonempty"}
	}
	if c.MaxBytes <= 0 {
		return &config.ConfigError{Field: "max_bytes", Reason: "must be positive"}
	}
	for _, file := range c.Files {
		if !fs.ValidPath(file) {
			return &config.ConfigError{Field: "files", Reason: "invalid filesystem path"}
		}
		switch strings.ToLower(path.Ext(file)) {
		case ".yaml", ".yml", ".json", ".toml":
		default:
			return &config.ConfigError{Field: file, Reason: "unsupported configuration extension"}
		}
	}
	return nil
}
func New(cfg Config, deps Dependencies) (config.Source, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.FS == nil {
		return nil, &config.ConfigError{Field: "fs", Reason: "filesystem is required"}
	}
	value := reflect.ValueOf(deps.FS)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, &config.ConfigError{Field: "fs", Reason: "nil filesystem"}
	}
	cfg.Files = append([]string(nil), cfg.Files...)
	return &source{cfg: cfg, fs: deps.FS}, nil
}
func (s *source) String() string { return "viper:" + strings.Join(s.cfg.Files, ",") }
func (s *source) Load(ctx context.Context) (config.Document, error) {
	if ctx == nil {
		return nil, config.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	merged := sdk.New()
	for _, file := range s.cfg.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := s.read(ctx, file)
		if err != nil {
			if !s.cfg.Required && errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		current := sdk.New()
		format := strings.TrimPrefix(strings.ToLower(path.Ext(file)), ".")
		if format == "yml" {
			format = "yaml"
		}
		current.SetConfigType(format)
		if err := current.ReadConfig(bytes.NewReader(raw)); err != nil {
			return nil, &config.ConfigError{Field: file, Reason: "invalid " + format + " from viper:" + file}
		}
		if err := merged.MergeConfigMap(current.AllSettings()); err != nil {
			return nil, &config.ConfigError{Field: file, Reason: "cannot merge viper:" + file}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return config.Document(merged.AllSettings()), nil
}
func (s *source) read(ctx context.Context, file string) (raw []byte, err error) {
	reader, err := s.fs.Open(file)
	if err != nil {
		return nil, fmt.Errorf("open viper:%s: %w", file, err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			raw = nil
			err = errors.Join(err, fmt.Errorf("close viper:%s: %w", file, closeErr))
		}
	}()
	input := &boundedReader{ctx: ctx, reader: reader, remaining: s.cfg.MaxBytes}
	raw, err = io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("read viper:%s: %w", file, err)
	}
	return raw, nil
}

type boundedReader struct {
	ctx       context.Context
	reader    io.Reader
	remaining int64
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		if n > 0 {
			return 0, config.ErrLimitExceeded
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}
