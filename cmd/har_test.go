package cmd_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat267/min/cmd"
)

func TestHarCmd(t *testing.T) {
	tmpDir := t.TempDir()
	harPath := filepath.Join(tmpDir, "test.har")
	outJson := filepath.Join(tmpDir, "openapi.json")
	outYaml := filepath.Join(tmpDir, "openapi.yaml")

	harContent := `{
  "log": {
    "entries": [
      {
        "request": {
          "method": "POST",
          "url": "https://api.example.com/v1/users?source=web",
          "queryString": [{"name": "source", "value": "web"}],
          "postData": {
            "mimeType": "application/json",
            "text": "{\"name\": \"Alice\", \"age\": 30}"
          }
        },
        "response": {
          "status": 201,
          "statusText": "Created",
          "content": {
            "mimeType": "application/json",
            "text": "{\"id\": 1, \"name\": \"Alice\", \"age\": 30}"
          }
        }
      }
    ]
  }
}`

	if err := os.WriteFile(harPath, []byte(harContent), 0644); err != nil {
		t.Fatalf("failed to write test HAR: %v", err)
	}

	// Test JSON output
	harJsonCmd := &cmd.HarConvertCmd{
		Input:  harPath,
		Output: outJson,
		Title:  "Test API",
	}
	if err := harJsonCmd.Run(context.Background()); err != nil {
		t.Fatalf("HarCmd JSON failed: %v", err)
	}

	jsonBytes, err := os.ReadFile(outJson)
	if err != nil {
		t.Fatalf("failed to read outJson: %v", err)
	}
	if !strings.Contains(string(jsonBytes), "openapi") || !strings.Contains(string(jsonBytes), "/v1/users") {
		t.Errorf("unexpected JSON spec output: %s", string(jsonBytes))
	}

	// Test YAML output
	harYamlCmd := &cmd.HarConvertCmd{
		Input:  harPath,
		Output: outYaml,
		Title:  "Test API",
	}
	if err := harYamlCmd.Run(context.Background()); err != nil {
		t.Fatalf("HarCmd YAML failed: %v", err)
	}

	yamlBytes, err := os.ReadFile(outYaml)
	if err != nil {
		t.Fatalf("failed to read outYaml: %v", err)
	}
	if !strings.Contains(string(yamlBytes), "openapi: 3.0.3") {
		t.Errorf("unexpected YAML spec output: %s", string(yamlBytes))
	}
}

func TestHarCmdIncrementalMerge(t *testing.T) {
	tmpDir := t.TempDir()
	har1Path := filepath.Join(tmpDir, "test1.har")
	har2Path := filepath.Join(tmpDir, "test2.har")
	outYaml := filepath.Join(tmpDir, "openapi.yaml")

	har1Content := `{
  "log": {
    "entries": [
      {
        "request": { "method": "GET", "url": "https://api.example.com/v1/users" },
        "response": { "status": 200, "statusText": "OK" }
      }
    ]
  }
}`
	har2Content := `{
  "log": {
    "entries": [
      {
        "request": { "method": "POST", "url": "https://api.example.com/v1/posts" },
        "response": { "status": 201, "statusText": "Created" }
      }
    ]
  }
}`

	if err := os.WriteFile(har1Path, []byte(har1Content), 0644); err != nil {
		t.Fatalf("failed to write har1: %v", err)
	}
	if err := os.WriteFile(har2Path, []byte(har2Content), 0644); err != nil {
		t.Fatalf("failed to write har2: %v", err)
	}

	// Step 1: Run har1 -> openapi.yaml
	cmd1 := &cmd.HarConvertCmd{Input: har1Path, Output: outYaml, Title: "Test API"}
	if err := cmd1.Run(context.Background()); err != nil {
		t.Fatalf("cmd1 failed: %v", err)
	}

	// Step 2: Run har2 -> openapi.yaml (should merge /v1/posts alongside existing /v1/users)
	cmd2 := &cmd.HarConvertCmd{Input: har2Path, Output: outYaml, Title: "Test API"}
	if err := cmd2.Run(context.Background()); err != nil {
		t.Fatalf("cmd2 failed: %v", err)
	}

	mergedBytes, err := os.ReadFile(outYaml)
	if err != nil {
		t.Fatalf("read merged yaml failed: %v", err)
	}

	mergedContent := string(mergedBytes)
	if !strings.Contains(mergedContent, "/v1/users") || !strings.Contains(mergedContent, "/v1/posts") {
		t.Errorf("expected merged OpenAPI spec to contain both endpoints, got:\n%s", mergedContent)
	}
}

func TestHarCmdFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	harPath := filepath.Join(tmpDir, "full_export.har")
	outYaml := filepath.Join(tmpDir, "openapi.yaml")

	harContent := `{
  "log": {
    "entries": [
      {
        "request": { "method": "GET", "url": "https://app.example.com/assets/app.js" },
        "response": { "status": 200, "content": { "mimeType": "application/javascript" } }
      },
      {
        "request": { "method": "GET", "url": "https://api.example.com/v1/users" },
        "response": { "status": 200, "content": { "mimeType": "application/json" } }
      },
      {
        "request": { "method": "GET", "url": "https://other.com/v1/data" },
        "response": { "status": 200, "content": { "mimeType": "application/json" } }
      }
    ]
  }
}`

	if err := os.WriteFile(harPath, []byte(harContent), 0644); err != nil {
		t.Fatalf("failed to write test HAR: %v", err)
	}

	filterCmd := &cmd.HarConvertCmd{
		Input:   harPath,
		Output:  outYaml,
		Host:    "api.example.com",
		Filter:  "/v1/",
		ApiOnly: true,
	}

	if err := filterCmd.Run(context.Background()); err != nil {
		t.Fatalf("filterCmd failed: %v", err)
	}

	outBytes, err := os.ReadFile(outYaml)
	if err != nil {
		t.Fatalf("failed to read outYaml: %v", err)
	}

	outContent := string(outBytes)
	if !strings.Contains(outContent, "/v1/users") {
		t.Errorf("expected /v1/users in filtered spec")
	}
	if strings.Contains(outContent, "app.js") {
		t.Errorf("static asset app.js should have been filtered out")
	}
	if strings.Contains(outContent, "other.com") {
		t.Errorf("other.com request should have been filtered out by host")
	}
}


