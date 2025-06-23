# go-files-mirror

A Go package for mirroring files from one directory to another while maintaining the directory structure. It supports pattern matching, concurrent processing, and file watching capabilities.

## Features

- **Pattern-based file selection** using glob patterns (minimatch style)
- **Concurrent processing** with configurable parallelism
- **Directory structure preservation** in the output directory
- **File watching** for real-time synchronization
- **Graceful shutdown** support with context cancellation
- **Circular reference prevention** to avoid infinite loops
- **Customizable error handling** with callbacks

## Installation

```bash
go get github.com/ideamans/go-files-mirror
```

## Usage

### Basic Example with Graceful Shutdown

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    mirrortransform "github.com/ideamans/go-mirror-transform"
)

func main() {
    config := mirrortransform.Config{
        InputDir:  "images",
        OutputDir: "output",
        Patterns:  []string{"**/*.jpg", "**/*.png", "**/*.gif"},
        Concurrency: 4,
        FileCallback: func(inputPath, outputPath string) (bool, error) {
            // Process the file (e.g., convert to WebP)
            // outputPath directory is guaranteed to exist
            log.Printf("Processing: %s -> %s\n", inputPath, outputPath)
            return true, nil
        },
    }

    mt, err := mirrortransform.NewMirrorTransform(&config)
    if err != nil {
        log.Fatal(err)
    }

    // Create context with cancellation
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle graceful shutdown on Ctrl+C
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigCh
        log.Println("Shutdown signal received, stopping gracefully...")
        cancel()
    }()

    // Run crawl with context
    if err := mt.Crawl(ctx); err != nil {
        if err == context.Canceled {
            log.Println("Crawl stopped gracefully")
        } else {
            log.Fatal(err)
        }
    }
}
```

### Watch Mode with Graceful Shutdown

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    mirrortransform "github.com/ideamans/go-mirror-transform"
)

func main() {
    config := mirrortransform.Config{
        InputDir:  "images",
        OutputDir: "output",
        Patterns:  []string{"**/*.jpg", "**/*.png", "**/*.gif"},
        Concurrency: 4,
        FileCallback: func(inputPath, outputPath string) (bool, error) {
            log.Printf("Processing: %s -> %s\n", inputPath, outputPath)
            // Process the file (e.g., convert to WebP)
            return true, nil
        },
        ErrorCallback: func(path string, err error) (bool, error) {
            log.Printf("Error at %s: %v\n", path, err)
            return false, nil // Continue processing
        },
    }

    mt, err := mirrortransform.NewMirrorTransform(&config)
    if err != nil {
        log.Fatal(err)
    }

    // Create context with cancellation
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigCh
        log.Println("Shutdown signal received, stopping watch...")
        cancel()
    }()

    log.Println("Watching for file changes. Press Ctrl+C to stop.")
    
    // Start watching (blocks until context is cancelled)
    if err := mt.Watch(ctx); err != nil {
        if err == context.Canceled {
            log.Println("Watch stopped gracefully")
        } else {
            log.Fatal(err)
        }
    }
}
```

### Combining Crawl and Watch with Timeout

```go
package main

import (
    "context"
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"
    mirrortransform "github.com/ideamans/go-mirror-transform"
)

func main() {
    var (
        watchMode bool
        timeout   time.Duration
    )
    flag.BoolVar(&watchMode, "watch", false, "Enable watch mode")
    flag.DurationVar(&timeout, "timeout", 60*time.Second, "Processing timeout")
    flag.Parse()

    config := mirrortransform.Config{
        InputDir:    "images",
        OutputDir:   "output",
        Patterns:    []string{"**/*.jpg", "**/*.png", "**/*.gif"},
        Concurrency: 4,
        FileCallback: func(inputPath, outputPath string) (bool, error) {
            log.Printf("Processing: %s\n", inputPath)
            // Your processing logic here
            return true, nil
        },
        ErrorCallback: func(path string, err error) (bool, error) {
            log.Printf("Error at %s: %v\n", path, err)
            return false, nil // Continue processing
        },
    }

    mt, err := mirrortransform.NewMirrorTransform(&config)
    if err != nil {
        log.Fatal(err)
    }

    // Create base context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    // Setup signal handling for graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    
    // Create a separate goroutine to handle signals
    go func() {
        select {
        case <-sigCh:
            log.Println("Interrupt signal received, shutting down...")
            cancel()
        case <-ctx.Done():
            // Context cancelled by timeout or other reason
        }
    }()

    // Start processing
    startTime := time.Now()
    if watchMode {
        log.Printf("Starting in watch mode. Will timeout in %v. Press Ctrl+C to stop.", timeout)
        err = mt.Watch(ctx)
    } else {
        log.Printf("Starting crawl. Will timeout in %v. Press Ctrl+C to stop.", timeout)
        err = mt.Crawl(ctx)
    }

    // Handle different types of context cancellation
    duration := time.Since(startTime)
    switch {
    case err == context.DeadlineExceeded:
        log.Printf("Operation timed out after %v", duration)
    case err == context.Canceled:
        log.Printf("Operation cancelled after %v", duration)
    case err != nil:
        log.Fatalf("Operation failed: %v", err)
    default:
        log.Printf("Operation completed successfully in %v", duration)
    }
}
```

## Configuration

### Config Fields

- `InputDir` (string, required): Root directory to scan for files
- `OutputDir` (string, required): Root directory for processed files
- `Patterns` ([]string, required): Glob patterns to match files (e.g., `**/*.jpg`)
- `ExcludePatterns` ([]string): Patterns for files/directories to exclude
- `Concurrency` (int): Desired number of parallel file processors
- `MaxConcurrency` (int): Maximum allowed concurrency (defaults to CPU count)
- `FileCallback` (func, required): Function called for each matching file
- `ErrorCallback` (func): Function called when errors occur during traversal

## Pattern Syntax

Patterns use minimatch-style glob syntax:
- `*` matches any characters except path separators
- `**` matches zero or more directories
- `?` matches any single character
- `[abc]` matches any character in the set
- `{a,b,c}` matches any of the alternatives

Examples:
- `**/*.jpg` - all JPG files in any subdirectory
- `images/**/*.{jpg,png}` - JPG and PNG files under images/
- `**/thumb_*.jpg` - JPG files starting with "thumb_"

## Concurrency

The package uses two levels of parallelism:
1. Directory scanning runs in parallel goroutines
2. File processing runs in a separate pool of workers

The actual concurrency is `min(Concurrency, MaxConcurrency)`. This design ensures efficient processing regardless of directory structure.

## Safety Features

- **Circular reference prevention**: Automatically detects and prevents processing when output directory is inside input directory
- **Graceful shutdown**: Waits for ongoing file operations to complete before exiting
- **Directory creation**: Automatically creates output directories as needed
- **Path cleaning**: Handles trailing slashes and path separators correctly

## License

MIT License