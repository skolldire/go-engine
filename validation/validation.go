// Package validation validates structs by their `validate` tags and reports
// failures as paths and rule names, never as values.
//
// It wraps go-playground/validator without exposing any of its types, so the
// library can change underneath, and it keeps no global validator: every
// Validator is built explicitly and injected where it is used.
package validation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	ErrInvalidConfig = errors.New("invalid validation configuration")
	ErrInvalidCall   = errors.New("invalid validation call")
	ErrNilContext    = errors.New("nil validation context")
	ErrInvalidRule   = errors.New("invalid validation rule")
)

// ConfigError names the rejected field with a fixed reason.
type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "validation: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

// FieldNames values.
const (
	fieldNamesJSON = "json"
	fieldNamesGo   = "go"
)

type Config struct {
	// FieldNames selects how a failing field is named in FieldError.Path:
	// "json" uses the json tag, "go" the Go field name.
	FieldNames string `mapstructure:"field_names" json:"field_names"`
	// RequiredStruct limits Struct to a struct or a pointer to one.
	RequiredStruct bool `mapstructure:"required_struct" json:"required_struct"`
}

// Rule is a named check a struct tag can refer to. value is the field's
// value and param the text after "=" in the tag, empty when there is none.
type Rule func(ctx context.Context, value any, param string) bool

type Dependencies struct {
	Rules map[string]Rule
}

// FieldError describes one failed rule. It has no field for the value on
// purpose: a password or a token must not be able to reach a log or an HTTP
// response through a validation error.
type FieldError struct {
	Path  string
	Rule  string
	Param string
}

// Errors is the error Struct returns when rules fail, in the order the
// library reports them.
type Errors []FieldError

// Error lists each path with its rule, and no value.
func (e Errors) Error() string {
	var b strings.Builder
	b.WriteString("validation failed")
	for i, fe := range e {
		if i == 0 {
			b.WriteString(": ")
		} else {
			b.WriteString("; ")
		}
		b.WriteString(fe.Path)
		b.WriteString(" (")
		b.WriteString(fe.Rule)
		if fe.Param != "" {
			b.WriteString("=")
			b.WriteString(fe.Param)
		}
		b.WriteString(")")
	}
	return b.String()
}

// Validator is safe for concurrent use and meant to be reused: the library
// caches what it learns about each struct type.
type Validator struct {
	lib            *validator.Validate
	goNames        bool
	requiredStruct bool
}

func DefaultConfig() Config { return Config{FieldNames: fieldNamesJSON, RequiredStruct: true} }

func (cfg Config) Validate() error {
	if cfg.FieldNames != fieldNamesJSON && cfg.FieldNames != fieldNamesGo {
		return &ConfigError{"FieldNames", "must be json or go"}
	}
	return nil
}

// New builds a Validator and registers deps.Rules. A rule that cannot be
// registered is an error here, at construction, so that a struct tag never
// refers to a rule that silently does not exist.
func New(cfg Config, deps Dependencies) (*Validator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// The library recommends this option and makes it the default in its
	// next major version: without it, "required" on a nested struct value
	// is silently ignored.
	lib := validator.New(validator.WithRequiredStructEnabled())
	if cfg.FieldNames == fieldNamesJSON {
		lib.RegisterTagNameFunc(jsonName)
	}
	// Sorted so that the rule reported is the same on every run.
	for _, name := range sortedNames(deps.Rules) {
		rule := deps.Rules[name]
		if !validRuleName(name) {
			return nil, fmt.Errorf("%w: name must be letters, digits, '_' or '-'", ErrInvalidRule)
		}
		if rule == nil {
			return nil, fmt.Errorf("%w: %s is nil", ErrInvalidRule, name)
		}
		if known(lib, name) {
			return nil, fmt.Errorf("%w: %s is a built-in rule", ErrInvalidRule, name)
		}
		if err := lib.RegisterValidationCtx(name, adapt(rule)); err != nil {
			return nil, fmt.Errorf("%w: %s cannot be registered", ErrInvalidRule, name)
		}
	}
	return &Validator{lib: lib, goNames: cfg.FieldNames == fieldNamesGo, requiredStruct: cfg.RequiredStruct}, nil
}

// jsonName returns the json tag name. An empty result makes the library
// fall back to the Go name, which is what happens for `json:"-"` and for a
// tag that only carries options.
func jsonName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "-" {
		return ""
	}
	return name
}

