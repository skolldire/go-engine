# router/chi

Experimental, not released. D51 adapter around existing chi/v5 v5.5.0 and router.Policy; no duplicated host/proxy/CORS policy. `New(router.Config)(*Adapter,error)`, `Native()*chi.Mux`, `Handler()http.Handler`. New validates policy without I/O. Handler wraps the native mux in Policy. There is no Freeze and no host-specific route API: every native route serves any host allowed by policy. No translation to a catch-all host.

```go
adapter, err := chiadapter.New(router.Config{AllowedHosts: []string{"api.example.com"}})
// Handle err; explicitly configure native middleware if wanted:
adapter.Native().Use(myMiddleware)
adapter.Native().Get("/items/{id}", myHandler)
handler := adapter.Handler()
```

No chi Logger/Recoverer/Compress/RequestID is implicit. Register routes/middleware before serving; mutation while serving is a caller race. Native chi types appear only in this adapter module. The optional router server can serve Handler, but the adapter owns no server or resource lifecycle. Module/tests resolve only router, chi and stdlib; consumers measure their own graphs. See MIGRATION/RELEASE_NOTES. Proposed v0.1.0 unapproved.
