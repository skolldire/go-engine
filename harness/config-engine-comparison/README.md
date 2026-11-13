# Config / legacy engine comparison harness (D43.3)

Nonpublished comparison, own go.mod and local replaces. It retains both TestModularConfig functions and BenchmarkConfigDecoderComparison from pkg/engine/config_modular_test.go. No build tag or workspace is needed. The private newDecoder call is replaced with the exported engine.NewRawConfig(...).Decode API; no private engine identifier is exported and no legacy production behavior changes. The public entry adds metadata/unknown-key handling to the benchmark, so its overhead is a public-API comparison rather than the previous private-decoder microbenchmark. Fixtures/assertions and documented conversion/environment differences are retained.

```sh
GOWORK=off go test -race ./...
GOWORK=off go test -run '^$' -bench . -benchmem -benchtime=100ms ./...
```

The root no longer imports unreleased config packages under a build tag, so root GOWORK=off go mod tidy stays unchanged. Existing check-config-engine/bench-config-engine aliases delegate here with GOWORK=off. This harness explicitly depends on the legacy root for comparison, not for use by new production modules. Root/Viper dependencies retain existing versions; no release version or track is approved.
