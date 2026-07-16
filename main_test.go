package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParameterSpecificity(t *testing.T) {
	// 1. Build the binary
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := filepath.Join(tmpDir, "min")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	// 2. Define the config structures for unmarshaling the test output
	type CoreConfigTest struct {
		Timeout int64 `json:"timeout"` // duration represented as nanoseconds in JSON
		Retries int   `json:"retries"`
	}
	type ConfigTest struct {
		AdminToken string         `json:"admin-token"`
		Core       CoreConfigTest `json:"core"`
		Debug      bool           `json:"debug"`
		DryRun     bool           `json:"dry-run"`
	}

	// Helper function to run the command and parse the JSON config output
	runConfigShow := func(env []string, args ...string) (ConfigTest, error) {
		cmdArgs := append([]string{"config", "show"}, args...)
		cmd := exec.Command(binPath, cmdArgs...)
		cmd.Env = append(os.Environ(), env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return ConfigTest{}, fmt.Errorf("failed to run: %w (stderr: %s)", err, stderr.String())
		}

		var cfg ConfigTest
		if err := json.Unmarshal(stdout.Bytes(), &cfg); err != nil {
			return ConfigTest{}, fmt.Errorf("failed to unmarshal JSON: %w (output: %s)", err, stdout.String())
		}
		return cfg, nil
	}

	// Scenario 1: Default values (no flags, no env vars, empty config file)
	emptyConfigPath := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(emptyConfigPath, []byte("{}"), 0600); err != nil {
		t.Fatalf("failed to write empty config file: %v", err)
	}
	cfg1, err := runConfigShow(nil, "--config-file", emptyConfigPath)
	if err != nil {
		t.Fatalf("Scenario 1 failed: %v", err)
	}
	if cfg1.Core.Retries != 3 {
		t.Errorf("expected default core.retries = 3, got %d", cfg1.Core.Retries)
	}
	if cfg1.AdminToken != "" {
		t.Errorf("expected default admin-token = \"\", got %q", cfg1.AdminToken)
	}

	// Scenario 2: Config file overrides defaults
	configPath := filepath.Join(tmpDir, "config.json")
	configJSON := `{
		"admin-token": "config-token",
		"core": {
			"timeout": "5m",
			"retries": 10
		},
		"debug": true,
		"dry-run": true
	}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg2, err := runConfigShow(nil, "--config-file", configPath)
	if err != nil {
		t.Fatalf("Scenario 2 failed: %v", err)
	}
	if cfg2.AdminToken != "config-token" {
		t.Errorf("expected admin-token = \"config-token\", got %q", cfg2.AdminToken)
	}
	if cfg2.Core.Retries != 10 {
		t.Errorf("expected core.retries = 10, got %d", cfg2.Core.Retries)
	}
	if cfg2.Debug != true {
		t.Errorf("expected debug = true, got %v", cfg2.Debug)
	}

	// Scenario 3: Env vars override config file
	envVars := []string{
		"MIN_ADMIN_TOKEN=env-token",
		"MIN_CORE_TIMEOUT=30m",
	}
	cfg3, err := runConfigShow(envVars, "--config-file", configPath)
	if err != nil {
		t.Fatalf("Scenario 3 failed: %v", err)
	}
	if cfg3.AdminToken != "env-token" {
		t.Errorf("expected admin-token = \"env-token\", got %q", cfg3.AdminToken)
	}
	if cfg3.Core.Retries != 10 { // remains unchanged from config file
		t.Errorf("expected core.retries = 10, got %d", cfg3.Core.Retries)
	}
	// 30m duration = 1800000000000 ns
	if cfg3.Core.Timeout != 1800000000000 {
		t.Errorf("expected core.timeout = 1800000000000, got %d", cfg3.Core.Timeout)
	}

	// Scenario 4: Flags override env vars
	cfg4, err := runConfigShow(envVars,
		"--config-file", configPath,
		"--admin-token", "flag-token",
		"--core-timeout", "45m",
	)
	if err != nil {
		t.Fatalf("Scenario 4 failed: %v", err)
	}
	if cfg4.AdminToken != "flag-token" {
		t.Errorf("expected admin-token = \"flag-token\", got %q", cfg4.AdminToken)
	}
	// 45m duration = 2700000000000 ns
	if cfg4.Core.Timeout != 2700000000000 {
		t.Errorf("expected core.timeout = 2700000000000, got %d", cfg4.Core.Timeout)
	}
}
