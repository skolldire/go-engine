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

- `pkg/...`:
- `pkg/app/builder.go`:

## How to test

<!-- Concrete steps to verify the change. Include the test command if relevant. -->

```bash
go test ./pkg/... -v -run TestNameOfTest
```

## Checklist

- [ ] `go build ./...` passes (all affected modules)
- [ ] `go test ./...` passes — coverage ≥ 85% in modified packages
- [ ] `golangci-lint run` with no new warnings
- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] README updated if the public API changed (builder, getters, YAML config)
- [ ] No secrets, tokens, or credentials in the diff
- [ ] Reviewers identified
