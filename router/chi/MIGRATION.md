# Migration — experimental, not released

Legacy pkg/router and all consumers remain unchanged. No aliases, compatibility bridges or deprecations.

| Before | Explicit assembly |
|---|---|
| NewService normalizes zero timeouts | New(Config), DefaultServerConfig plus literal validation |
| Implicit chi middleware stack | router/middleware constructors or caller/native chi middleware |
| Implicit chi Logger and compression | caller policy outside neutral router |
| Any Host accepted | required AllowedHosts; unknown/malformed421 |
| trustedRealIP mutates RemoteAddr | ClientIP(ctx), original peer retained |
| go-chi/cors | explicit stdlib CORS policy with D51.1 defaults |
| Profile-gated pprof in API mux | explicit operations handler, preferably internal listener |
| PORT/signal.Notify/shutdown hooks inside Run | explicit Listen, caller signal.NotifyContext and ordered resource shutdown |
| Router() exposes chi everywhere | router/chi.Native only when selected |
| Legacy deps can return503 and publish cause | diagnostic200, private DTO; readiness503 separately |

Before: `app := legacy.NewService(cfg); app.AddRoute("GET", "/items/{id}", handler); app.Run(ctx)`.

After: construct `router.New(router.Config{AllowedHosts: []string{"api.example.com"}})`, add explicit middleware with Use, Handle the route, Freeze, then call `router.Serve(ctx, serverConfig, mux)`. Check every returned error. `serverConfig := router.DefaultServerConfig(); serverConfig.Listen = ":8080"` applies defaults deliberately. Application owns signal handling and closes resources after Serve returns. For chi register native routes before serving Adapter.Handler; no runtime registration safety is invented.

Operational routes are not automatically mounted in the neutral router or chi. `operations.New(operations.DefaultConfig(), operations.Dependencies{Health: registry})` can be served separately with an explicit internal listener. WriteTimeout must exceed MaxProfileDuration when enabling profiling. Do not expose diagnostics or profiling merely because an application profile string permits it.

Recovery requires an explicit callback and does not replace already-started responses. ErrAbortHandler is rethrown. Timeout only cancels context; it does not send a504 or forcibly terminate a handler. Request IDs are untrusted unless acceptance is deliberately enabled. Host and CORS policy are not authentication.
