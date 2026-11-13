# client minimal consumer

With Go 1.27.4: `GOWORK=off GOPROXY=off go run -mod=readonly .`.
Only client and stdlib; sole replace `../../../../client`. No engine/app,
config, concurrency or SDK. Composed uses an injected in-memory RoundTripper;
it never opens a port or sends a request to the network. The application owns
middleware deadlines/resources. This is local candidate validation, not E7.
