# Composed client/otel consumer

Application assembly uses real telemetry/otel with SDK SpanRecorder/ManualReader and resilience+retry. One logical middleware observation encloses three remote attempts; an uncertain publication is Reconcile without repetition. The two logical calls produce two histogram observations. No payload/error text is sent to tracing. Providers are closed explicitly by this assembler.

Local replaces ../../../../../{client,client/otel,resilience,retry,telemetry/otel} point only at used modules. SDK dependencies are declared as composed-only data in scripts/modules/consumer-deps.tsv; they do not grant the wrapper or minimal permission to import the SDK. The temporal test uses synctest, not real sleeps.

Run go run . and go test -race ./... with GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly after preparing the existing-version cache. No engine/framework/exporter/collector/network service is used.
