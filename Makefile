.PHONY: all build test clean run install fmt vet lint

# Binary name
BINARY_NAME=sonarqube-exporter

# Build the application
all: clean fmt vet test build

# Build the binary
build:
	@echo "Building..."
	go build -o bin/$(BINARY_NAME) cmd/exporter/main.go

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
coverage: test
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Run the application
run: build
	@echo "Running..."
	./bin/$(BINARY_NAME)

# Install dependencies
install:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Run golangci-lint (if installed)
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY_NAME)-linux-amd64 cmd/exporter/main.go
	GOOS=darwin GOARCH=amd64 go build -o bin/$(BINARY_NAME)-darwin-amd64 cmd/exporter/main.go
	GOOS=darwin GOARCH=arm64 go build -o bin/$(BINARY_NAME)-darwin-arm64 cmd/exporter/main.go
	GOOS=windows GOARCH=amd64 go build -o bin/$(BINARY_NAME)-windows-amd64.exe cmd/exporter/main.go

# Help command
help:
	@echo "Available targets:"
	@echo "  all         - Clean, format, vet, test, and build"
	@echo "  build       - Build the binary"
	@echo "  test        - Run tests"
	@echo "  coverage    - Run tests with coverage report"
	@echo "  clean       - Remove build artifacts"
	@echo "  run         - Build and run the application"
	@echo "  install     - Install dependencies"
	@echo "  fmt         - Format code"
	@echo "  vet         - Run go vet"
	@echo "  lint        - Run golangci-lint"
	@echo "  build-all   - Build for multiple platforms"
	@echo "  help        - Show this help message"
