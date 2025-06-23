package filesmirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
)

// Watch monitors the input directory for changes and processes new/modified files.
// This method blocks until the context is cancelled.
func (fm *filesMirror) Watch(ctx context.Context) error {
	// Check for circular references
	if err := fm.checkCircularReference(); err != nil {
		return err
	}

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Close()

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
	taskChan := make(chan fileTask, 1000)
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

	// Add directories to watch
	if err := fm.addWatchDirs(watcher); err != nil {
		return fmt.Errorf("failed to add watch directories: %w", err)
	}

	// Start event handler
	wg.Add(1)
	go func() {
		defer wg.Done()
		fm.handleWatchEvents(processorCtx, watcher, taskChan, errChan)
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
		// Should not happen as watch runs indefinitely
		return nil
	}
}

// addWatchDirs recursively adds directories to the watcher.
func (fm *filesMirror) addWatchDirs(watcher *fsnotify.Watcher) error {
	return filepath.Walk(fm.config.InputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if fm.config.ErrorCallback != nil {
				stop, retErr := fm.config.ErrorCallback(path, err)
				if retErr != nil {
					return fmt.Errorf("error callback failed at %q: %w", path, retErr)
				}
				if stop {
					return fmt.Errorf("stopped due to error at %q: %w", path, err)
				}
				return nil
			}
			return fmt.Errorf("failed to access %q: %w", path, err)
		}

		// Only watch directories
		if !info.IsDir() {
			return nil
		}

		// Get relative path from input directory
		relPath, err := filepath.Rel(fm.config.InputDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q: %w", path, err)
		}

		// Check exclude patterns for directories
		if relPath != "." {
			for _, pattern := range fm.config.ExcludePatterns {
				match, err := doublestar.Match(pattern, relPath)
				if err != nil {
					return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
				}
				if match {
					return filepath.SkipDir
				}
			}
		}

		// Add directory to watcher
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("failed to add watch for %q: %w", path, err)
		}

		return nil
	})
}

// handleWatchEvents handles file system events from the watcher.
func (fm *filesMirror) handleWatchEvents(ctx context.Context, watcher *fsnotify.Watcher, taskChan chan<- fileTask, errChan chan<- error) {
	for {
		select {
		case <-ctx.Done():
			close(taskChan)
			return

		case event, ok := <-watcher.Events:
			if !ok {
				close(taskChan)
				return
			}

			// Handle the event
			if err := fm.processWatchEvent(ctx, watcher, event, taskChan); err != nil {
				select {
				case errChan <- err:
				case <-ctx.Done():
				}
				close(taskChan)
				return
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				close(taskChan)
				return
			}

			if fm.config.ErrorCallback != nil {
				stop, retErr := fm.config.ErrorCallback("watcher", err)
				if retErr != nil {
					select {
					case errChan <- fmt.Errorf("error callback failed: %w", retErr):
					case <-ctx.Done():
					}
					close(taskChan)
					return
				}
				if stop {
					select {
					case errChan <- fmt.Errorf("stopped due to watcher error: %w", err):
					case <-ctx.Done():
					}
					close(taskChan)
					return
				}
			} else {
				select {
				case errChan <- fmt.Errorf("watcher error: %w", err):
				case <-ctx.Done():
				}
				close(taskChan)
				return
			}
		}
	}
}

// processWatchEvent processes a single file system event.
func (fm *filesMirror) processWatchEvent(ctx context.Context, watcher *fsnotify.Watcher, event fsnotify.Event, taskChan chan<- fileTask) error {
	// Ignore remove and rename events
	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		return nil
	}

	// Get file info
	info, err := os.Stat(event.Name)
	if err != nil {
		// File might have been deleted between event and stat
		if os.IsNotExist(err) {
			return nil
		}
		if fm.config.ErrorCallback != nil {
			stop, retErr := fm.config.ErrorCallback(event.Name, err)
			if retErr != nil {
				return fmt.Errorf("error callback failed at %q: %w", event.Name, retErr)
			}
			if stop {
				return fmt.Errorf("stopped due to error at %q: %w", event.Name, err)
			}
			return nil
		}
		return fmt.Errorf("failed to stat %q: %w", event.Name, err)
	}

	// If it's a new directory, add it to the watcher
	if info.IsDir() {
		// Check if we need to watch this directory
		relPath, err := filepath.Rel(fm.config.InputDir, event.Name)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q: %w", event.Name, err)
		}

		// Check exclude patterns
		for _, pattern := range fm.config.ExcludePatterns {
			match, err := doublestar.Match(pattern, relPath)
			if err != nil {
				return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
			}
			if match {
				return nil
			}
		}

		// Add to watcher
		if err := watcher.Add(event.Name); err != nil {
			return fmt.Errorf("failed to add watch for new directory %q: %w", event.Name, err)
		}
		return nil
	}

	// Process file event
	relPath, err := filepath.Rel(fm.config.InputDir, event.Name)
	if err != nil {
		return fmt.Errorf("failed to get relative path for %q: %w", event.Name, err)
	}

	// Check exclude patterns
	for _, pattern := range fm.config.ExcludePatterns {
		match, err := doublestar.Match(pattern, relPath)
		if err != nil {
			return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
		if match {
			return nil
		}
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
	case taskChan <- fileTask{inputPath: event.Name, outputPath: outputPath}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}