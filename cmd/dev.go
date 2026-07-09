package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dat267/min/pkg/dev"
)

type DevCmdGroup struct {
	Http    HttpCmd        `cmd:"" help:"Generate Go code from cURL string"`
	Har     HarConvertCmd  `cmd:"" help:"Convert HAR capture file to OpenAPI 3.0 specification"`
	OpenAPI OpenAPIGenCmd  `cmd:"" help:"Generate Go SDK client from OpenAPI specification"`
}

type HttpCmd struct {
	CurlCmd string `arg:"" help:"cURL command string"`
	OutPath string `short:"o" default:"tmp/main.go" help:"Output path for generated Go code"`
}

func (c *HttpCmd) Run() error {
	sourceCode, respBody, err := dev.ProcessCurl(c.CurlCmd)
	if err != nil {
		return fmt.Errorf("process curl failed: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(c.OutPath), 0755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}

	if err := os.WriteFile(c.OutPath, []byte(sourceCode), 0644); err != nil {
		return fmt.Errorf("write output file failed: %w", err)
	}

	fmt.Printf("Wrote generated Go code to %s (%d bytes)\n", c.OutPath, len(sourceCode))
	fmt.Printf("Response Body:\n%s\n", string(respBody))
	return nil
}
