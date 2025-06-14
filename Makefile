# GoRL - Go Rate Limiter Makefile

# Variables
BINARY_NAME=gorl
MAIN_PATH=./cmd/gorl
BUILD_DIR=./build
DOCKER_IMAGE=gorl
DOCKER_TAG=latest
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# Go related variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Default target
.PHONY: all
all: clean deps test build

# Help target
.PHONY: help
help: ## Show this help message
	@echo "GoRL - Go Rate Limiter"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Development targets
.PHONY: deps
deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy

.PHONY: build
build: ## Build the binary
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

.PHONY: build-all
build-all: ## Build for all platforms
	mkdir -p $(BUILD_DIR)
	# Linux
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	# macOS
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	# Windows
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

.PHONY: install
install: ## Install the binary to $GOPATH/bin
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) $(MAIN_PATH)

.PHONY: run
run: ## Run the application with default settings
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH) && \
	$(BUILD_DIR)/$(BINARY_NAME) -help

.PHONY: run-example
run-example: build ## Run with example configuration
	$(BUILD_DIR)/$(BINARY_NAME) -url=https://httpbin.org/get -rate=5 -duration=10s -compact

.PHONY: run-live
run-live: build ## Run with live statistics
	$(BUILD_DIR)/$(BINARY_NAME) -url=https://httpbin.org/get -rate=3 -duration=15s -live

# Testing targets
.PHONY: test
test: ## Run all tests
	$(GOTEST) -v -timeout=60s ./test/...

.PHONY: test-unit
test-unit: ## Run unit tests only
	$(GOTEST) -v -timeout=30s -run="^Test.*" ./test/config_test.go ./test/stats_test.go ./test/limiter_test.go ./test/main_test.go ./test/custom_algorithm_test.go

.PHONY: test-integration
test-integration: ## Run integration tests only
	$(GOTEST) -v -timeout=120s -run="^TestIntegration.*|^TestEndToEnd.*|^TestComponent.*|^TestAllAlgorithms.*|^TestHighConcurrency.*|^TestError.*|^TestStats.*|^TestConfig.*Integration|^TestEnvironment.*" ./test/integration_test.go

.PHONY: test-tester
test-tester: ## Run tester package tests
	$(GOTEST) -v -timeout=60s ./test/tester_test.go

.PHONY: test-main
test-main: ## Run main package tests
	$(GOTEST) -v -timeout=30s ./test/main_test.go

.PHONY: test-custom
test-custom: ## Run custom algorithm tests
	$(GOTEST) -v -timeout=30s ./test/custom_algorithm_test.go

.PHONY: test-short
test-short: ## Run tests with short flag (skip long running tests)
	$(GOTEST) -v -short -timeout=30s ./test/...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	$(GOTEST) -v -timeout=60s -coverprofile=coverage.out ./test/...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: test-coverage-detailed
test-coverage-detailed: ## Run tests with detailed coverage per package
	@echo "Running coverage for all packages..."
	@for pkg in config limiter stats tester main custom_algorithm; do \
		echo "Testing $$pkg..."; \
		$(GOTEST) -v -timeout=30s -coverprofile=coverage-$$pkg.out ./test/$${pkg}_test.go; \
		if [ -f coverage-$$pkg.out ]; then \
			$(GOCMD) tool cover -html=coverage-$$pkg.out -o coverage-$$pkg.html; \
			$(GOCMD) tool cover -func=coverage-$$pkg.out | tail -1; \
		fi; \
	done
	@echo "Integration test coverage..."
	$(GOTEST) -v -timeout=120s -coverprofile=coverage-integration.out ./test/integration_test.go
	$(GOCMD) tool cover -html=coverage-integration.out -o coverage-integration.html
	$(GOCMD) tool cover -func=coverage-integration.out | tail -1

.PHONY: test-race
test-race: ## Run tests with race detection
	$(GOTEST) -v -timeout=90s -race ./test/...

.PHONY: test-timeout
test-timeout: ## Run tests with timeout
	$(GOTEST) -v -timeout=30s ./test/...

.PHONY: bench
bench: ## Run benchmarks
	$(GOTEST) -bench=. -benchmem -timeout=120s ./test/...

.PHONY: bench-limiter
bench-limiter: ## Run benchmarks for rate limiters only
	$(GOTEST) -bench=BenchmarkTokenBucket -benchmem -timeout=60s ./test/limiter_test.go
	$(GOTEST) -bench=BenchmarkLeakyBucket -benchmem -timeout=60s ./test/limiter_test.go
	$(GOTEST) -bench=BenchmarkFixedWindow -benchmem -timeout=60s ./test/limiter_test.go
	$(GOTEST) -bench=BenchmarkSlidingWindow -benchmem -timeout=60s ./test/limiter_test.go

.PHONY: bench-compare
bench-compare: ## Compare benchmark results across algorithms
	$(GOTEST) -bench=. -benchmem -timeout=120s ./test/limiter_test.go > bench-results.txt
	@echo "Benchmark results saved to bench-results.txt"

.PHONY: test-verbose
test-verbose: ## Run tests with verbose output and detailed logs
	$(GOTEST) -v -timeout=60s -count=1 ./test/... -args -test.v

.PHONY: test-profile
test-profile: ## Run tests with CPU profiling
	$(GOTEST) -v -timeout=120s -cpuprofile=cpu.prof -memprofile=mem.prof ./test/...
	@echo "CPU profile: cpu.prof, Memory profile: mem.prof"

