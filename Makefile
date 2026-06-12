.PHONY: help build test test-unit test-e2e coverage coverage-html clean

GO ?= go
BIN_DIR := bin
E2E_CB_SERVICE := $(BIN_DIR)/e2e-cb-service
E2E_DEBOUNCE_SERVICE := $(BIN_DIR)/e2e-debounce-service
E2E_RETRY_SERVICE := $(BIN_DIR)/e2e-retry-service
E2E_THROTTLE_SERVICE := $(BIN_DIR)/e2e-throttle-service
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
PKG := ./pkg/...
E2E := ./e2e/...

help:
	@echo "Usage:"
	@echo "  make build          Build the e2e demo service binary"
	@echo "  make test-unit      Run unit tests"
	@echo "  make test-e2e       Run e2e tests"
	@echo "  make test           Run unit and e2e tests"
	@echo "  make coverage       Run unit tests and show coverage summary"
	@echo "  make coverage-html  Generate HTML coverage report for unit tests"
	@echo "  make clean          Remove build artifacts and coverage files"

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(E2E_CB_SERVICE) ./e2e/cb_service/main.go
	$(GO) build -o $(E2E_DEBOUNCE_SERVICE) ./e2e/debounce_service/main.go
	$(GO) build -o $(E2E_RETRY_SERVICE) ./e2e/retry_service/main.go
	$(GO) build -o $(E2E_THROTTLE_SERVICE) ./e2e/throttle_service/main.go

test-unit:
	$(GO) test -count=1 -v $(PKG)

test-e2e:
	$(GO) test -count=1 -v -timeout 60s $(E2E)

test: test-unit test-e2e

coverage:
	$(GO) test -count=1 -coverprofile=$(COVERAGE_FILE) $(PKG)
	$(GO) tool cover -func=$(COVERAGE_FILE)

coverage-html: coverage
	$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Wrote $(COVERAGE_HTML)"

clean:
	rm -rf $(BIN_DIR) $(COVERAGE_FILE) $(COVERAGE_HTML)
