# Changelog

## Experimental (not released)

- D54: struct validation over go-playground/validator v10.30.6 with no library type in the API, no global validator and `Errors`/`FieldError` that carry path, rule and parameter but never a value.
- Injected named rules receiving the caller's context, refused at construction when empty, invalid, nil or colliding with a built-in; malformed tags reported as `ErrInvalidCall` instead of a panic.
- `FieldNames` json or go; `RequiredStruct` to accept only structs or also collections; `router/errors` 422 recipe and independent consumers.
- Proposed v0.1.0 unapproved; no published base or legacy migration.
