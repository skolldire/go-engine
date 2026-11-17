# validation

Experimental D54 candidate, not released. Go 1.27.4; depends on the repository's existing `github.com/go-playground/validator/v10` v10.30.6.

Struct validation by `validate` tags, with two guarantees the library alone does not give: there is no global validator, and a validation error never contains a field's value. No type of the library appears in this package's API.

```go
v, err := validation.New(validation.DefaultConfig(), validation.Dependencies{})
// handle err; build once, inject where needed

type Signup struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
}

err = v.Struct(ctx, &Signup{Email: "nope", Password: "short"})
var failed validation.Errors
if errors.As(err, &failed) {
	// failed[0] == FieldError{Path: "email", Rule: "email"}
	// failed[1] == FieldError{Path: "password", Rule: "min", Param: "12"}
}
```

Tags use the library's syntax and its built-in rules (`required`, `email`, `min`, `oneof`, `dive`, cross-field rules and the rest); see its documentation for the list.

## Configuration

| Field | Default | Rule |
|---|---|---|
| `FieldNames` | `"json"` | `"json"` or `"go"` |
| `RequiredStruct` | `true` | |

`Config.Validate` completes nothing: an empty `FieldNames` is rejected, so start from `DefaultConfig()`. `ConfigError{Field, Reason}` wraps `ErrInvalidConfig`. Both fields carry snake_case `mapstructure` and `json` tags.

**`FieldNames`** decides how a failing field is named in `FieldError.Path`. With `"json"` it is the name in the field's `json` tag; a field with no tag, with `json:"-"` or with an options-only tag (`json:",omitempty"`) keeps its Go name. With `"go"` it is always the Go name. Nested structs join with `.`, and slice, array and map elements use brackets: `address.city`, `items[2].email`, `labels[region]`.

**`RequiredStruct`** set means `Struct` accepts only a struct or a pointer to one. Unset, it also accepts a slice, array or map and validates it element by element, with paths such as `[1].email`. Any other value is `ErrInvalidCall` in both modes.

## Rules

A `Rule` is a named check that tags can refer to:

```go
type Rule func(ctx context.Context, value any, param string) bool

v, err := validation.New(cfg, validation.Dependencies{Rules: map[string]validation.Rule{
	"not_blank":   func(_ context.Context, value any, _ string) bool { s, ok := value.(string); return ok && strings.TrimSpace(s) != "" },
	"multiple_of": func(_ context.Context, value any, param string) bool { /* `validate:"multiple_of=5"` → param "5" */ },
}})
```

`value` is the field's value and `param` the text after `=` in the tag, empty when there is none. `ctx` is the context given to `Struct`, so a rule can read request-scoped values or observe cancellation. A panic in a rule propagates to the caller of `Struct` with its original value.

Rules are registered in `New`, and a rule that cannot be registered is `ErrInvalidRule` there, never ignored:

- an empty name, or one with characters other than letters, digits, `_` and `-`;
- a nil `Rule`;
- a name the library already uses, whether a built-in rule (`required`, `email`), an alias (`iscolor`) or a reserved tag (`dive`, `omitempty`).

A rule belongs to the `Validator` it was given to. Two validators can register the same name with different meanings, and neither sees the other's rules.

As in the library, a rule is not run for a field skipped by `omitempty` or for a nil pointer.

## Errors

`Struct(ctx, value)` returns:

- `nil` when every rule passes;
- `Errors` (use `errors.As`) when rules fail, in the order the library reports them: declaration order, depth first;
- `ErrNilContext` for a nil context;
- an error matching `ErrInvalidCall` for a nil value, a nil pointer, a value of a kind that cannot be validated, or a misuse of tags.

`FieldError` has three fields, `Path`, `Rule` and `Param`, and deliberately no value and no library message. A failing `password` field appears as its path and the rule it broke; what was submitted cannot reach a log line or an HTTP response through the error, because the error does not hold it. `Errors.Error()` reads `validation failed: password (min=12); items[2].email (email)`. The library's own error is not reachable through `errors.As` or `errors.Unwrap`.

Two things in a path come from data rather than from the type, and are worth knowing:

- **Map keys.** A failure inside a map is located by its key, `labels[region]`. Do not key maps by secrets.
- **`Param`** is the text written in the struct tag, not user input. For a cross-field rule such as `eqfield=Password` it is the other field's name.

A malformed tag (a misspelled rule, a parameter that does not parse, a rule that does not apply to the field's type) makes the library panic. `Struct` turns that into an error matching `ErrInvalidCall`. For the two most common mistakes, an undefined rule and an invalid tag, the message names the rule and the field; any other is reported with a fixed message, since the library's text cannot be shown to be free of field values. This is a programming error: find it with a test that validates one instance of each type.

## Behavior to know

- **No global state.** There is no package-level validator, no `Set`/`Get` and no singleton. Build a `Validator` and pass it to what needs it.
- **Concurrency and reuse.** A `Validator` is safe for concurrent use and should be reused: the library caches what it learns about each struct type, so the cache grows with the number of distinct types validated, not with the number of calls.
- **`required` on a nested struct is applied.** The validator is built with the library's `WithRequiredStructEnabled`, which the library recommends and will make its default: `required` on a struct-typed field fails for the zero value instead of being silently ignored.
- **Only `Struct` is offered.** There is no counterpart to the library's `Var` for a single value. Put the value in a struct, or check it directly.
- **Translations and custom messages** are not part of this package. A caller that needs wording maps `Rule` and `Param` to text.

## Recipe: 422 with `router/errors`

Turning validation errors into HTTP is the caller's job. With `router/errors` it is a classifier, exercised by `examples/standalone/validation/composed`:

```go
func classify(err error) routererrors.Problem {
	var failed validation.Errors
	if errors.As(err, &failed) {
		parts := make([]string, 0, len(failed))
		for _, fe := range failed {
			parts = append(parts, fe.Path+": "+fe.Rule)
		}
		return routererrors.Problem{
			Status:  http.StatusUnprocessableEntity,
			Code:    "validation_failed",
			Title:   "Unprocessable Entity",
			Details: strings.Join(parts, "; "), // the "detail" member of the response
		}
	}
	// ErrInvalidCall is a server-side mistake, not a client one.
	return routererrors.Problem{Status: http.StatusInternalServerError, Code: "internal_error", Title: "Internal Server Error"}
}
```

The response body then carries `"detail": "email: email; password: min"`. `router/errors` bounds the detail with `MaxDetailsBytes`; a request with very many failures is truncated there.

See [MIGRATION](MIGRATION.md), [RELEASE_NOTES](RELEASE_NOTES.md) and the independent consumers under `examples/standalone/validation`. The proposed v0.1.0 is unapproved and unreleased; this is not published stability.
