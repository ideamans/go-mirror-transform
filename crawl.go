package filesmirror

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
func (fm *filesMirror) Crawl(ctx context.Context) error {
	// Check for circular references
	if err := fm.checkCircularReference(); err != nil {
		return err
	}

	// Determine concurrency
	concurrency := fm.config.Concurrency
	maxConcurrency := fm.config.MaxConcurrency
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

	for range concurrency {
		wg.Add(1)
		go fm.fileProcessor(processorCtx, taskChan, errChan, &wg)
	}

	// Start directory scanner
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(taskChan)
		
		if err := fm.scanDirectory(ctx, taskChan, errChan); err != nil {
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
func (fm *filesMirror) checkCircularReference() error {
	inputAbs, err := filepath.Abs(fm.config.InputDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of input directory: %w", err)
	}

	outputAbs, err := filepath.Abs(fm.config.OutputDir)
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
func (fm *filesMirror) scanDirectory(ctx context.Context, taskChan chan<- fileTask, _ chan<- error) error {
	return filepath.Walk(fm.config.InputDir, func(path string, info os.FileInfo, err error) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Handle walk error
		if err != nil {
			if fm.config.ErrorCallback != nil {
				stop, retErr := fm.config.ErrorCallback(path, err)
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
		relPath, err := filepath.Rel(fm.config.InputDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q: %w", path, err)
		}

		// Check exclude patterns
		for _, pattern := range fm.config.ExcludePatterns {
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
		for _, pattern := range fm.config.Patterns {
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
		outputPath := filepath.Join(fm.config.OutputDir, relPath)

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
func (fm *filesMirror) fileProcessor(ctx context.Context, taskChan <-chan fileTask, errChan chan<- error, wg *sync.WaitGroup) {
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
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				select {
				case errChan <- fmt.Errorf("failed to create output directory %q: %w", outputDir, err):
				case <-ctx.Done():
				}
				return
			}

			// Call the file callback
			continueProcessing, err := fm.config.FileCallback(task.inputPath, task.outputPath)
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