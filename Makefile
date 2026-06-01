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
