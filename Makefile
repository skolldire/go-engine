MODULE_NAME := $(shell basename $(shell git rev-parse --show-toplevel 2>/dev/null || pwd))

# Every module in the repository, in dependency order: the core first, then the
# adapter families, then the preset that composes all of them. `go.work` binds
# them together for local development, so a change to the core is visible to
# every family without a tagged release in between.
MODULES := . aws messaging http \
    database/sql database/redis database/mongodb database/memcached \
    preset/full

# Modules that ship an adapter family. The core is deliberately absent: its
# whole point is that it resolves none of their dependencies.
FAMILY_MODULES := aws messaging http \
    database/sql database/redis database/mongodb database/memcached

.PHONY: all init clean test lint lint-arch lint-deps tidy

all: init test

init:
	@chmod +x init.sh && ./init.sh

clean:
	@for m in $(MODULES); do (cd $$m && go clean -testcache); done

## test: runs the suite of every module with the race detector
test:
	@FAILED=0; \
	for m in $(MODULES); do \
		echo "==> test $$m"; \
		(cd $$m && go test -race -count=1 ./...) || FAILED=1; \
	done; \
	[ $$FAILED -eq 0 ] || (echo ""; echo "FAIL: tests failed in one or more modules"; exit 1)

## lint: golangci-lint over every module, all sharing the root configuration
lint:
	@FAILED=0; \
	root=$$(pwd); \
	for m in $(MODULES); do \
		echo "==> lint $$m"; \
		(cd $$m && golangci-lint run --config $$root/.golangci.yml ./...) || FAILED=1; \
	done; \
	[ $$FAILED -eq 0 ] || (echo ""; echo "FAIL: lint failed in one or more modules"; exit 1)

## tidy: go mod tidy in every module, then re-sync the workspace
tidy:
	@for m in $(MODULES); do echo "==> tidy $$m"; (cd $$m && GOWORK=off go mod tidy); done
	@go work sync

COVERAGE_THRESHOLD := 80

# Packages with meaningful logic in the core module. The adapter families are
# covered by their own suites, which `make test` runs.
#
# it is tracked separately; gating on it today would only mean disabling the gate.
CRITICAL_PKGS := \
    ./pkg/engine/... \
    ./pkg/router/... \
    ./pkg/health/... \
    ./pkg/utilities/resilience/... \
    ./pkg/utilities/error_handler/... \
    ./pkg/utilities/retry_backoff/...

.PHONY: coverage coverage-check coverage-module

## coverage: generates coverage.out and coverage.html for the core module
coverage:
	@echo "==> Generating coverage report..."
	@go test ./... -coverprofile=coverage.out -covermode=atomic 2>/dev/null || true
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1
	@echo "==> HTML report: coverage.html"

## coverage-check: fails if any critical package is below COVERAGE_THRESHOLD
coverage-check:
	@echo "==> Checking minimum coverage ($(COVERAGE_THRESHOLD)%)..."
	@FAILED=0; \
	for pkg in $(CRITICAL_PKGS); do \
		outfile=$$(echo $$pkg | tr '/.' '__' | tr -d '*').out; \
		if ! go test $$pkg -coverprofile=$$outfile -covermode=atomic -count=1; then \
			printf "FAIL  %-50s (tests failed)\n" "$$pkg"; \
			FAILED=1; \
			rm -f $$outfile; \
			continue; \
		fi; \
		if [ -f "$$outfile" ]; then \
			pct=$$(go tool cover -func=$$outfile 2>/dev/null | tail -1 | awk '{gsub(/%/,""); print int($$3)}'); \
			if [ "$${pct:-0}" -lt "$(COVERAGE_THRESHOLD)" ]; then \
				printf "FAIL  %-50s %d%%\n" "$$pkg" "$$pct"; \
				FAILED=1; \
			else \
				printf "OK    %-50s %d%%\n" "$$pkg" "$$pct"; \
			fi; \
			rm -f $$outfile; \
		fi; \
	done; \
	[ $$FAILED -eq 0 ] || (echo ""; echo "FAIL: one or more packages are below $(COVERAGE_THRESHOLD)%"; exit 1)
	@echo "==> All critical packages pass $(COVERAGE_THRESHOLD)%"

## coverage-module: coverage for a specific package
## Usage: make coverage-module PKG=pkg/health
coverage-module:
	@[ -n "$(PKG)" ] || (echo "Usage: make coverage-module PKG=pkg/health"; exit 1)
	@echo "==> Coverage for ./$(PKG)/..."
	@go test ./$(PKG)/... -coverprofile=coverage_module.out -covermode=atomic -count=1
	@go tool cover -func=coverage_module.out | tail -5
	@go tool cover -html=coverage_module.out -o coverage_module.html
	@echo "==> Report: coverage_module.html"
	@rm -f coverage_module.out

