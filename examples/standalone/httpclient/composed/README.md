# httpclient composed

The recommended composition, with real modules: `httpclient`, `client`, `resilience`, `retry`, `client/logging` and `logging`, against an `httptest` server on loopback. Local replaces point four levels up, one per module used.

Each logical call is one `client.Operation` that creates its request inside the operation, calls `Do`, applies `CheckStatus` and closes the body before returning. `resilience.Wrap` retries it with a classifier written here, by method and status; `client/logging` wraps the result so a call logs once however many attempts it took.

What `run` verifies:

- a `GET` answered 503 and then 200 is retried and succeeds;
- a `PUT` is retried and the second attempt carries the whole body again;
- a `POST` without an `Idempotency-Key` answered 503 is sent once and returns a `*httpclient.StatusError`;
- a `POST` with a key is retried, by explicit decision;
- a `POST` without a key answered 500 is not retried and comes back as `retry.ErrReconciliationRequired`;
- every call produced exactly one log record, with nothing the upstream wrote in it;
- all eight attempts shared one connection (`MaxConnsPerHost` is 1), which is only possible if every attempt closed its body.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root: `scripts/check-module.sh httpclient consumers`.
