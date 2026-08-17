MODULE_NAME := $(shell basename $(shell git rev-parse --show-toplevel 2>/dev/null || pwd))

.PHONY: all init clean test lint lint-arch

all: init test

init:
	@chmod +x init.sh && ./init.sh

clean:
	go clean -testcache

test:
	go test ./... -v

lint:
	golangci-lint run ./...

COVERAGE_THRESHOLD := 80

# Packages with meaningful logic in the root module.
# Sub-modules (aws/, database/*, messaging/) are tested independently.
CRITICAL_PKGS := \
    ./pkg/app/router/... \
    ./pkg/app/build/... \
    ./pkg/health/... \
    ./pkg/utilities/resilience/... \
    ./pkg/utilities/error_handler/... \
    ./pkg/utilities/retry_backoff/...
# Note: ./pkg/app (root) is excluded — service.go initializes real AWS clients
# that require live infrastructure. Those are covered by integration tests.

.PHONY: coverage coverage-check coverage-module

## coverage: generates coverage.out and coverage.html for the root module
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

# lint-arch enforces the dependency-inversion boundary that makes the library
# modular: the core must not know about any adapter.
#
#   pkg/engine/**  ->  must not import provider/**, preset/**, or any SDK
#   pkg/**         ->  must not import gorm.io/gorm (confined to database/sql)
#
# Without this gate the coupling grows back one import at a time, which is
# exactly how a YAML reader ended up pulling in forty modules.
lint-arch:
	@echo "==> Checking architectural constraints..."
	@FAILED=0; \
	if grep -rn '"gorm.io/gorm"' pkg/ 2>/dev/null; then \
		echo ""; \
		echo "VIOLATION: gorm.io/gorm imported in pkg/"; \
		echo "GORM must only be used in the database/sql sub-package."; \
		FAILED=1; \
	fi; \
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
	for forbidden in \
		'github.com/aws/aws-sdk-go-v2' \
		'go.mongodb.org' \
		'github.com/redis/go-redis' \
		'github.com/segmentio/kafka-go' \
		'github.com/rabbitmq/amqp091-go' \
		'github.com/bradfitz/gomemcache' \
		'go.opentelemetry.io/otel/sdk' \
		'gorm.io/gorm'; do \
		if grep -rn "\"$$forbidden" pkg/engine/ 2>/dev/null | grep -v '_test\.go'; then \
			echo ""; \
			echo "VIOLATION: pkg/engine imports $$forbidden"; \
			echo "The core carries no adapter SDK. Put it behind an engine.Provider."; \
			FAILED=1; \
		fi; \
	done; \
	[ $$FAILED -eq 0 ] || (echo ""; echo "FAIL: architectural violations found"; exit 1)
	@echo "==> OK: no architectural violations found"

## lint-deps: reports how many external packages the core actually resolves
lint-deps:
	@echo "==> Core dependency footprint (pkg/engine):"
	@go list -deps ./pkg/engine/ | grep -E '^[a-z0-9.-]+\.[a-z]{2,}/' | wc -l | xargs echo "   external packages:"
	@go list -deps ./pkg/engine/ | grep -E '^[a-z0-9.-]+\.[a-z]{2,}/' | sed -E 's|^([^/]+/[^/]+).*|\1|' | sort -u | sed 's/^/   /'
