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
		go test $$pkg -coverprofile=$$outfile -covermode=atomic -count=1 2>/dev/null || true; \
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

# lint-arch enforces the hexagonal architecture boundary:
# gorm.io/gorm must NOT be imported in the root module's pkg/ packages.
# GORM is confined to the database/sql sub-module; consumers inject a
# *gorm.DB (or gormsql.DBClient) via app.NewAppBuilder().WithCustomClient().
lint-arch:
	@echo "==> Checking architectural constraints..."
	@if grep -rn '"gorm.io/gorm"' pkg/ 2>/dev/null; then \
		echo ""; \
		echo "VIOLATION: gorm.io/gorm imported in pkg/"; \
		echo "GORM must only be used in the database/sql sub-module."; \
		echo "Inject a *gormsql.DBClient via WithCustomClient instead."; \
		exit 1; \
	fi
	@echo "==> OK: no architectural violations found"
