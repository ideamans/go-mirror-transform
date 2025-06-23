package mirrortransform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWatchBasic tests basic watch functionality.
func TestWatchBasic(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	// Create input directory
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	processedFiles := make(map[string]string)
	var mu sync.Mutex

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg", "**/*.png"},
		Concurrency: 2,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			mu.Lock()
			processedFiles[inputPath] = outputPath
			mu.Unlock()

			// Simulate file processing
			return true, os.WriteFile(outputPath+".processed", []byte("done"), 0644)
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching in background
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- mt.Watch(ctx)
	}()

	// Give watcher time to start
	time.Sleep(200 * time.Millisecond)

	// Create test files
	testFiles := []string{
		"file1.jpg",
		"file2.png",
		"file3.txt", // Should be ignored
		"dir1/file4.jpg",
	}

	for _, file := range testFiles {
		path := filepath.Join(inputDir, file)
		dir := filepath.Dir(path)

		// Create directory first if it doesn't exist
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory %s: %v", dir, err)
			}
			// Give watcher time to detect new directory
			time.Sleep(200 * time.Millisecond)
		}

		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}

		// Give watcher time to process
		time.Sleep(100 * time.Millisecond)
	}

	// Wait a bit more for processing
	time.Sleep(300 * time.Millisecond)

	// Cancel watch
	cancel()

	// Check for watch errors
	select {
	case err := <-watchErr:
		if err != context.Canceled {
			t.Errorf("Watch returned unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Watch did not return after cancel")
	}

	// Verify processed files
	expectedFiles := []string{
		"file1.jpg",
		"file2.png",
		"dir1/file4.jpg",
	}

	mu.Lock()
	defer mu.Unlock()

	for _, relPath := range expectedFiles {
		inputPath := filepath.Join(inputDir, relPath)
		outputPath := filepath.Join(outputDir, relPath)

		if processed, ok := processedFiles[inputPath]; !ok {
			t.Errorf("File %s was not processed", inputPath)
		} else if processed != outputPath {
			t.Errorf("File %s: expected output path %s, got %s", inputPath, outputPath, processed)
		}

		// Check if marker file exists
		if _, err := os.Stat(outputPath + ".processed"); os.IsNotExist(err) {
			t.Errorf("Marker file for %s was not created", outputPath)
		}
	}

	// Verify file3.txt was not processed
	if _, ok := processedFiles[filepath.Join(inputDir, "file3.txt")]; ok {
		t.Error("file3.txt should not have been processed")
	}
}

// TestWatchFileModification tests that file modifications are detected.
func TestWatchFileModification(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	// Create input directory and initial file
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	testFile := filepath.Join(inputDir, "test.jpg")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	var processCount int32

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg"},
		Concurrency: 1,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			atomic.AddInt32(&processCount, 1)
			return true, nil
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching
	go mt.Watch(ctx)

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Modify the file
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// File should have been processed at least once
	count := atomic.LoadInt32(&processCount)
	if count < 1 {
		t.Errorf("Expected file to be processed at least once, got %d", count)
	}
}

// TestWatchExcludePatterns tests exclude pattern functionality in watch mode.
func TestWatchExcludePatterns(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	// Create input directory
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	processedFiles := make(map[string]bool)
	var mu sync.Mutex

	config := Config{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		Patterns:        []string{"**/*.jpg"},
		ExcludePatterns: []string{"temp/**", ".*/**"},
		Concurrency:     1,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			mu.Lock()
			processedFiles[inputPath] = true
			mu.Unlock()
			return true, nil
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching
	go mt.Watch(ctx)

	// Give watcher time to start
	time.Sleep(200 * time.Millisecond)

	// Create test files
	testFiles := []struct {
		path    string
		process bool
	}{
		{"file1.jpg", true},
		{"temp/file2.jpg", false},
		{".hidden/file3.jpg", false},
		{"valid/file4.jpg", true},
	}

	for _, tf := range testFiles {
		path := filepath.Join(inputDir, tf.path)
		dir := filepath.Dir(path)

		// Create directory first if it doesn't exist
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory %s: %v", dir, err)
			}
			// Give watcher time to detect new directory
			time.Sleep(200 * time.Millisecond)
		}

		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}

		// Give time for each file to be processed
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Verify results
	mu.Lock()
	defer mu.Unlock()

	for _, tf := range testFiles {
		path := filepath.Join(inputDir, tf.path)
		processed := processedFiles[path]

		if tf.process && !processed {
			t.Errorf("File %s should have been processed but wasn't", tf.path)
		} else if !tf.process && processed {
			t.Errorf("File %s should not have been processed but was", tf.path)
		}
	}
}

// TestWatchContextCancellation tests graceful shutdown on context cancellation.
func TestWatchContextCancellation(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	// Create input directory
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	var processing int32
	var processed int32

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg"},
		Concurrency: 4,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			atomic.AddInt32(&processing, 1)
			defer atomic.AddInt32(&processing, -1)

			// Simulate slow processing
			time.Sleep(100 * time.Millisecond)

			atomic.AddInt32(&processed, 1)
			return true, nil
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start watching
	watchDone := make(chan error, 1)
	go func() {
		watchDone <- mt.Watch(ctx)
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Create multiple files quickly
	for i := 0; i < 10; i++ {
		path := filepath.Join(inputDir, fmt.Sprintf("file%d.jpg", i))
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	// Give some time for processing to start
	time.Sleep(50 * time.Millisecond)

	// Cancel while processing
	cancel()

	// Wait for watch to complete
	select {
	case err := <-watchDone:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Watch did not complete within timeout")
	}

	// Verify no processing is still running
	if n := atomic.LoadInt32(&processing); n != 0 {
		t.Errorf("Expected no processing after cancel, but %d still running", n)
	}

	// Some files should have been processed
	if n := atomic.LoadInt32(&processed); n == 0 {
		t.Error("No files were processed before cancellation")
	}
}

// TestWatchNewDirectory tests that new directories are automatically watched.
func TestWatchNewDirectory(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	// Create input directory
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	processedFiles := make(map[string]bool)
	var mu sync.Mutex

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg"},
		Concurrency: 1,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			mu.Lock()
			processedFiles[inputPath] = true
			mu.Unlock()
			return true, nil
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching
	go mt.Watch(ctx)

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Create a new directory
	newDir := filepath.Join(inputDir, "newdir")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("Failed to create new directory: %v", err)
	}

	// Give watcher time to detect new directory
	time.Sleep(100 * time.Millisecond)

	// Create a file in the new directory
	newFile := filepath.Join(newDir, "test.jpg")
	if err := os.WriteFile(newFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file in new directory: %v", err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify the file in new directory was processed
	mu.Lock()
	defer mu.Unlock()

	if !processedFiles[newFile] {
		t.Error("File in new directory was not processed")
	}
}

// TestWatchFileCallbackError tests that file callback errors stop the watch.
func TestWatchFileCallbackError(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	// Create input directory
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg"},
		Concurrency: 1,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			// Always fail
			return false, fmt.Errorf("simulated file callback error")
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- mt.Watch(ctx)
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Create a test file that will trigger error
	testFile := filepath.Join(inputDir, "test.jpg")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for error
	select {
	case err := <-watchErr:
		if err == nil {
			t.Error("Expected error from file callback")
		}
		// Should contain the error message
		if !strings.Contains(err.Error(), "file callback failed") {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Watch did not return error within timeout")
	}
}
