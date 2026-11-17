# Migration — experimental, not released

The legacy `pkg/utilities/validation` and its consumers (`aws/pkg/clients/sns`, `ses` and `ssm`) stay unchanged and supported. There are no aliases, compatibility bridges or deprecations; nothing is migrated automatically.

| Legacy `pkg/utilities/validation` | `validation` |
|---|---|
| `NewValidator()` returning `*validator.Validate` | `New(cfg, deps)` returning `*validation.Validator` and an error |
| the library's type as the public API | no library type exposed |
| `SetGlobalValidator` / `GetGlobalValidator` | none; an instance is built and injected |
| json tag names, always | `FieldNames`: `"json"` (default) or `"go"` |
| built-in `not_empty`, registration error ignored | not built in; register it as a `Rule`, or use `required` |
| `validator.ValidationErrors` with `Value()` and messages | `Errors` of `FieldError{Path, Rule, Param}`, no values |
| a malformed tag panics | an error matching `ErrInvalidCall` |
| `Var(value, "required,email")` | none; see below |

## Construction: from a global to an instance

Before:

```go
v := validation.GetGlobalValidator() // created on first use, shared by the process
err := v.Struct(req)
```

After:

```go
v, err := validation.New(validation.DefaultConfig(), validation.Dependencies{})
// handle err; then pass v to the handlers or services that validate
err = v.Struct(ctx, &req)
```

`Struct` takes a context, which rules receive. Code that replaced the global in tests with `SetGlobalValidator` now passes a different `*Validator`.

## `not_empty`

The legacy validator registered `not_empty` (a string that is not blank after trimming) and discarded the registration error. It is not built in here. Either register it:

```go
v, err := validation.New(cfg, validation.Dependencies{Rules: map[string]validation.Rule{
	"not_empty": func(_ context.Context, value any, _ string) bool {
		s, ok := value.(string)
		return ok && strings.TrimSpace(s) != ""
	},
}})
```

or use `required` where a non-zero string is enough. `required` accepts a string of spaces; `not_empty` does not. Existing struct tags saying `not_empty` keep working once the rule is registered; without it, validating such a struct is `ErrInvalidCall`.

## Errors

Before, callers received the library's errors, which include the offending value:

```go
var verrs validator.ValidationErrors
if errors.As(err, &verrs) {
	for _, fe := range verrs {
		log.Printf("%s failed %s with %v", fe.Namespace(), fe.Tag(), fe.Value()) // the value is logged
	}
}
```

After:

```go
var failed validation.Errors
if errors.As(err, &failed) {
	for _, fe := range failed {
		log.Printf("%s failed %s", fe.Path, fe.Rule) // there is no value to log
	}
}
```

| Library `FieldError` | `validation.FieldError` |
|---|---|
| `Namespace()` → `Signup.items[2].email` | `Path` → `items[2].email` (no root type name) |
| `Tag()` | `Rule` |
| `Param()` | `Param` |
| `Value()`, `Error()`, `Translate()` | none |

`errors.As(err, &validator.ValidationErrors{})` no longer matches. Code that needs the rejected value reads it from the struct it validated.

## Differences in behavior

- **`required` on a nested struct value is now applied.** The legacy validator used the library's default, which ignores it. A struct field tagged `required` now fails when it is the zero value.
- **A malformed tag is an error, not a panic.**
- **`json:"-"` fields** are named by their Go name in paths; the legacy tag-name function did the same.
- **`Var` has no counterpart.** The three AWS clients validate single values with `GetGlobalValidator().Var(value, "required,email")`. This package validates structs. To move such a check, declare a small struct with the tag, or keep the check as direct code. Those clients are not migrated by this lot.
- **Registering a rule over a built-in name is refused.** With the library, `RegisterValidation("email", fn)` silently replaces the built-in rule.
