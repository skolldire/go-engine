MODULE_NAME := $(shell basename $(shell git rev-parse --show-toplevel 2>/dev/null || pwd))

.PHONY: all init clean test

all: init test

init:
	@chmod +x init.sh && ./init.sh

clean:
	go clean -testcache

test:
	go test ./... -v