# Linting and code quality targets
.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with autofix
	golangci-lint run --fix

.PHONY: fmt
fmt: ## Format code with gofumpt
	gofumpt -l -w .

.PHONY: imports
imports: ## Fix imports with goimports
	goimports -local github.com/rRateLimit/gorl -w .

.PHONY: vet
vet: ## Run go vet
	$(GOCMD) vet ./...

.PHONY: tidy
tidy: ## Tidy dependencies
	$(GOCMD) mod tidy
	$(GOCMD) mod verify

.PHONY: check
check: fmt imports vet lint ## Run all code quality checks

# Security and vulnerability checks
.PHONY: sec
sec: ## Run security checks
	gosec ./...

.PHONY: vuln
vuln: ## Check for vulnerabilities
	govulncheck ./...

.PHONY: deps-update
deps-update: ## Update dependencies
	$(GOCMD) get -u ./...
	$(GOCMD) mod tidy

.PHONY: deps-check
deps-check: ## Check for dependency issues
	$(GOCMD) mod verify
	$(GOCMD) list -m -u all

# CI related targets
.PHONY: ci-setup
ci-setup: ## Install CI dependencies
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOCMD) install github.com/securecodewarrior/sast-scan/gosec@latest
	$(GOCMD) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GOCMD) install mvdan.cc/gofumpt@latest
	$(GOCMD) install golang.org/x/tools/cmd/goimports@latest

.PHONY: ci-test
ci-test: check test-coverage ## Run all CI checks and tests

.PHONY: pre-commit
pre-commit: check test-unit ## Run pre-commit checks

# Cleaning targets
.PHONY: clean
clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f *.out *.prof *.html bench-results.txt
	rm -f gorl-*

.PHONY: clean-all
clean-all: clean ## Clean everything including dependencies
	$(GOMOD) clean -cache

# Docker targets
.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

.PHONY: docker-run
docker-run: ## Run Docker container
	docker run --rm $(DOCKER_IMAGE):$(DOCKER_TAG) -help

.PHONY: docker-run-example
docker-run-example: ## Run Docker container with example
	docker run --rm $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-url=https://httpbin.org/get -rate=5 -duration=10s -compact

.PHONY: docker-push
docker-push: docker-build ## Push Docker image
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: docker-clean
docker-clean: ## Clean Docker images
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) || true

# Docker Compose targets
.PHONY: compose-up
compose-up: ## Start services with docker-compose
	docker-compose up -d

.PHONY: compose-down
compose-down: ## Stop services with docker-compose
	docker-compose down

.PHONY: compose-logs
compose-logs: ## Show docker-compose logs
	docker-compose logs -f

# Development helpers
.PHONY: watch
watch: ## Watch for changes and rebuild (requires entr)
	find . -name '*.go' | entr -r make build

.PHONY: dev
dev: clean deps check build ## Full development cycle

# Release targets
.PHONY: release
release: clean deps test build-all ## Prepare release artifacts
	@echo "Release artifacts built in $(BUILD_DIR)/"
	@ls -la $(BUILD_DIR)/

# Configuration examples
.PHONY: examples
examples: ## Show configuration examples
	@echo "=== Configuration Examples ==="
	@echo ""
	@echo "1. Basic usage:"
	@echo "   ./$(BINARY_NAME) -url=https://httpbin.org/get -rate=5 -duration=30s"
	@echo ""
	@echo "2. With live statistics:"
	@echo "   ./$(BINARY_NAME) -url=https://httpbin.org/get -rate=10 -live"
	@echo ""
	@echo "3. Using different algorithms:"
	@echo "   ./$(BINARY_NAME) -url=https://api.example.com -rate=5 -algorithm=leaky-bucket"
	@echo ""
	@echo "4. With custom HTTP settings:"
	@echo "   ./$(BINARY_NAME) -url=https://api.example.com -rate=10 -http-timeout=10s -disable-keep-alives"
	@echo ""
	@echo "5. Using environment variables:"
	@echo "   export GORL_URL=https://httpbin.org/get"
	@echo "   export GORL_RATE=5"
	@echo "   ./$(BINARY_NAME)"
	@echo ""
	@echo "6. Using config file:"
	@echo "   ./$(BINARY_NAME) -config=config.example.json"

# Environment setup
.PHONY: setup
setup: ## Setup development environment
	@echo "Setting up development environment..."
	$(GOMOD) download
	@echo "Installing development tools..."
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Development environment ready!"

# Show project status
.PHONY: status
status: ## Show project status
	@echo "=== Project Status ==="
	@echo "Version: $(VERSION)"
	@echo "Go version: $$(go version)"
	@echo "Module: $$(head -1 go.mod)"
	@echo "Dependencies: $$(go list -m all | wc -l) modules"
	@echo "Test files: $$(find . -name '*_test.go' | wc -l) files"
	@echo "Build artifacts: $$(ls -1 $(BUILD_DIR) 2>/dev/null | wc -l) files"

## Test the application
test:
	@echo "Running tests..."
	@go test -v ./...

## Run benchmarks
bench:
	@echo "Running Go benchmarks..."
	@go test -bench=. -benchmem ./test/... -run=^Benchmark

## Run performance tests
perf-test:
	@echo "Running performance tests..."
	@go test -v ./test/... -run="Performance|HighLoad|Stress|Accuracy" -timeout=30m

## Run full benchmark suite
benchmark: build
	@echo "Running full benchmark suite..."
	@./scripts/benchmark.sh