func sortedNames(rules map[string]Rule) []string {
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	// Insertion sort: rule sets are small and this avoids an import.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

// validRuleName accepts a conservative alphabet, a subset of what the
// library allows, so a name can never contain a character the tag syntax
// gives meaning to.
func validRuleName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

const undefinedRule = "Undefined validation function"

// known reports whether the library already gives name a meaning, as a
// rule, an alias or a reserved tag. The library has no lookup for this and
// registering over a built-in replaces it without notice, so the name is
// tried on a throwaway value: only "undefined" means it is free.
func known(lib *validator.Validate, name string) (taken bool) {
	defer func() {
		if r := recover(); r != nil {
			message, _ := r.(string)
			taken = !strings.HasPrefix(message, undefinedRule)
		}
	}()
	_ = lib.Var("", name)
	return true
}

// call carries what one Struct call needs across the library: the caller's
// context for the rules, and whether a rule is running, which is how a
// panic raised by a rule is told apart from one raised by the library.
type call struct {
	ctx    context.Context
	inRule bool
}

type callKey struct{}

func adapt(rule Rule) validator.FuncCtx {
	return func(ctx context.Context, fl validator.FieldLevel) bool {
		c, _ := ctx.Value(callKey{}).(*call)
		if c == nil {
			// Only reachable through the name probe in New.
			return rule(ctx, fl.Field().Interface(), fl.Param())
		}
		c.inRule = true
		ok := rule(c.ctx, fl.Field().Interface(), fl.Param())
		c.inRule = false
		return ok
	}
}

// Struct validates value against its `validate` tags. It returns nil, an
// Errors listing the failed rules, or an error matching ErrNilContext or
// ErrInvalidCall.
func (v *Validator) Struct(ctx context.Context, value any) (err error) {
	if ctx == nil {
		return ErrNilContext
	}
	if v == nil || v.lib == nil {
		return fmt.Errorf("%w: validator must come from New", ErrInvalidCall)
	}
	if value == nil {
		return fmt.Errorf("%w: nil value", ErrInvalidCall)
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Errorf("%w: nil pointer", ErrInvalidCall)
		}
		rv = rv.Elem()
	}
	isStruct := rv.Kind() == reflect.Struct
	collection := rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array || rv.Kind() == reflect.Map
	if !isStruct && (v.requiredStruct || !collection) {
		return fmt.Errorf("%w: value must be a struct or a pointer to one", ErrInvalidCall)
	}
	c := &call{ctx: ctx}
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if c.inRule {
			// A rule's panic is the application's: it goes on unchanged.
			panic(r)
		}
		err = usageError(r)
	}()
	inner := context.WithValue(ctx, callKey{}, c)
	var failure error
	if isStruct {
		failure = v.lib.StructCtx(inner, rv.Interface())
	} else {
		// A slice, array or map is validated element by element.
		failure = v.lib.VarCtx(inner, rv.Interface(), "dive")
	}
	if failure == nil {
		return nil
	}
	var failed validator.ValidationErrors
	if !errors.As(failure, &failed) {
		return fmt.Errorf("%w: value cannot be validated", ErrInvalidCall)
	}
	root := ""
	if isStruct && rv.Type().Name() != "" {
		root = rv.Type().Name() + "."
	}
	out := make(Errors, 0, len(failed))
	for _, fe := range failed {
		path := fe.Namespace()
		if v.goNames {
			path = fe.StructNamespace()
		}
		out = append(out, FieldError{Path: strings.TrimPrefix(path, root), Rule: fe.Tag(), Param: fe.Param()})
	}
	return out
}

// usageError turns a library panic into an error. The library panics on a
// misuse of tags, and most of its messages are built from the tag and the
// field name only. Those two known ones are passed on because they say what
// to fix; any other is replaced, since it cannot be shown to be free of
// field values.
func usageError(r any) error {
	if message, ok := r.(string); ok && (strings.HasPrefix(message, undefinedRule) || strings.HasPrefix(message, "Invalid validation tag on field")) {
		return fmt.Errorf("%w: %s", ErrInvalidCall, message)
	}
	return fmt.Errorf("%w: a validate tag is malformed or does not apply to its field type", ErrInvalidCall)
}
