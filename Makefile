.PHONY: all test coverage lint clean help

# Default target
all: test

# Run tests
test:
	go test -v -race -parallel 4 ./...

# Run tests with coverage
coverage:
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run --timeout=5m

# Run benchmarks
bench:
	go test -bench=. -benchmem ./...

# Clean build artifacts
clean:
	rm -f coverage.out coverage.html
	go clean -cache

# Format code
fmt:
	go fmt ./...
	go mod tidy

# Install dependencies
deps:
	go mod download
	go mod tidy

# Check for vulnerabilities
vuln:
	@which govulncheck > /dev/null || (echo "Installing govulncheck..." && go install golang.org/x/vuln/cmd/govulncheck@latest)
	govulncheck ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  test      - Run tests"
	@echo "  coverage  - Run tests with coverage report"
	@echo "  lint      - Run linter"
	@echo "  bench     - Run benchmarks"
	@echo "  clean     - Clean build artifacts"
	@echo "  fmt       - Format code"
	@echo "  deps      - Install dependencies"
	@echo "  vuln      - Check for vulnerabilities"
	@echo "  help      - Show this help message"