## What this PR does

<!-- One or two sentences. What problem it solves or what it adds. -->

Closes #<!-- issue number -->

## Type of change

<!-- Mark with x all that apply -->

- [ ] `feat` — new functionality
- [ ] `fix` — bug correction
- [ ] `refactor` — internal change with no new behavior
- [ ] `docs` — documentation only
- [ ] `test` — tests only
- [ ] `chore` — dependencies, CI, configuration
- [ ] `perf` — performance improvement

## Breaking changes

<!-- Describe changes to public interfaces, method signatures, or observable behavior. Delete if not applicable. -->

**BREAKING:**

## Key changes

<!-- List the most relevant files or packages and what changed in each. -->

- `pkg/...` (core module):
- `aws/...`, `messaging/...`, `http/...`, `database/...` (family modules):

## How to test

<!-- Concrete steps to verify the change. Include the test command if relevant. -->

```bash
make test                                  # every module, with -race
go test ./pkg/... -v -run TestNameOfTest   # one package in the core module
```

## Checklist

- [ ] `go build ./...` passes in every affected module
- [ ] `make test` passes — coverage ≥ 85% in modified packages
- [ ] `make lint` with no new warnings
- [ ] `make lint-arch` passes (the core resolves no adapter)
- [ ] `make tidy` run if any dependency changed
- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] README updated if the public API changed (builder, getters, YAML config)
- [ ] No secrets, tokens, or credentials in the diff
- [ ] Reviewers identified
