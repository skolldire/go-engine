# router/errors

Experimental candidate, not released. Package `errors` is a stdlib-only HTTP error responder; import it as `routererrors`. D49, D49.1 and D49.2 are approved.

```go
cfg := routererrors.DefaultConfig()
responder, err := routererrors.NewResponder(cfg, routererrors.Dependencies{
    Classify: func(err error) routererrors.Problem {
        return routererrors.Problem{Status: 500, Code: "internal_error", Title: "Internal Server Error"}
    },
})
// Handle construction errors. In an HTTP handler, before committing the response:
// writeErr := responder.Write(req.Context(), writer, internalErr)
```

`Config` has snake_case JSON/mapstructure tags. Defaults are IncludeRequestID=true and MaxDetailsBytes=4096. Explicit literals are validated without normalization; MaxDetailsBytes must be positive. Classify is mandatory. NewResponder copies configuration and functions, performs no callbacks or I/O and creates no goroutines. The responder is immutable; do not copy it to simulate a separately constructed instance.

Write validates context, writer (including a typed nil pointer), then input error. Nil context returns ErrNilContext; other invalid calls wrap ErrInvalidCall. The zero value and nil receiver are unsupported. ConfigError identifies Field and Reason and unwraps ErrInvalidConfig without printing values.

Classify receives the original error exactly once. The module never calls its Error/String methods or inspects its causes. The caller authorizes all public content and supplies an unknown-error case. A successfully handled business error is not returned by Write.

The response is compatible with [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html):

```json
{"type":"about:blank","status":404,"title":"Resource not found","detail":"Public explanation","code":"not_found","request_id":"application-id"}
```

Only these members exist; detail and request_id are omitted when empty. There is no instance. With about:blank the RFC recommends the standard status phrase for title; caller-specific titles are permitted by this contract. Headers are Content-Type `application/problem+json`, Cache-Control `no-store` and X-Content-Type-Options `nosniff`. Existing unrelated headers remain caller-owned.

Status must be 400..599. Code is 1..64 bytes of ASCII letters, digits, underscore, dot or hyphen. Title is nonempty valid UTF-8, at most 256 bytes. Invalid core fields produce a fixed 500/internal_error/Internal Server Error fallback without details, followed by ErrInvalidProblem. Invalid values are never published. A valid request ID can accompany the fallback.

Details is public text, not an arbitrary error. Invalid UTF-8 or content exceeding MaxDetailsBytes is omitted completely, without an extra error. Bytes are measured before JSON escaping; this is not a body or process-memory limit. No redaction of deliberately supplied public text is promised.

RequestID is optional and called at most once with the exact context, only when IncludeRequestID is true. Empty, invalid UTF-8, more than 128 bytes or Unicode control characters (C0/C1 included) cause complete omission. IDs are neither generated nor normalized, read from headers, or copied into headers. The application must choose a trusted source.

JSON is prepared before headers. Write makes one body-write attempt, never retries or appends http.Error. Writer errors preserve identity and errors.Is/errors.As; a short write without error becomes io.ErrShortWrite. If fallback and writing both fail, both causes remain accessible. A returned error does not imply nothing was written. Invoke once per response before committing headers; previous writes cannot universally be detected. Writers are borrowed and never closed.

Canceled contexts do not automatically suppress responses. No contexts are derived and no writer is forcibly interrupted. Panics of callbacks or writers propagate. Callback failures before the commit phase leave module headers/body untouched. Foreign writer causes are not redacted. Reuse is concurrent-safe when callbacks are safe and each call owns its writer; sharing one writer concurrently is unsupported. There is no history, cache, background work, logging or router dependency.

See [MIGRATION.md](MIGRATION.md), [RELEASE_NOTES.md](RELEASE_NOTES.md) and the independent minimal/composed consumers. Proposed version v0.1.0 remains unapproved; catalog track stays unreleased.
