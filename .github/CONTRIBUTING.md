# Contributing

Thank you for your interest in improving **go-engine**. This guide covers everything you need to contribute effectively.

---

## Table of Contents

- [Before you start](#before-you-start)
- [Environment setup](#environment-setup)
- [Workflow](#workflow)
- [Code conventions](#code-conventions)
- [Tests](#tests)
- [Commits](#commits)
- [Pull requests](#pull-requests)
- [Types of contribution](#types-of-contribution)
- [FAQ](#faq)

---

## Before you start

- Check [open issues](https://github.com/skolldire/go-engine/issues) to avoid duplicates.
- For large changes (new package, public interface change), **open an issue first** and describe the problem or proposal. This avoids discarded work.
- For bug fixes and small improvements, you can open a PR directly.

---

## Environment setup

**Requirements:**

| Tool | Minimum version |
|---|---|
| Go | 1.21 (see `go.mod`) |
| Make | any |
| golangci-lint | v1.57+ |

```bash
# Clone
git clone https://github.com/skolldire/go-engine.git
cd go-engine

# Initialize (downloads tools and verifies setup)
make init

# Run all tests
make all

# Or manually
go test ./... -v
```

### Sub-modules

The repository has multiple Go modules. If your change touches `aws/`, `messaging/`, or `database/*`, enter the corresponding directory:

```bash
cd aws && go test ./...
cd messaging && go test ./...
cd database/redis && go test ./...
```

---

## Workflow

```
main/master
    └── feature/short-description    ← your branch
    └── fix/bug-description
    └── docs/what-it-documents
    └── refactor/what-it-refactors
```

1. Create a branch from `master`: `git checkout -b feature/health-kafka-checker`
2. Make atomic commits (see [Commits](#commits))
3. Run tests and linter before opening the PR
4. Open the PR against `master` using the [template](pull_request_template.md)

---

## Code conventions

### Package structure

Every package follows this strict convention:

```
pkg/my-package/
├── entity.go       # structs, interfaces, constants — no logic
├── service.go      # Service interface implementation
└── service_test.go # tests (mocks in mocks_test.go if extensive)
```

The public interface is always `Service`. The constructor is always `NewClient` or `NewService`.

### Style rules

- **No comments explaining what the code does** — names should be self-explanatory.
- Comment only the **why**: hidden constraints, non-obvious invariants, workarounds for external bugs.
- Do not add error handling for impossible scenarios. Trust framework and Go type guarantees.
- Do not introduce abstractions before having them in three places.
- `gofmt` and `golangci-lint` are requirements, not suggestions.

### Interfaces

- If you add a new client, define the interface in `entity.go` and verify the implementation satisfies it with `var _ Service = (*MyService)(nil)`.
- External clients (AWS SDK, Redis, etc.) are accessed through local interfaces (see `redisPinger`, `sqlPinger` in `pkg/health/checkers.go`) to enable testing without real infrastructure.

### Multi-module

- If your change requires a new external dependency, consider whether it belongs in the root module or a sub-module. AWS dependencies go in `aws/`, messaging in `messaging/`, databases in `database/*`.
- Do not add dependencies to the root module that are only needed in a sub-module.

---

## Tests

- Minimum acceptable coverage: **85%** per package.
- Use `testify/assert` and `testify/mock`.
- Mocks for external interfaces go in the same `_test.go` file where they are used, or in `mocks_test.go` if they are extensive.
- Do not mock the database or Redis if the test can use a real in-memory implementation. Mock the engine interfaces, not the full external clients.
- A test that passes with mocks but fails against the real implementation has no value.

```bash
# View coverage per function
go test ./pkg/health/... -coverprofile=cover.out && go tool cover -func=cover.out

# Clear cache (useful if tests use external files)
go clean -testcache && go test ./...
```

---

## Commits

We use [Conventional Commits](https://www.conventionalcommits.org/) with optional gitmoji:

```
<type>(<scope>): <short description in imperative form>

[optional body: what and why, not how]

[footer: BREAKING CHANGE: description | Closes #123]
```

**Allowed types:**

| Type | When to use |
|---|---|
| `feat` | New functionality |
| `fix` | Bug correction |
| `refactor` | Change that neither adds functionality nor fixes a bug |
| `test` | Tests only |
| `docs` | Documentation only |
| `chore` | Dependencies, CI, configuration |
| `perf` | Performance improvement |

**Examples:**

```
feat(health): add unified GET /health endpoint for ECS Fargate

fix(builder): prevent double health route mount when WithRouter called twice

docs(readme): add builder method reference table

BREAKING CHANGE: ServiceRegistry.Health type changed from health.Service to *health.HealthService
```

---

## Pull requests

- The PR must pass **all CI checks** (build, tests, lint) before review.
- One PR = one responsibility. Do not mix feat + refactor in the same PR if they are independent.
- Breaking changes must be documented in the PR body and in `CHANGELOG.md` under `[Unreleased]`.
- Update `CHANGELOG.md` in every PR that adds, changes, or removes observable behavior.
- If you change a public interface (`Service`, `Engine` methods, builder methods), update the README.

### Checklist before requesting review

```
[ ] go build ./... passes (all affected modules)
[ ] go test ./... passes with coverage ≥ 85% in modified packages
[ ] golangci-lint run passes with no new warnings
[ ] CHANGELOG.md updated under [Unreleased]
[ ] README updated if the public API changed (builder, getters, YAML config)
[ ] No secrets or credentials in the diff
[ ] Reviewers identified
```

---

## Types of contribution

### New client or integration

1. Create the directory `pkg/clients/my-client/` with `entity.go` and `service.go`.
2. Define the `Service` interface with the required methods.
3. Add the client to `ServiceRegistry` in `registry.go`.
4. Add the getter in the Engine's `entity.go`.
5. Add initialization in `service.go` (or in the corresponding sub-module).
6. Add tests with coverage ≥ 85%.
7. Document in the README (getter table + YAML config section if applicable).

### New health checker

Implement the `health.Checker` interface:

```go
type Checker interface {
    Check(ctx context.Context) error
}
```

Add the constructor in `pkg/health/checkers.go` and tests in `health_test.go`.

### Bug fix

- Reproduce the bug with a test that **fails before** your fix.
- The test must **pass after** the fix.
- Document in the commit the exact scenario that was failing.

### Documentation

- Fix inaccurate code examples — compile them if possible.
- The README is the source of truth for the public API; keeping it accurate is as important as the code.

---

## FAQ

**Can I add a new dependency to the root module?**
Only if it is absolutely necessary and has no alternative in the stdlib. Discuss it in the issue before adding it.

**How do I test a client that requires real AWS?**
Use local interfaces (see `redisPinger`, `sqlPinger`) so tests can use mocks. Integration tests against real AWS (LocalStack) can be added as tests with `//go:build integration`.

**What if CI fails on the linter but the code works?**
The linter is a requirement. Fix the warnings before requesting review — we do not use `//nolint` except in justified cases with a comment explaining why.

**Where does SQL/GORM go?**
SQL is not auto-initialized by the engine. Inject it via `WithCustomClient` and retrieve it with `GetCustomClient`. See the README for an example.
