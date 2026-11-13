# Candidate: concurrency/v1.0.0

Status: prepared candidate, **not tagged, not published, not accepted as stable**.
E6 (version manifest and user authorization) and E7 (real-tag consumer) are pending.
The module declares Go 1.27.4 and has no dependency requirements, even for tests.

Features: managed typed Pool/Job/Future; atomic bounded admission; keyed FIFO
with round-robin fairness; count/weight/key limits; panic containment; explicit
cooperative cancellation; drain/cancel shutdown; bounded streams and weighted
batches; shared weighted FIFO limiter. No retry engine or framework dependency.

Unpublished prototype signatures change as described in MIGRATION.md. Legacy
task_executor is deprecated/frozen and is not a compatibility wrapper. No priority
scheduler, distributed exclusion or financial consistency guarantee is introduced.

## Post-publication verification E7 (run only once authorized tag exists)

A real tag uses the directory prefix `concurrency/v1.0.0`; go get selects module
version `v1.0.0`, not the directory-prefixed tag as its version argument.

```sh
# Outside this repository, in a fresh temporary directory:
go mod init example.com/concurrency-tag-check
GOWORK=off go get github.com/skolldire/go-engine/concurrency@v1.0.0
# Copy main.go from the minimal consumer only, not its go.mod or local replace.
GOWORK=off go run -mod=readonly .
GOWORK=off go list -m -json all
GOWORK=off go mod graph
# Record go version, selected module version, module sums and successful output.
```

Repeat with composed/main.go in another fresh module. No replace directive,
workspace, root module or SDK is allowed. Run once using a fresh module cache
to establish actual tag resolution, then offline with GOPROXY=off to establish
reproducibility. Capture exact commit/tag and consumer source hashes. A failure
is a release incident; never move an immutable tag to hide it.

Do not modify the version workflow or create a manifest in this task. The owner
proposes v1.0.0, the user approves, release tooling establishes E6, then E7 is
performed against the published artifact. Until then report E7 pending.
