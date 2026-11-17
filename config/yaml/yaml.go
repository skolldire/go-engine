// Package yaml is a strict YAML source for config.
//
// It reads one document into the same value domain config/json produces:
// string-keyed maps, lists, strings, booleans, nulls and exact decimal
// numbers. Everything YAML allows beyond that (custom tags, non-string
// keys, duplicate keys, several documents, non-finite numbers, unbounded
// alias expansion) is an error naming the line, the column and the key
// path, never the value.
package yaml

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	yamlv3 "go.yaml.in/yaml/v3"

	"github.com/skolldire/go-engine/config"
)

// maxExpandedNodes bounds the document after aliases and merge keys are
// expanded. A few lines of nested aliases can describe billions of nodes;
// counting what expansion produces stops that whatever shape it takes. It
// is part of the contract and documented in the README.
const maxExpandedNodes = 100_000

type Config struct {
	// Path is opened in FS; with a Reader it only names the source.
	Path string `mapstructure:"path" json:"path"`
	// MaxBytes bounds the raw file, enforced while reading.
	MaxBytes int64 `mapstructure:"max_bytes" json:"max_bytes"`
	// Required makes a missing file an error. When false, only
	// fs.ErrNotExist is tolerated.
	Required bool `mapstructure:"required" json:"required"`
}

// Dependencies takes exactly one of FS or Reader.
type Dependencies struct {
	FS     fs.FS
	Reader func(context.Context) (io.ReadCloser, error)
}

// Source implements config.Source and fmt.Stringer.
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

