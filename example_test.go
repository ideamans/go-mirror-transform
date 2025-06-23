package mirrortransform_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	mirrortransform "github.com/ideamans/go-mirror-transform"
)

// ExampleMirrorTransform_Crawl demonstrates basic usage of MirrorTransform.
func ExampleMirrorTransform_Crawl() {
	// Configure the mirror
	config := mirrortransform.Config{
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
	mt, err := mirrortransform.NewMirrorTransform(&config)
	if err != nil {
		log.Fatal(err)
	}

	// Start crawling
	ctx := context.Background()
	if err := mt.Crawl(ctx); err != nil {
		log.Fatal(err)
	}
}
