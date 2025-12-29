# Makefile for aws-ce plugin

.PHONY: build test clean lint help install

# Variables
PLUGIN_NAME = aws-ce
BINARY_NAME = pulumicost-plugin-$(PLUGIN_NAME)
BUILD_DIR = bin
CMD_DIR = cmd/plugin

# Default target
help:
	@echo "Available commands:"
	@echo "  build     - Build the plugin binary"
	@echo "  test      - Run tests"
	@echo "  clean     - Clean build artifacts"
	@echo "  lint      - Run linters"
	@echo "  install   - Install plugin to local registry"
	@echo "  help      - Show this help"

# Build the plugin binary
build:
	@echo "Building $(PLUGIN_NAME) plugin..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "✅ Plugin built: $(BUILD_DIR)/$(BINARY_NAME)"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
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
install: build
	@echo "Installing plugin to local registry..."
	@mkdir -p ~/.pulumicost/plugins/$(PLUGIN_NAME)/1.0.0
	@cp $(BUILD_DIR)/$(BINARY_NAME) ~/.pulumicost/plugins/$(PLUGIN_NAME)/1.0.0/
	@cp manifest.yaml ~/.pulumicost/plugins/$(PLUGIN_NAME)/1.0.0/plugin.manifest.json
	@echo "✅ Plugin installed to ~/.pulumicost/plugins/$(PLUGIN_NAME)/1.0.0/"

# Development build with debug info
build-debug:
	@echo "Building $(PLUGIN_NAME) plugin with debug info..."
	@mkdir -p $(BUILD_DIR)
	@go build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "✅ Debug build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✅ Code formatting complete"

# Update dependencies
deps:
	@echo "Updating dependencies..."
	@go mod tidy
	@go mod download
	@echo "✅ Dependencies updated"

# Ensure dependencies (alias for deps)
ensure: deps

# Check for security vulnerabilities
security:
	@echo "Checking for security vulnerabilities..."
	@govulncheck ./...
	@echo "✅ Security check complete"
