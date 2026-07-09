package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dat267/min/cmd"
)

func TestHttpCmd_Run(t *testing.T) {
	tmpDir, _ := os.MkdirTemp(os.TempDir(), "*")
	outPath := filepath.Join(tmpDir, "main.go")

	cmd := &cmd.HttpCmd{
		CurlCmd: "curl -X GET https://api.github.com/repos/golang/go",
		OutPath: outPath,
	}

	_ = cmd.Run()
}
