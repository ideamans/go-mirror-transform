package mirrortransform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCrawlBasic tests basic crawl functionality.
func TestCrawlBasic(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	// Create test directory structure
	createTestFiles(t, inputDir, []string{
		"file1.jpg",
		"file2.png",
		"file3.txt",
		"dir1/file4.jpg",
		"dir1/file5.gif",
		"dir2/subdir/file6.png",
	})

	processedFiles := make(map[string]string)
	var mu sync.Mutex

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg", "**/*.png", "**/*.gif"},
		Concurrency: 2,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			mu.Lock()
			processedFiles[inputPath] = outputPath
			mu.Unlock()

			// Simulate file processing by creating a marker file
			return true, os.WriteFile(outputPath+".processed", []byte("done"), 0644)
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx := context.Background()
	if err := mt.Crawl(ctx); err != nil {
		t.Fatalf("Crawl failed: %v", err)
	}

	// Verify processed files
	expectedFiles := []string{
		"file1.jpg",
		"file2.png",
		"dir1/file4.jpg",
		"dir1/file5.gif",
		"dir2/subdir/file6.png",
	}

	if len(processedFiles) != len(expectedFiles) {
		t.Errorf("Expected %d files to be processed, got %d", len(expectedFiles), len(processedFiles))
	}

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
}

// TestCrawlExcludePatterns tests exclude pattern functionality.
func TestCrawlExcludePatterns(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	createTestFiles(t, inputDir, []string{
		"file1.jpg",
		"temp/file2.jpg",
		"cache/file3.jpg",
		"dir1/file4.jpg",
		".hidden/file5.jpg",
	})

	var processedCount int32

	config := Config{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		Patterns:        []string{"**/*.jpg"},
		ExcludePatterns: []string{"temp/**", "cache/**", ".*/**"},
		Concurrency:     1,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			atomic.AddInt32(&processedCount, 1)
			return true, nil
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx := context.Background()
	if err := mt.Crawl(ctx); err != nil {
		t.Fatalf("Crawl failed: %v", err)
	}

	// Should only process file1.jpg and dir1/file4.jpg
	if processedCount != 2 {
		t.Errorf("Expected 2 files to be processed, got %d", processedCount)
	}
}

// TestCrawlConcurrency tests different concurrency levels.
func TestCrawlConcurrency(t *testing.T) {
	t.Parallel()
	concurrencyLevels := []int{1, 2, 4}

	for _, concurrency := range concurrencyLevels {
		concurrency := concurrency // capture range variable
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {
			t.Parallel()
			testDir := t.TempDir()
			inputDir := filepath.Join(testDir, "input")
			outputDir := filepath.Join(testDir, "output")

			// Create many test files
			var files []string
			for i := 0; i < 20; i++ {
				files = append(files, fmt.Sprintf("file%d.jpg", i))
			}
			createTestFiles(t, inputDir, files)

			var processedCount int32
			var maxConcurrent int32
			var currentConcurrent int32

			config := Config{
				InputDir:    inputDir,
				OutputDir:   outputDir,
				Patterns:    []string{"**/*.jpg"},
				Concurrency: concurrency,
				FileCallback: func(inputPath, outputPath string) (bool, error) {
					// Track concurrent executions
					current := atomic.AddInt32(&currentConcurrent, 1)
					defer atomic.AddInt32(&currentConcurrent, -1)

					// Update max concurrent
					for {
						max := atomic.LoadInt32(&maxConcurrent)
						if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
							break
						}
					}

					// Simulate some work
					time.Sleep(10 * time.Millisecond)

					atomic.AddInt32(&processedCount, 1)
					return true, nil
				},
			}

			mt, err := NewMirrorTransform(&config)
			if err != nil {
				t.Fatalf("Failed to create MirrorTransform: %v", err)
			}

			ctx := context.Background()
			if err := mt.Crawl(ctx); err != nil {
				t.Fatalf("Crawl failed: %v", err)
			}

			if processedCount != int32(len(files)) {
				t.Errorf("Expected %d files to be processed, got %d", len(files), processedCount)
			}

			// Verify concurrency was respected
			if maxConcurrent > int32(concurrency) {
				t.Errorf("Max concurrent executions %d exceeded configured concurrency %d", maxConcurrent, concurrency)
			}
		})
	}
}

// TestCrawlContextCancellation tests graceful shutdown on context cancellation.
func TestCrawlContextCancellation(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	// Create many test files
	var files []string
	for i := 0; i < 100; i++ {
		files = append(files, fmt.Sprintf("file%d.jpg", i))
	}
	createTestFiles(t, inputDir, files)

	var processedCount int32

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg"},
		Concurrency: 4,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			// Simulate slow processing
			time.Sleep(50 * time.Millisecond)

			atomic.AddInt32(&processedCount, 1)
			return true, nil
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short time
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err = mt.Crawl(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}

	// Should have processed some files but not all
	processed := atomic.LoadInt32(&processedCount)
	if processed == 0 {
		t.Error("No files were processed before cancellation")
	}
	if processed == int32(len(files)) {
		t.Error("All files were processed despite cancellation")
	}
}

