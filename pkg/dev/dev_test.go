package dev_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dat267/min/pkg/dev"
)

func TestProcessCurl(t *testing.T) {
	curlCmd := "curl https://api.github.com/repos/golang/go"

	gotSource, gotBody, err := dev.ProcessCurl(curlCmd)
	if err != nil {
		t.Fatalf("ProcessCurl() error = %v", err)
	}

	outPath := "../../tmp/main.go"
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}

	if err := os.WriteFile(outPath, []byte(gotSource), 0644); err != nil {
		t.Fatalf("failed to write output file: %v", err)
	}

	fmt.Printf("Successfully wrote generated source to %s (%d bytes)", outPath, len(gotSource))
	fmt.Printf("\nResponse Body: \n%s\n", string(gotBody))
}
