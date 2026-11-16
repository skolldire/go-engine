# router

Experimental D51/D51.1/D51.2 candidate, not released. Go1.27.4, stdlib only, including tests. Routing, policy and optional server are separate: New/NewPolicy perform no I/O or environment lookup. Literal configurations are validated without silently applying server defaults.

```go
m, err := router.New(router.Config{AllowedHosts: []string{"api.example.com", "*.example.com"}})
// Handle err, then register before Freeze:
err = m.Handle("GET", "", "/items/{id}", handler)
err = m.Freeze()
```

Config has AllowedHosts, TrustedProxies and optional *CORS with snake_case tags. The CORS pointer explicitly distinguishes absent/disabled policy from present lists; it is the approved presence exception. New and NewPolicy copy policy lists. ConfigError{Field,Reason} wraps ErrInvalidConfig; invalid registration/calls wrap ErrInvalidCall. There is no default AllowedHosts, no public catch-all host, environment read, global mux, compression, request ID, logging, recovery or timeout installed implicitly.

Allowed hosts normalize lowercase, remove port and one trailing DNS dot. ASCII DNS labels/A-labels, IPv4 and bracketed IPv6 are supported; raw Unicode, invalid names, empty lists/entries and IP wildcards are rejected. *.example.com matches a.example.com, not the apex or a.b.example.com. Policy reads only Request.Host; forwarded host headers never authorize a host. Disallowed/malformed hosts return empty421 before other policy/middleware.

Handle(method,host,pattern,http.Handler) uses a capitalized HTTP token method (or empty for any), an allowed normalized host (or empty for hostless), and native ServeMux path syntax, including {id}, {path...} and {$}. No embedded method or host in pattern. A host covered by an AllowedHosts wildcard can have its own exact group. Host dispatch chooses an existing exact group, then matching wildcard group, then hostless. Only that group's ServeMux matches method/path: a404/405 does not fall through to another host group. Duplicates return ErrDuplicateRoute; conflicts/invalid native patterns return ErrInvalidCall using a throwaway validation mux. Native matching, PathValue, redirects and method handling are not reimplemented.

Use accepts explicit middleware. Execution is Policy(host→proxy→CORS), then Use middleware in given order (first outermost), then route. Freeze is explicit, idempotent, and seals Use/Handle with ErrFrozen. Before publication ServeHTTP returns empty503; it never auto-freezes. Registration, Freeze and serving are synchronized. Middleware factories execute outside the registration lock; factories must return valid handlers and must not recursively Freeze the same mux. Once frozen no route history is retained beyond the dispatch tables. Callbacks/handlers own their concurrent safety.

TrustedProxies accepts IPs/CIDRs. Without them headers are ignored. An untrusted RemoteAddr also makes forwarding headers irrelevant. With a trusted peer, X-Forwarded-For is walked right-to-left until the first untrusted IP; the entire remaining untrusted prefix is ignored. A malformed encountered entry fails closed to the peer; an all-trusted chain yields the leftmost supplied IP. X-Real-IP is used only when XFF is absent and the peer is trusted. ClientIP(ctx)(netip.Addr,bool) exposes the selected value; RemoteAddr stays unchanged. No proxy identity or IP is inferred from Host.

CORS origins are exact scheme://host[:port], lowercased; no path, final slash, null or partial wildcard. A sole * permits all origins without credentials. Config methods default to GET/HEAD/POST; headers default to Accept, Accept-Language, Content-Language, Content-Type and Range (safelisted names). A preflight carries header names, not values: the caller/browser remains responsible for safelisted value constraints. Header * with credentials is invalid. ExposedHeaders is explicit. MaxAge must be nonnegative, emitted in whole seconds rounded down and omitted when0. Vary: Origin is added whenever CORS is configured, including missing/disallowed origins. Allowed explicit origins/credentials echo the request origin; sole * emits *. Recognized OPTIONS preflights never reach a route: allowed204, denied403 empty. Normal disallowed origins proceed without access-control headers. CORS is a browser policy, not authentication.

The `middleware` subpackage exposes:

```go
RequestID(RequestIDConfig) (router.Middleware, error)
RequestIDFrom(context.Context) (string, bool)
Recover(func(http.ResponseWriter, *http.Request, any)) (router.Middleware, error)
Timeout(time.Duration) (router.Middleware, error)
```

RequestIDConfig has Header (empty defaults X-Request-ID), AcceptIncoming (false) and optional Generate func()string. Default generation uses crypto/rand16 bytes in hex. Incoming IDs are accepted only when opted in, printable ASCII1..128 bytes; otherwise regenerated. ID is in context and response header; no trusted incoming default. Invalid generated IDs panic with fixed text, no value. Recover requires a callback, recovers ordinary panics but rethrows http.ErrAbortHandler. It never publishes the panic itself. After headers/body are committed the callback cannot reliably replace the response: observe/abort according to application policy. Timeout requires d>0 and only derives/cancels a request context; it never writes or uses http.TimeoutHandler. Handlers must cooperate.

DefaultServerConfig: Listen empty(required), ReadHeaderTimeout5s>0, ReadTimeout0≥0, WriteTimeout30s≥0, IdleTimeout120s>0, ShutdownTimeout15s>0, MaxHeaderBytes1MiB>0, optional paired TLSCertFile/TLSKeyFile; all snake_case tags. Serve(ctx,cfg,h) opens the listener only when invoked. ServeListener(ctx,cfg,ln,h) takes its lifetime after validation, supports a preconfigured TLS listener when paths are absent; paired paths use native ServeTLS. No PORT, signal.Notify or shutdown hooks. Nil ctx/handler/listener is ErrInvalidCall. Already-canceled Serve does not listen; ServeListener closes the supplied listener. Graceful cancellation returns nil; startup/serve/shutdown errors preserve causes. Shutdown uses WithoutCancel(ctx)+ShutdownTimeout; after a failed drain remaining server connections are closed, while arbitrary noncooperative handler code cannot be forcibly terminated. One serving goroutine is awaited before return. The application uses signal.NotifyContext and orders resource closure afterward.

Use operations on a separate internal listener and supply caller middleware/observability explicitly. See independent minimal/composed consumers. The composed assembly repeats host policy, request ID, error/recovery mapping, health wiring, separate operations handler and server shutdown sequencing; these are future presets/httpapi assembly responsibilities, not hidden router defaults. See MIGRATION and RELEASE_NOTES for evidence and limitations. Proposed v0.1.0 unapproved.
