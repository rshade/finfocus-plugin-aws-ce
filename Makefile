# Makefile for aws-ce plugin

.PHONY: help build test test-coverage clean lint install install-local build-debug fmt deps ensure develop vuln lint-md release-check test-integration docker

# Variables
PLUGIN_NAME = aws-ce
BINARY_NAME = finfocus-plugin-$(PLUGIN_NAME)
BUILD_DIR = bin
CMD_DIR = cmd/plugin
GO ?= go
FINFOCUS_HOME ?= $(HOME)/.finfocus
PLUGIN_VERSION ?= $(shell sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' manifest.json)
INSTALL_DIR = $(FINFOCUS_HOME)/plugins/$(PLUGIN_NAME)/$(PLUGIN_VERSION)

# Default target
help:
	@echo "Available commands:"
	@echo "  build     - Build the plugin binary"
	@echo "  test      - Run tests"
	@echo "  clean     - Clean build artifacts"
	@echo "  lint      - Run linters"
	@echo "  install   - Install plugin to local registry"
	@echo "  develop   - Fetch Go dependencies and prepare the build directory"
	@echo "  test-integration - Run subprocess and protocol conformance tests"
	@echo "  install-local - Install using the manifest version and FINFOCUS_HOME"
	@echo "  docker    - Blocked on the CE-3.2 owner decision"
	@echo "  help      - Show this help"

# Build the plugin binary
build:
	@echo "Building $(PLUGIN_NAME) plugin..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "✅ Plugin built: $(BUILD_DIR)/$(BINARY_NAME)"

# Run tests
test:
	@echo "Running tests..."
	@$(GO) test -count=1 -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@$(GO) test -count=1 -coverprofile=coverage.out ./...
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "✅ Clean complete"

# Run linters
lint:
	@echo "Running linters..."
	@golangci-lint run --allow-parallel-runners
	@echo "✅ Linting complete"

# Install plugin to local registry
install: install-local

install-local: build
	@test -n "$(PLUGIN_VERSION)" || { echo "manifest.json must supply a plugin version" >&2; exit 1; }
	@mkdir -p "$(INSTALL_DIR)"
	@cp "$(BUILD_DIR)/$(BINARY_NAME)" "$(INSTALL_DIR)/"
	@echo "Plugin installed to $(INSTALL_DIR)/"

# Docker packaging is excluded until the owner resolves CE-3.2.
docker:
	@echo "CE-3.2: Docker image builds need an owner decision; releases publish archives only." >&2
	@exit 1

# Run real process and protocol integration tests.
test-integration:
	@$(GO) test -count=1 -v ./test/integration/... ./test/conformance/...

# Prepare local development dependencies and build output.
develop: deps
	@mkdir -p "$(BUILD_DIR)"

# Development build with debug info
build-debug:
	@echo "Building $(PLUGIN_NAME) plugin with debug info..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "✅ Debug build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Format code
fmt:
	@echo "Formatting code..."
	@$(GO) fmt ./...
	@echo "✅ Code formatting complete"

# Update dependencies
deps:
	@echo "Updating dependencies..."
	@$(GO) mod tidy
	@$(GO) mod download
	@echo "✅ Dependencies updated"

# Ensure dependencies (alias for deps)
ensure: deps

# Check for security vulnerabilities
vuln:
	@echo "Checking for security vulnerabilities..."
	@govulncheck ./...
	@echo "✅ Vulnerability check complete"

# Lint markdown files
lint-md:
	@echo "Linting markdown..."
	@markdownlint-cli2 "**/*.md"
	@echo "✅ Markdown linting complete"

# Release checks (goreleaser validation)
release-check:
	@echo "Checking goreleaser configuration..."
	@goreleaser check
	@echo "✅ Release configuration valid"