# lint-arch enforces the boundary that makes the library modular.
#
# Most of it is now structural: the core is its own module, so it *cannot*
# import an adapter without a require line appearing in its go.mod. That is what
# the first check reads — the module manifest, not the source. The remaining
# checks catch the two ways the boundary can still be crossed inside the core
# module: pkg/engine reaching for a provider or a preset that lives beside it.
#
# Without this gate the coupling grows back one import at a time, which is
# exactly how a YAML reader ended up pulling in forty modules.
# google.golang.org/grpc is absent from this list on purpose: pkg/telemetry/otel
# ships the OTLP/gRPC exporter, and it is part of the core by design. Package
# pruning keeps it off the build of anyone who does not import it.
FORBIDDEN_IN_CORE := \
    github.com/aws/aws-sdk-go-v2 \
    go.mongodb.org \
    github.com/redis/go-redis \
    github.com/segmentio/kafka-go \
    github.com/rabbitmq/amqp091-go \
    github.com/bradfitz/gomemcache \
    gorm.io/gorm

## check-modules: fails if an intra-repository require would break a consumer
check-modules:
	@./scripts/check-module-versions.sh

## smoke: builds a throwaway consumer outside the repo with GOWORK=off
## Usage: make smoke            (working tree, import graph only)
##        make smoke VERSION=v0.30.0   (published tags, end to end)
smoke:
	@./scripts/smoke-external-consumer.sh $(VERSION)

lint-arch:
	@echo "==> Checking architectural constraints..."
	@FAILED=0; \
	for forbidden in $(FORBIDDEN_IN_CORE); do \
		if go list -deps ./... 2>/dev/null | grep -q "^$$forbidden"; then \
			echo ""; \
			echo "VIOLATION: the core module resolves $$forbidden"; \
			echo "An adapter SDK belongs to its family module, behind an engine.Provider."; \
			FAILED=1; \
		fi; \
	done; \
	for family in $(FAMILY_MODULES); do \
		if grep -q "github.com/skolldire/go-engine/$$family " go.mod 2>/dev/null; then \
			echo ""; \
			echo "VIOLATION: the core module depends on the $$family module"; \
			echo "The dependency runs family -> core, never the other way round."; \
			FAILED=1; \
		fi; \
	done; \
	if grep -rn 'skolldire/go-engine/provider/' pkg/engine/ 2>/dev/null | grep -v '_test\.go'; then \
		echo ""; \
		echo "VIOLATION: pkg/engine imports a provider."; \
		echo "The core must not know any adapter; invert the dependency with engine.Provider."; \
		FAILED=1; \
	fi; \
	if grep -rn 'skolldire/go-engine/preset/' pkg/engine/ 2>/dev/null | grep -v '_test\.go'; then \
		echo ""; \
		echo "VIOLATION: pkg/engine imports a preset."; \
		FAILED=1; \
	fi; \
	if grep -rn '"go.opentelemetry.io/otel/sdk' pkg/engine/ 2>/dev/null | grep -v '_test\.go'; then \
		echo ""; \
		echo "VIOLATION: pkg/engine imports the OTel SDK."; \
		echo "It belongs behind provider/otel."; \
		FAILED=1; \
	fi; \
	[ $$FAILED -eq 0 ] || (echo ""; echo "FAIL: architectural violations found"; exit 1)
	@echo "==> OK: no architectural violations found"

## lint-deps: reports the external dependency footprint of every module
lint-deps:
	@printf "%-26s %10s %10s\n" "MODULE" "PACKAGES" "MODULES"
	@for m in $(MODULES); do \
		deps=$$(cd $$m && go list -deps ./... 2>/dev/null | grep -E '^[a-z0-9.-]+\.[a-z]{2,}/' | grep -v '^github.com/skolldire/go-engine'); \
		pkgs=$$(echo "$$deps" | grep -c .); \
		mods=$$(echo "$$deps" | sed -E 's|^([^/]+/[^/]+).*|\1|' | sort -u | grep -c .); \
		printf "%-26s %10s %10s\n" "$$m" "$$pkgs" "$$mods"; \
	done
	@echo ""
	@echo "==> Core (pkg/engine) resolves:"
	@go list -deps ./pkg/engine/ | grep -E '^[a-z0-9.-]+\.[a-z]{2,}/' | grep -v '^github.com/skolldire/go-engine' \
		| sed -E 's|^([^/]+/[^/]+).*|\1|' | sort -u | sed 's/^/   /'
