.PHONY: all build daemon client release release-daemon release-client proto clean install test

# Build flags for release builds
RELEASE_FLAGS = -ldflags="-s -w" -trimpath

# Default target
all: build

# Build both daemon and client (development)
build: daemon client

# Build daemon (development)
daemon:
	@echo "Building daemon..."
	@mkdir -p bin
	@go build -o bin/tts-daemon ./cmd/tts-daemon

# Build client (development)
client:
	@echo "Building client..."
	@mkdir -p bin
	@go build -o bin/tts-client ./cmd/tts-client

# Build both daemon and client (release/optimized)
release: release-daemon release-client

# Build daemon (release/optimized)
release-daemon:
	@echo "Building daemon (release mode)..."
	@mkdir -p bin
	@go build $(RELEASE_FLAGS) -o bin/tts-daemon ./cmd/tts-daemon

# Build client (release/optimized)
release-client:
	@echo "Building client (release mode)..."
	@mkdir -p bin
	@go build $(RELEASE_FLAGS) -o bin/tts-client ./cmd/tts-client

# Generate gRPC code from proto files
proto:
	@echo "Generating gRPC code..."
	@./generate.sh

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@go clean

# Install binaries to GOPATH/bin
install:
	@echo "Installing..."
	@go install ./cmd/tts-daemon
	@go install ./cmd/tts-client

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

# Run daemon (requires config file)
run-daemon:
	@./bin/tts-daemon

# Run client in help mode
run-client:
	@./bin/tts-client -h

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Vet code
vet:
	@echo "Vetting code..."
	@go vet ./...

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@golangci-lint run
