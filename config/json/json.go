package json

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"

	"github.com/skolldire/go-engine/config"
)

type Config struct {
	Path     string
	MaxBytes int64
	Required bool
}
type Dependencies struct {
	FS     fs.FS
	Reader func(context.Context) (io.ReadCloser, error)
}
type Source struct {
	cfg  Config
	deps Dependencies
}

func DefaultConfig() Config { return Config{MaxBytes: 1 << 20, Required: true} }
func (c Config) Validate() error {
	if c.Path == "" {
		return &config.ConfigError{Field: "path", Reason: "must be nonempty"}
	}
	if c.MaxBytes <= 0 {
		return &config.ConfigError{Field: "max_bytes", Reason: "must be positive"}
	}
	return nil
}
func New(cfg Config, deps Dependencies) (*Source, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.FS != nil {
		v := reflect.ValueOf(deps.FS)
		if v.Kind() == reflect.Pointer && v.IsNil() {
			return nil, &config.ConfigError{Field: "fs", Reason: "nil filesystem"}
		}
	}
	if (deps.FS == nil) == (deps.Reader == nil) {
		return nil, &config.ConfigError{Field: "dependencies", Reason: "exactly one FS or Reader required"}
	}
	if deps.FS != nil && !fs.ValidPath(cfg.Path) {
		return nil, &config.ConfigError{Field: "path", Reason: "invalid filesystem path"}
	}
	return &Source{cfg: cfg, deps: deps}, nil
}
func (s *Source) String() string {
	if s == nil {
		return "json"
	}
	return "json:" + s.cfg.Path
}
func (s *Source) Load(ctx context.Context) (document config.Document, err error) {
	if ctx == nil {
		return nil, config.ErrNilContext
	}
	if s == nil || s.cfg.MaxBytes <= 0 || (s.deps.FS == nil && s.deps.Reader == nil) {
		return nil, &config.ConfigError{Field: "json", Reason: "source must be initialized"}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var reader io.ReadCloser
	if s.deps.Reader != nil {
		reader, err = s.deps.Reader(ctx)
	} else {
		reader, err = s.deps.FS.Open(s.cfg.Path)
	}
	if err != nil {
		if !s.cfg.Required && errors.Is(err, fs.ErrNotExist) {
			return config.Document{}, nil
		}
		return nil, fmt.Errorf("open %s: %w", s, err)
	}
	if reader == nil {
		return nil, &config.ConfigError{Field: s.cfg.Path, Reason: "nil reader from " + s.String()}
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			document = nil
			err = errors.Join(err, fmt.Errorf("close %s: %w", s, closeErr))
		}
	}()
	bounded := &boundedReader{ctx: ctx, reader: reader, remaining: s.cfg.MaxBytes}
	decoder := stdjson.NewDecoder(bounded)
	decoder.UseNumber()
	document, err = parse(decoder)
	if err != nil {
		if bounded.err != nil {
			return nil, fmt.Errorf("read %s: %w", s, bounded.err)
		}
		var ce *config.ConfigError
		if errors.As(err, &ce) {
			field := ce.Field
			if field == "" {
				field = s.cfg.Path
			}
			return nil, &config.ConfigError{Field: field, Reason: ce.Reason + " from " + s.String()}
		}
		return nil, &config.ConfigError{Field: s.cfg.Path, Reason: "invalid JSON from " + s.String()}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return document, nil
}

type boundedReader struct {
	ctx       context.Context
	reader    io.Reader
	remaining int64
	err       error
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		r.err = err
		return 0, err
	}
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		if n > 0 {
			r.err = config.ErrLimitExceeded
			return 0, r.err
		}
		if err != nil && err != io.EOF {
			r.err = err
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}

type frame struct {
	object    map[string]any
	list      []any
	key       string
	expectKey bool
	isObject  bool
}

func parse(decoder *stdjson.Decoder) (config.Document, error) {
	var stack []frame
	var root any
	rootSet := false
	appendValue := func(value any) error {
		if len(stack) == 0 {
			if rootSet {
				return &config.ConfigError{Reason: "multiple JSON values"}
			}
			root = value
			rootSet = true
			return nil
		}
		parent := &stack[len(stack)-1]
		if parent.isObject {
			parent.object[parent.key] = value
			parent.expectKey = true
		} else {
			parent.list = append(parent.list, value)
		}
		return nil
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if len(stack) != 0 || !rootSet {
				return nil, io.ErrUnexpectedEOF
			}
			object, ok := root.(map[string]any)
			if !ok {
				return nil, &config.ConfigError{Reason: "JSON root must be object"}
			}
			return config.Document(object), nil
		}
		if err != nil {
			return nil, err
		}
		if delimiter, ok := token.(stdjson.Delim); ok {
			switch delimiter {
			case '{':
				stack = append(stack, frame{object: map[string]any{}, expectKey: true, isObject: true})
			case '[':
				stack = append(stack, frame{list: []any{}})
			case '}', ']':
				if len(stack) == 0 {
					return nil, &config.ConfigError{Reason: "unexpected JSON closure"}
				}
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				var value any = top.list
				if top.isObject {
					value = top.object
				}
				if err := appendValue(value); err != nil {
					return nil, err
				}
			}
			continue
		}
		if len(stack) > 0 && stack[len(stack)-1].isObject && stack[len(stack)-1].expectKey {
			parent := &stack[len(stack)-1]
			key, ok := token.(string)
			if !ok {
				return nil, &config.ConfigError{Reason: "invalid JSON object key"}
			}
			if _, exists := parent.object[key]; exists {
				return nil, &config.ConfigError{Reason: "duplicate JSON object key"}
			}
			parent.key = key
			parent.expectKey = false
		} else if err := appendValue(token); err != nil {
			return nil, err
		}
	}
}
