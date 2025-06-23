package mirrortransform

import (
	"context"
	"fmt"
	"path/filepath"
)

// FileCallback is called for each file that matches the pattern.
// inputPath is the full path of the source file.
// outputPath is the full path where the output should be written.
// The directory for outputPath is guaranteed to exist.
// If continueProcessing is false, the crawl will stop.
type FileCallback func(inputPath, outputPath string) (continueProcessing bool, err error)

// ErrorCallback is called when an error occurs during directory traversal.
// path is the location where the error occurred.
// If stop is true, the crawl will stop.
// If err is non-nil, it will be wrapped and returned from Crawl.
type ErrorCallback func(path string, err error) (stop bool, retErr error)

// Config holds the configuration for MirrorTransform.
type Config struct {
	// InputDir is the root directory to scan for files.
	InputDir string

	// OutputDir is the root directory where processed files will be placed.
	OutputDir string

	// Patterns are glob patterns (minimatch style) to match files.
	// Example: []string{"**/*.jpg", "**/*.png"}
	Patterns []string

	// ExcludePatterns are glob patterns for files/directories to exclude.
	ExcludePatterns []string

	// Concurrency is the desired number of parallel file processors.
	// The actual concurrency will be min(Concurrency, MaxConcurrency).
	Concurrency int

	// MaxConcurrency is the maximum allowed concurrency.
	// Defaults to runtime.NumCPU() if not set.
	MaxConcurrency int

	// FileCallback is called for each matching file.
	FileCallback FileCallback

	// ErrorCallback is called when errors occur during traversal.
	// If nil, errors will cause Crawl to return immediately.
	ErrorCallback ErrorCallback
}

// MirrorTransform provides functionality to mirror files from one directory
// to another while maintaining the directory structure.
type MirrorTransform interface {
	// Crawl traverses the input directory and processes matching files.
	// It respects the context for cancellation.
	Crawl(ctx context.Context) error

	// Watch monitors the input directory for changes and processes new/modified files.
	// This method blocks until the context is cancelled.
	Watch(ctx context.Context) error
}

// mirrorTransform is the concrete implementation of MirrorTransform.
type mirrorTransform struct {
	config Config
}

// NewMirrorTransform creates a new MirrorTransform instance with the given configuration.
func NewMirrorTransform(config *Config) (MirrorTransform, error) {
	// Validate configuration
	if config.InputDir == "" {
		return nil, fmt.Errorf("input directory is required")
	}
	if config.OutputDir == "" {
		return nil, fmt.Errorf("output directory is required")
	}
	if len(config.Patterns) == 0 {
		return nil, fmt.Errorf("at least one pattern is required")
	}
	if config.FileCallback == nil {
		return nil, fmt.Errorf("file callback is required")
	}

	// Clean paths to ensure consistent handling
	config.InputDir = filepath.Clean(config.InputDir)
	config.OutputDir = filepath.Clean(config.OutputDir)

	return &mirrorTransform{
		config: *config,
	}, nil
}
