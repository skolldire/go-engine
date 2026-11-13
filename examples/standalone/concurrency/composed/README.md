# concurrency/composed

Independent candidate consumer; only stdlib and concurrency. Its sole replace
is ../../../../concurrency, the local module under review, never the root.
This is local evidence E4, not the tag-real evidence E7.

From this directory, with Go 1.27.4 in PATH:

```sh
GOWORK=off GOPROXY=off GOTOOLCHAIN=local go run -mod=readonly .
GOWORK=off GOPROXY=off GOTOOLCHAIN=local go test -race -mod=readonly ./...
```

Full dependency allowlist checks: from repository root, make check-concurrency.
