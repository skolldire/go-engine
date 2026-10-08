# Example service

A microservice built with `go-engine`. It is an independent module — it is not
published, and the library does not depend on it.

It exists for two reasons that pull in the same direction:

- **It is the worked example** of how to implement the library: how to load
  configuration, register adapters, wire layers, and shut down cleanly.
- **It is the check that the library still works when implemented.** A change
  can compile and pass every unit test in the library while making an
  application awkward or impossible to build. That shows up here first.

That second role is not hypothetical: writing these tests is what revealed the
engine exposed no way for an application to read its own configuration.

## Layout

```
cmd/api/main.go        the service: the canonical wiring example
config/                the configuration the service and its tests both read
internal/handler/      HTTP layer, depends on router.Service
internal/usecase/      application logic, depends on interfaces
internal/repository/   data access, depends on a narrow Cache interface
test/                  validation, split by what it needs
```

The dependency direction is the point: `repository` declares the slice of Redis
it needs as its own interface rather than importing the provider. That is what
lets the use cases be tested against a fake, and keeps adapter types out of the
domain.

## Running

```bash
go run ./cmd/api            # needs the services in config/application.yaml
SCOPE=test go run ./cmd/api # merges config/application-test.yaml on top
```

## Testing

The suite is split by what it needs, so the fast half runs everywhere:

```bash
go test ./...               # no Docker: layers, wiring, HTTP, configuration
go test -tags e2e ./... -v  # adds real Redis, Postgres, Mongo, Memcached, LocalStack
```

### Without Docker

| Test | Proves |
|---|---|
| `TestServiceConfigurationIsValid` | The file the service ships with actually loads, with real durations and every adapter section reaching its provider |
| `TestEnvironmentPlaceholders*` | `${VAR}` and `${VAR:-default}` resolve, decoded into the adapter's own `Config` |
| `TestEngineBuildsWithoutAdapters` | The core is usable alone; a section with no provider builds nothing |
| `TestService_*` | The service end to end over HTTP against a fake cache, including the status a caller mistake gets versus a dependency failure |

### With Docker

Containers are managed by [testcontainers-go](https://golang.testcontainers.org/),
so there is no compose file to start and stop. Without a running daemon these
tests **skip** rather than fail.

| Test | Proves |
|---|---|
| `TestEngine_Redis` | Connect, round-trip, key prefix, and health following the container as it stops |
| `TestEngine_Postgres` | The SQL provider with an injected driver, real queries, and the configured pool size |
| `TestEngine_MongoDB` / `TestEngine_Memcached` | The providers reach a real server |
| `TestEngine_AWS` | SQS and S3 against LocalStack, discovered from the file by the AWS preset |
| `TestEngine_LifecycleReleasesInLIFOOrder` | Components close in reverse construction order |
| `TestEngine_ConfigErrors` | A bare-number duration, an unknown key and a malformed section each fail the build with an actionable message |

The `e2e` tag keeps the container suite out of the loop a developer runs on
every save: it pulls images and takes minutes.
