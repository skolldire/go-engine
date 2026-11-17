# validation composed

`validation` with `router/errors`, both real modules, driven through `httptest` without a listener. A handler decodes a JSON body, validates it and, on failure, hands the error to a `router/errors` responder whose classifier turns `validation.Errors` into a 422 with the failing paths in `detail`.

`run` checks that a valid body is answered 201, that an invalid one is answered 422 naming `email`, `password` and `plan` with their rules, that neither the submitted password nor the rejected plan value is in the response, and that malformed JSON is a 400. `validation` knows nothing about HTTP; the mapping is the classifier in this file.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root: `scripts/check-module.sh validation consumers`.