// New validates and stores; it performs no I/O.
func New(cfg Config, deps Dependencies) (config.Source, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.FS != nil {
		if v := reflect.ValueOf(deps.FS); v.Kind() == reflect.Pointer && v.IsNil() {
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
		return "yaml"
	}
	return "yaml:" + s.cfg.Path
}

func (s *Source) Load(ctx context.Context) (document config.Document, err error) {
	if ctx == nil {
		return nil, config.ErrNilContext
	}
	if s == nil || s.cfg.MaxBytes <= 0 || (s.deps.FS == nil && s.deps.Reader == nil) {
		return nil, &config.ConfigError{Field: "yaml", Reason: "source must be initialized"}
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
	raw, readErr := io.ReadAll(&boundedReader{ctx: ctx, reader: reader, remaining: s.cfg.MaxBytes})
	if readErr != nil {
		return nil, fmt.Errorf("read %s: %w", s, readErr)
	}
	document, failure := parse(raw)
	if failure != nil {
		return nil, failure.describe(s)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return document, nil
}

// boundedReader stops at the limit instead of reading a file of any size
// and measuring it afterwards.
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

// failure is a parse error before it is attached to a source. It holds a
// position and a key path, and by construction no scalar value.
type failure struct {
	path         string
	line, column int
	reason       string
	limit        bool
}

func (f *failure) describe(s *Source) error {
	field := f.path
	if field == "" {
		field = s.cfg.Path
	}
	reason := f.reason
	if f.line > 0 {
		reason += " at line " + strconv.Itoa(f.line)
		if f.column > 0 {
			reason += " column " + strconv.Itoa(f.column)
		}
	}
	reason += " from " + s.String()
	if f.limit {
		return fmt.Errorf("%s: %s: %w", field, reason, config.ErrLimitExceeded)
	}
	return &config.ConfigError{Field: field, Reason: reason}
}

func at(n *yamlv3.Node, path, reason string) *failure {
	return &failure{path: path, line: n.Line, column: n.Column, reason: reason}
}

func parse(raw []byte) (document config.Document, fail *failure) {
	// The library also accepts UTF-16; a configuration file in anything
	// but UTF-8 is far more likely a mistake than a choice.
	if !utf8.Valid(raw) {
		return nil, &failure{reason: "invalid YAML Unicode"}
	}
	// The library reports its own errors as values, but it is a parser of
	// untrusted input: an unexpected panic inside it must not take down
	// the process that was only loading its configuration.
	defer func() {
		if recover() != nil {
			document, fail = nil, &failure{reason: "invalid YAML"}
		}
	}()
	decoder := yamlv3.NewDecoder(bytes.NewReader(raw))
	var first yamlv3.Node
	if err := decoder.Decode(&first); err != nil {
		if errors.Is(err, io.EOF) {
			return config.Document{}, nil
		}
		return nil, syntax(err)
	}
	var second yamlv3.Node
	switch err := decoder.Decode(&second); {
	case err == nil:
		return nil, &failure{line: second.Line, column: second.Column, reason: "multiple YAML documents"}
	case !errors.Is(err, io.EOF):
		return nil, syntax(err)
	}
	root := &first
	if root.Kind == yamlv3.DocumentNode {
		if len(root.Content) != 1 {
			return config.Document{}, nil
		}
		root = root.Content[0]
	}
	c := &converter{active: map[*yamlv3.Node]bool{}}
	// A document holding only null ("---", "~", or comments around
	// nothing) carries no settings, like an empty file.
	if root.Kind == yamlv3.ScalarNode && root.ShortTag() == "!!null" && root.Style&yamlv3.TaggedStyle == 0 {
		return config.Document{}, nil
	}
	if root.Kind != yamlv3.MappingNode {
		return nil, at(root, "", "YAML root must be a mapping")
	}
	object, fail := c.mapping(root, "")
	if fail != nil {
		return nil, fail
	}
	return config.Document(object), nil
}

// syntax keeps the line the library reports and drops its message, which
// can quote the offending text.
func syntax(err error) *failure {
	f := &failure{reason: "invalid YAML"}
	text := err.Error()
	if i := strings.Index(text, "line "); i >= 0 {
		digits := text[i+len("line "):]
		end := 0
		for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
			end++
		}
		if n, convErr := strconv.Atoi(digits[:end]); convErr == nil {
			f.line = n
		}
	}
	return f
}

type converter struct {
	nodes int
	// active holds the collections being converted, which is what makes an
	// alias that points back into its own anchor detectable.
	active map[*yamlv3.Node]bool
}

func (c *converter) count(n *yamlv3.Node, path string) *failure {
	c.nodes++
	if c.nodes > maxExpandedNodes {
		f := at(n, path, "YAML expands to more than "+strconv.Itoa(maxExpandedNodes)+" nodes")
		f.limit = true
		return f
	}
	return nil
}

// resolve follows an alias to its anchor, refusing one that is still being
// built.
func (c *converter) resolve(n *yamlv3.Node, path string) (*yamlv3.Node, *failure) {
	if n.Kind != yamlv3.AliasNode {
		return n, nil
	}
	if n.Alias == nil || c.active[n.Alias] {
		return nil, at(n, path, "recursive YAML alias")
	}
	return n.Alias, nil
}

func (c *converter) value(n *yamlv3.Node, path string) (any, *failure) {
	if fail := c.count(n, path); fail != nil {
		return nil, fail
	}
	target, fail := c.resolve(n, path)
	if fail != nil {
		return nil, fail
	}
	switch target.Kind {
	case yamlv3.MappingNode:
		return c.mapping(target, path)
	case yamlv3.SequenceNode:
		if target.ShortTag() != "!!seq" {
			return nil, at(n, path, "YAML tag not allowed")
		}
		c.active[target] = true
		defer delete(c.active, target)
		list := make([]any, 0, len(target.Content))
		for i, item := range target.Content {
			v, fail := c.value(item, path+"["+strconv.Itoa(i)+"]")
			if fail != nil {
				return nil, fail
			}
			list = append(list, v)
		}
		return list, nil
	case yamlv3.ScalarNode:
		return scalar(target, n, path)
	}
	return nil, at(n, path, "unsupported YAML node")
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// mapping converts a map. Explicit keys are collected first and merge keys
// applied after, so an explicit key wins wherever "<<" appears in the map.
func (c *converter) mapping(n *yamlv3.Node, path string) (map[string]any, *failure) {
	if n.ShortTag() != "!!map" {
		return nil, at(n, path, "YAML tag not allowed")
	}
	c.active[n] = true
	defer delete(c.active, n)
	out := make(map[string]any, len(n.Content)/2)
	var merge *yamlv3.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode, valueNode := n.Content[i], n.Content[i+1]
		if fail := c.count(keyNode, path); fail != nil {
			return nil, fail
		}
		if keyNode.Kind == yamlv3.ScalarNode && keyNode.ShortTag() == "!!merge" {
			if merge != nil {
				return nil, at(keyNode, join(path, "<<"), "duplicate YAML key")
			}
			merge = valueNode
			continue
		}
		target, fail := c.resolve(keyNode, path)
		if fail != nil {
			return nil, fail
		}
		if target.Kind != yamlv3.ScalarNode || !stringLike(target) {
			return nil, at(keyNode, path, "YAML key must be a string")
		}
		key := target.Value
		field := join(path, key)
		if _, exists := out[key]; exists {
			return nil, at(keyNode, field, "duplicate YAML key")
		}
		v, fail := c.value(valueNode, field)
		if fail != nil {
			return nil, fail
		}
		out[key] = v
	}
	if merge == nil {
		return out, nil
	}
	field := join(path, "<<")
	if fail := c.count(merge, field); fail != nil {
		return nil, fail
	}
	target, fail := c.resolve(merge, field)
	if fail != nil {
		return nil, fail
	}
	sources := []*yamlv3.Node{merge}
	if target.Kind == yamlv3.SequenceNode {
		// With a list of maps, an earlier one wins over a later one.
		sources = target.Content
	}
	for _, source := range sources {
		resolved, fail := c.resolve(source, field)
		if fail != nil {
			return nil, fail
		}
		if resolved.Kind != yamlv3.MappingNode {
			return nil, at(source, field, "YAML merge value must be a mapping or a list of mappings")
		}
		merged, fail := c.mapping(resolved, path)
		if fail != nil {
			return nil, fail
		}
		for key, v := range merged {
			if _, exists := out[key]; !exists {
				out[key] = v
			}
		}
	}
	return out, nil
}

// stringLike reports whether a scalar is a string for this source. A plain
// scalar the library would turn into a timestamp stays text: dates and
// durations are interpreted when a section is decoded, by the target type.
func stringLike(n *yamlv3.Node) bool {
	switch n.ShortTag() {
	case "!!str":
		return true
	case "!!timestamp":
		return n.Style&yamlv3.TaggedStyle == 0
	}
	return false
}

// scalar converts target; position is the node the error should point at,
// which differs from target when the value arrived through an alias.
func scalar(target, position *yamlv3.Node, path string) (any, *failure) {
	if stringLike(target) {
		return target.Value, nil
	}
	text := target.Value
	switch target.ShortTag() {
	case "!!merge":
		// "<<" in value position is just that text.
		return text, nil
	case "!!null":
		switch text {
		case "", "~", "null", "Null", "NULL":
			return nil, nil
		}
		return nil, at(position, path, "invalid YAML null")
	case "!!bool":
		// YAML 1.2: only these spellings. yes/no/on/off are strings.
		switch text {
		case "true", "True", "TRUE":
			return true, nil
		case "false", "False", "FALSE":
			return false, nil
		}
		return nil, at(position, path, "invalid YAML boolean")
	case "!!int":
		if canonicalInt(text) {
			return json.Number(text), nil
		}
		// Hexadecimal, octal, binary, signs and digit separators: the
		// library's own resolution gives the value, written back in
		// decimal.
		var v any
		if err := target.Decode(&v); err == nil {
			switch i := v.(type) {
			case int:
				return json.Number(strconv.FormatInt(int64(i), 10)), nil
			case int64:
				return json.Number(strconv.FormatInt(i, 10)), nil
			case uint64:
				return json.Number(strconv.FormatUint(i, 10)), nil
			}
		}
		return nil, at(position, path, "invalid YAML integer")
	case "!!float":
		lower := strings.ToLower(text)
		if strings.Contains(lower, "inf") || strings.Contains(lower, "nan") {
			return nil, at(position, path, "non-finite YAML number")
		}
		// The digits are kept as written rather than passed through a
		// float64, so a long decimal is not rounded on the way in.
		if number := decimal(text); json.Valid([]byte(number)) {
			return json.Number(number), nil
		}
		var v any
		if err := target.Decode(&v); err == nil {
			if f, ok := v.(float64); ok {
				if number := strconv.FormatFloat(f, 'g', -1, 64); json.Valid([]byte(number)) {
					return json.Number(number), nil
				}
			}
		}
		return nil, at(position, path, "invalid YAML number")
	}
	return nil, at(position, path, "YAML tag not allowed")
}

// canonicalInt reports whether text is already a decimal integer in JSON
// form, which is the common case and needs no conversion.
func canonicalInt(text string) bool {
	digits := strings.TrimPrefix(text, "-")
	if digits == "" || len(digits) > 18 || (len(digits) > 1 && digits[0] == '0') || text == "-0" {
		return false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	return true
}

// decimal rewrites a YAML float into JSON number syntax without touching
// its digits: ".5" becomes "0.5", "1." becomes "1", "+1e3" becomes "1e3",
// "007.5" becomes "7.5". The caller validates the result.
func decimal(text string) string {
	s := strings.ReplaceAll(text, "_", "")
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	mantissa, exponent := s, ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa, exponent = s[:i], "e"+strings.TrimPrefix(s[i+1:], "+")
	}
	whole, fraction, dotted := strings.Cut(mantissa, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	out := sign + whole
	if dotted && fraction != "" {
		out += "." + fraction
	}
	return out + exponent
}
