package filesmirror_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/ideamans/go-files-mirror"
)

// ExampleFilesMirror_Crawl demonstrates basic usage of FilesMirror.
func ExampleFilesMirror_Crawl() {
	// Configure the mirror
	config := filesmirror.Config{
		InputDir:    "images",
		OutputDir:   "output",
		Patterns:    []string{"**/*.jpg", "**/*.jpeg", "**/*.png", "**/*.gif"},
		Concurrency: 4,
		FileCallback: func(inputPath, outputPath string) (bool, error) {
			// In a real scenario, you might convert images here
			// For this example, we'll just copy the file
			fmt.Printf("Processing: %s -> %s.webp\n", inputPath, outputPath)
			
			// Example: Copy file (in practice, you'd convert to WebP)
			src, err := os.Open(inputPath)
			if err != nil {
				return false, err
			}
			defer src.Close()

			dst, err := os.Create(outputPath + ".webp")
			if err != nil {
				return false, err
			}
			defer dst.Close()

			_, err = io.Copy(dst, src)
			return true, err
		},
	}

	// Create the mirror
	fm, err := filesmirror.NewFilesMirror(config)
	if err != nil {
		log.Fatal(err)
	}

	// Start crawling
	ctx := context.Background()
	if err := fm.Crawl(ctx); err != nil {
		log.Fatal(err)
	}
}