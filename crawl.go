package mirrortransform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

// fileTask represents a file to be processed.
type fileTask struct {
	inputPath  string
	outputPath string
}

// Crawl traverses the input directory and processes matching files.
func (mt *mirrorTransform) Crawl(ctx context.Context) error {
	// Check for circular references
	if err := mt.checkCircularReference(); err != nil {
		return err
	}

	// Determine concurrency
	concurrency := mt.config.Concurrency
	maxConcurrency := mt.config.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = runtime.NumCPU()
	}
	if concurrency <= 0 || concurrency > maxConcurrency {
		concurrency = maxConcurrency
	}

	// Create channels for communication
	taskChan := make(chan fileTask, 1000) // Buffered channel for better performance
	errChan := make(chan error, 1)

	// WaitGroup to track all goroutines
	var wg sync.WaitGroup

	// Start file processors
	processorCtx, cancelProcessors := context.WithCancel(ctx)
	defer cancelProcessors()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go mt.fileProcessor(processorCtx, taskChan, errChan, &wg)
	}

	// Start directory scanner
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(taskChan)

		if err := mt.scanDirectory(ctx, taskChan, errChan); err != nil {
			select {
			case errChan <- err:
			case <-ctx.Done():
			}
		}
	}()

	// Wait for completion or error
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		// Context cancelled, wait for graceful shutdown
		cancelProcessors()
		<-done
		return ctx.Err()
	case err := <-errChan:
		// Error occurred, cancel and wait for shutdown
		cancelProcessors()
		<-done
		return err
	case <-done:
		// All work completed successfully
		return nil
	}
}

// checkCircularReference checks if input and output directories would create a circular reference.
func (mt *mirrorTransform) checkCircularReference() error {
	inputAbs, err := filepath.Abs(mt.config.InputDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of input directory: %w", err)
	}

	outputAbs, err := filepath.Abs(mt.config.OutputDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of output directory: %w", err)
	}

	// Normalize paths for comparison
	inputAbs = filepath.Clean(inputAbs)
	outputAbs = filepath.Clean(outputAbs)

	// Check if output is inside input
	if strings.HasPrefix(outputAbs, inputAbs+string(filepath.Separator)) || outputAbs == inputAbs {
		return fmt.Errorf("output directory %q is inside input directory %q, which would create a circular reference", outputAbs, inputAbs)
	}

	// Check if input is inside output (safety check)
	if strings.HasPrefix(inputAbs, outputAbs+string(filepath.Separator)) {
		return fmt.Errorf("input directory %q is inside output directory %q, which would create a circular reference", inputAbs, outputAbs)
	}

	return nil
}

// scanDirectory recursively scans the directory and sends matching files to the task channel.
func (mt *mirrorTransform) scanDirectory(ctx context.Context, taskChan chan<- fileTask, _ chan<- error) error {
	return filepath.Walk(mt.config.InputDir, func(path string, info os.FileInfo, err error) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Handle walk error
		if err != nil {
			if mt.config.ErrorCallback != nil {
				stop, retErr := mt.config.ErrorCallback(path, err)
				if retErr != nil {
					return fmt.Errorf("error callback failed at %q: %w", path, retErr)
				}
				if stop {
					return fmt.Errorf("stopped due to error at %q: %w", path, err)
				}
				// Continue processing
				return nil
			}
			return fmt.Errorf("failed to access %q: %w", path, err)
		}

		// Get relative path from input directory
		relPath, err := filepath.Rel(mt.config.InputDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q: %w", path, err)
		}

		// Check exclude patterns
		for _, pattern := range mt.config.ExcludePatterns {
			match, err := doublestar.Match(pattern, relPath)
			if err != nil {
				return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
			}
			if match {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Skip directories for pattern matching
		if info.IsDir() {
			return nil
		}

		// Check if file matches any pattern
		matched := false
		for _, pattern := range mt.config.Patterns {
			match, err := doublestar.Match(pattern, relPath)
			if err != nil {
				return fmt.Errorf("invalid pattern %q: %w", pattern, err)
			}
			if match {
				matched = true
				break
			}
		}

		if !matched {
			return nil
		}

		// Create output path
		outputPath := filepath.Join(mt.config.OutputDir, relPath)

		// Send task to channel
		select {
		case taskChan <- fileTask{inputPath: path, outputPath: outputPath}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

// fileProcessor processes files from the task channel.
func (mt *mirrorTransform) fileProcessor(ctx context.Context, taskChan <-chan fileTask, errChan chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-taskChan:
			if !ok {
				return
			}

			// Ensure output directory exists
			outputDir := filepath.Dir(task.outputPath)
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				select {
				case errChan <- fmt.Errorf("failed to create output directory %q: %w", outputDir, err):
				case <-ctx.Done():
				}
				return
			}

			// Call the file callback
			continueProcessing, err := mt.config.FileCallback(task.inputPath, task.outputPath)
			if err != nil {
				select {
				case errChan <- fmt.Errorf("file callback failed for %q: %w", task.inputPath, err):
				case <-ctx.Done():
				}
				return
			}

			if !continueProcessing {
				select {
				case errChan <- fmt.Errorf("processing stopped by callback at %q", task.inputPath):
				case <-ctx.Done():
				}
				return
			}
		}
	}
}
