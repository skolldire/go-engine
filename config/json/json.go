package json

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"unicode/utf8"

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
func New(cfg Config, deps Dependencies) (config.Source, error) {
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
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) {
		return nil, &config.ConfigError{Field: s.cfg.Path, Reason: "nil reader from " + s.String()}
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			document = nil
			err = errors.Join(err, fmt.Errorf("close %s: %w", s, closeErr))
		}
	}()
	bounded := &boundedReader{ctx: ctx, reader: reader, remaining: s.cfg.MaxBytes}
	raw, readErr := io.ReadAll(bounded)
	if readErr != nil {
		return nil, fmt.Errorf("read %s: %w", s, readErr)
	}
	if !strictUnicode(raw) {
		return nil, &config.ConfigError{Field: s.cfg.Path, Reason: "invalid JSON Unicode from " + s.String()}
	}
	decoder := stdjson.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	document, err = parse(decoder)
	if err != nil {
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

func strictUnicode(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		high, ok := hexUnit(raw, i+1)
		if !ok {
			return false
		}
		i += 4
		if high >= 0xdc00 && high <= 0xdfff {
			return false
		}
		if high >= 0xd800 && high <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, ok := hexUnit(raw, i+3)
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
func hexUnit(raw []byte, start int) (uint16, bool) {
	if start+4 > len(raw) {
		return 0, false
	}
	var unit uint16
	for _, b := range raw[start : start+4] {
		unit <<= 4
		switch {
		case b >= '0' && b <= '9':
			unit |= uint16(b - '0')
		case b >= 'a' && b <= 'f':
			unit |= uint16(b - 'a' + 10)
		case b >= 'A' && b <= 'F':
			unit |= uint16(b - 'A' + 10)
		default:
			return 0, false
		}
	}
	return unit, true
}
