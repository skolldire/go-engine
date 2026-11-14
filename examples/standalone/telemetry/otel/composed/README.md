# Independent composed consumer

Local replaces ../../../../../telemetry/otel and ../../../../../client point only at the used modules. Run GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly go run . and go test -race ./.... Assembly uses the provider factory and SDK recorder. The observer subpackage imports only client and OTel API trace: no SDK or provider-factory dependency. It demonstrates typed partial results, error identity and parenting, with in-memory I/O and no globals/services. This is not a public client/otel wrapper.
