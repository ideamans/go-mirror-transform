# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Package Overview

This is `go-mirror-transform` (formerly `go-files-mirror`), a Go package for mirroring files from one directory to another while maintaining directory structure. The package supports pattern matching, concurrent processing, and file watching capabilities.

**Module name**: `github.com/ideamans/go-mirror-transform`

## Development Commands

### Running Tests
```bash
# Run all tests with race detection
make test

# Run tests with coverage report
make coverage

# Run a single test
go test -v -run TestCrawlBasic ./...

# Run tests for different concurrency levels (1, 2, 4)
go test -v -run TestCrawlConcurrency ./...
```

### Code Quality
```bash
# Run golangci-lint
make lint

# Format code
make fmt

# Check for vulnerabilities
make vuln
```

### Benchmarks
```bash
make bench
```

## Architecture Overview

### Core Components

1. **MirrorTransform Interface** (`mirrortransform.go`)
   - Main interface with `Crawl()` and `Watch()` methods
   - Configuration via `Config` struct passed by pointer to `NewMirrorTransform()`
   - Uses callback pattern for file processing and error handling

2. **Crawl Implementation** (`crawl.go`)
   - Two-level parallelism design:
     - Directory scanning goroutines that traverse the filesystem
     - File processor worker pool that handles matched files
   - Uses buffered channel (size 1000) for task communication
   - Implements circular reference prevention between input/output directories
   - Graceful shutdown via context cancellation

3. **Watch Implementation** (`watch.go`)
   - Uses `fsnotify` for file system event monitoring
   - Automatically watches new directories as they're created
   - Same file processor pool pattern as Crawl
   - Handles file creation, modification events (ignores remove/rename)

### Concurrency Model

The package uses a sophisticated concurrency model:
- Directory scanners and file processors run in separate goroutine pools
- This prevents imbalanced directory structures from causing inefficient parallelization
- Actual concurrency is `min(Config.Concurrency, Config.MaxConcurrency)`
- Default MaxConcurrency is `runtime.NumCPU()`

### Pattern Matching

Uses `github.com/bmatcuk/doublestar/v4` for minimatch-style glob patterns:
- `**/*.jpg` - matches all JPG files recursively
- `{*.jpg,*.png}` - matches multiple extensions
- ExcludePatterns can skip directories entirely (e.g., `node_modules/**`)

### Error Handling

Two types of callbacks:
- `FileCallback`: Returns `(continueProcessing bool, err error)`
- `ErrorCallback`: Returns `(stop bool, retErr error)`

Both support graceful degradation and selective error handling.

## CI/CD Configuration

GitHub Actions workflow (`.github/workflows/ci.yml`):
- **lint job**: Runs on Ubuntu with Go 1.23
- **tests job**: Matrix of [Windows, Linux, macOS] × [Go 1.22, Go 1.23]
- Triggers on push/PR to main and develop branches

## Important Notes

1. **Go Version Compatibility**: The package targets Go 1.22+ (uses traditional `for i := 0; i < n; i++` loops instead of `for range int` which requires Go 1.23)

2. **Import Path**: After renaming, use `github.com/ideamans/go-mirror-transform` 

3. **Config Pointer**: Always pass Config as a pointer to avoid large value copies:
   ```go
   mt, err := mirrortransform.NewMirrorTransform(&config)
   ```

4. **Context Usage**: Both Crawl and Watch methods respect context cancellation for graceful shutdown

5. **File System Safety**: 
   - Output directories are created automatically with 0o755 permissions
   - Circular references between input/output are detected and prevented
   - Path cleaning is applied to handle trailing slashes consistently

## GitHub Repository

- Repository: `ideamans/go-mirror-transform`
- Teams with push access: `next-gen-image`, `go-ai-managed`