// TestCrawlCircularReference tests circular reference detection.
func TestCrawlCircularReference(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")

	// Create the input directory
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	tests := []struct {
		name      string
		outputDir string
		wantErr   bool
	}{
		{
			name:      "OutputInsideInput",
			outputDir: filepath.Join(inputDir, "output"),
			wantErr:   true,
		},
		{
			name:      "OutputSameAsInput",
			outputDir: inputDir,
			wantErr:   true,
		},
		{
			name:      "ValidSeparateDirectories",
			outputDir: filepath.Join(testDir, "output"),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			config := Config{
				InputDir:  inputDir,
				OutputDir: tt.outputDir,
				Patterns:  []string{"**/*.jpg"},
				FileCallback: func(inputPath, outputPath string) (bool, error) {
					return true, nil
				},
			}

			mt, err := NewMirrorTransform(&config)
			if err != nil {
				t.Fatalf("Failed to create MirrorTransform: %v", err)
			}

			err = mt.Crawl(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Crawl() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestCrawlErrorHandling tests error callback functionality.
func TestCrawlErrorHandling(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	createTestFiles(t, inputDir, []string{
		"file1.jpg",
		"file2.jpg",
	})

	// Create a file that will cause an error when creating output directory
	badOutputPath := filepath.Join(outputDir, "file1.jpg")
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(badOutputPath, []byte("existing file"), 0644)

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg"},
		Concurrency: 1,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			if filepath.Base(inputPath) == "file1.jpg" {
				return false, fmt.Errorf("simulated error")
			}
			return true, nil
		},
		ErrorCallback: func(path string, err error) (bool, error) {
			// Continue processing
			return false, nil
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	// With error callback, should not return error
	err = mt.Crawl(context.Background())
	if err == nil {
		t.Error("Expected error from file callback")
	}

	// Now test without error callback
	config.ErrorCallback = nil
	mt, _ = NewMirrorTransform(&config)

	err = mt.Crawl(context.Background())
	if err == nil {
		t.Error("Expected error without error callback")
	}
}

// TestCrawlStopOnCallbackFalse tests that crawl stops when callback returns false.
func TestCrawlStopOnCallbackFalse(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	inputDir := filepath.Join(testDir, "input")
	outputDir := filepath.Join(testDir, "output")

	createTestFiles(t, inputDir, []string{
		"file1.jpg",
		"file2.jpg",
		"file3.jpg",
	})

	var processedCount int32

	config := Config{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		Patterns:    []string{"**/*.jpg"},
		Concurrency: 1,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			count := atomic.AddInt32(&processedCount, 1)
			// Stop after processing first file
			return count < 2, nil
		},
	}

	mt, err := NewMirrorTransform(&config)
	if err != nil {
		t.Fatalf("Failed to create MirrorTransform: %v", err)
	}

	err = mt.Crawl(context.Background())
	if err == nil {
		t.Error("Expected error when callback returns false")
	}

	// Should have processed at most 2 files (one that returned true, one that returned false)
	if processedCount > 2 {
		t.Errorf("Expected at most 2 files to be processed, got %d", processedCount)
	}
}

// Helper function to create test files
func createTestFiles(t *testing.T, baseDir string, files []string) {
	for _, file := range files {
		path := filepath.Join(baseDir, file)
		dir := filepath.Dir(path)

		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}

		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}
}

// TestNewMirrorTransformValidation tests configuration validation.
func TestNewMirrorTransformValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "MissingInputDir",
			config:  Config{OutputDir: "/tmp/out", Patterns: []string{"*.jpg"}},
			wantErr: true,
		},
		{
			name:    "MissingOutputDir",
			config:  Config{InputDir: "/tmp/in", Patterns: []string{"*.jpg"}},
			wantErr: true,
		},
		{
			name:    "MissingPatterns",
			config:  Config{InputDir: "/tmp/in", OutputDir: "/tmp/out"},
			wantErr: true,
		},
		{
			name: "MissingCallback",
			config: Config{
				InputDir:  "/tmp/in",
				OutputDir: "/tmp/out",
				Patterns:  []string{"*.jpg"},
			},
			wantErr: true,
		},
		{
			name: "ValidConfig",
			config: Config{
				InputDir:  "/tmp/in",
				OutputDir: "/tmp/out",
				Patterns:  []string{"*.jpg"},
				FileCallback: func(in, out string) (bool, error) {
					return true, nil
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewMirrorTransform(&tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMirrorTransform() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
