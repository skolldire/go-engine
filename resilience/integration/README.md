# Real resilience integration harness (D43.1)

Nonpublished harness, independent go.mod, GOWORK=off. This retains the original concrete-tool composition tests, stress, fuzz inputs and benchmarks while the resilience library depends only on client/retry. The tools are real circuitbreaker/concurrency/ratelimit instances; they are not reimplemented here.

| Original | Destination | Preservation |
|---|---|---|
| resilience/resilience_test.go | resilience/integration/resilience_test.go | all test functions retained; imports qualified and interface-held real values asserted only here for Stats/Allow observations |
| resilience/integration_test.go | resilience/integration/integration_test.go | same transport recovery cases |
| resilience/qualification_test.go | resilience/integration/qualification_test.go | all stress, fuzz and leak/retention cases retained |
| resilience/benchmark_test.go | resilience/integration/benchmark_test.go | all real-tool benchmark cases retained |
| resilience/testdata/fuzz/* | resilience/integration/testdata/fuzz/* | original corpus bytes preserved |

Only mandated rejection assertions change: bare ErrOpen/ErrProbeLimit identity checks become errors.Is checks for the backend cause AND resilience.ErrRejected. Operation-error identity checks remain strict. The composed consumer needs the same assertion correction because its rejection is intentionally now wrapped; actual tool construction and dependency assignments are unchanged. The module's new FuzzConfig/FuzzSequences/FuzzBudgets validate the interface-only boundary; this harness preserves the complete original cases/corpus too.

```sh
GOWORK=off go test -race ./...
RESILIENCE_STRESS=1 GOWORK=off go test -race -run '^TestStress' -count=3 ./...
GOWORK=off go test -run '^$' -bench . -benchtime=100ms -benchmem ./...
```

The generic checker uses the catalog row and a stress-env data override to preserve RESILIENCE_STRESS. No production dependency points to this harness. No tags/version are assigned.
