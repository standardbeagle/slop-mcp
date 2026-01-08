.PHONY: build build-mock test test-mock test-integration test-all clean install uninstall fmt lint

# Binary name
BINARY := slop-mcp
# Build directory
BUILD_DIR := ./build
# Install directory
INSTALL_DIR := $(HOME)/.local/bin
# Mock MCP directory
MOCK_MCP_DIR := ./internal/server/testdata/mock-mcp

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet
GOMOD := $(GOCMD) mod

# Build flags
LDFLAGS := -s -w
BUILD_FLAGS := -ldflags "$(LDFLAGS)"

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/slop-mcp

# Build with OAuth support (enables login to MCPs like Figma, Dart)
build-oauth:
	@echo "Building $(BINARY) with OAuth support..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(BUILD_FLAGS) -tags mcp_go_client_oauth -o $(BUILD_DIR)/$(BINARY) ./cmd/slop-mcp

# Build the mock MCP server for testing
build-mock:
	@echo "Building mock MCP server..."
	$(GOBUILD) -o $(MOCK_MCP_DIR)/mock-mcp $(MOCK_MCP_DIR)

# Run short tests (no integration)
test:
	@echo "Running unit tests..."
	$(GOTEST) -short -v ./...

# Run integration tests with mock MCP (fast, no network)
test-mock: build-mock
	@echo "Running integration tests with mock MCP..."
	USE_MOCK_MCP=1 $(GOTEST) -tags integration -v ./... -timeout 5m

# Run integration tests with real MCP (requires npx)
test-integration:
	@echo "Running integration tests with real MCP..."
	$(GOTEST) -tags integration -v ./... -timeout 15m

# Run all tests
test-all: test test-mock

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Lint code
lint:
	@echo "Linting code..."
	$(GOVET) ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(MOCK_MCP_DIR)/mock-mcp

# Install to ~/.local/bin using install command
install: build
	@echo "Installing $(BINARY) to $(INSTALL_DIR)..."
	install -D -m 755 $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

# Install with OAuth support
install-oauth: build-oauth
	@echo "Installing $(BINARY) (with OAuth) to $(INSTALL_DIR)..."
	install -D -m 755 $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) with OAuth support to $(INSTALL_DIR)/$(BINARY)"

# Uninstall from ~/.local/bin
uninstall:
	@echo "Removing $(BINARY) from $(INSTALL_DIR)..."
	@rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "Uninstalled $(BINARY)"

# Help
help:
	@echo "Available targets:"
	@echo "  build            - Build the binary"
	@echo "  build-oauth      - Build with OAuth support (for Figma, Dart, etc.)"
	@echo "  build-mock       - Build the mock MCP server for testing"
	@echo "  test             - Run unit tests (short mode)"
	@echo "  test-mock        - Run integration tests with mock MCP (fast)"
	@echo "  test-integration - Run integration tests with real MCP (slow)"
	@echo "  test-all         - Run unit + mock integration tests"
	@echo "  fmt              - Format code"
	@echo "  lint             - Lint code"
	@echo "  tidy             - Tidy go.mod dependencies"
	@echo "  clean            - Remove build artifacts"
	@echo "  install          - Build and install to ~/.local/bin"
	@echo "  install-oauth    - Build with OAuth and install to ~/.local/bin"
	@echo "  uninstall        - Remove from ~/.local/bin"
	@echo "  help             - Show this help"
