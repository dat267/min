package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		Timeout string `json:"timeout"`
		Retries int    `json:"retries"`
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
		cmd.Dir = tmpDir
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

	// Helper function to run the greet command and return stdout
	runGreet := func(env []string, args ...string) (string, error) {
		cmdArgs := append([]string{"greet"}, args...)
		cmd := exec.Command(binPath, cmdArgs...)
		cmd.Dir = tmpDir
		cmd.Env = append(os.Environ(), env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to run greet: %w (stderr: %s)", err, stderr.String())
		}
		return stdout.String(), nil
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
	if cfg3.Core.Timeout != "30m" {
		t.Errorf("expected core.timeout = \"30m\", got %q", cfg3.Core.Timeout)
	}

	// Scenario 4: Env vars fully override config file (no CLI flags for config values)
	envVars2 := []string{
		"MIN_ADMIN_TOKEN=env2-token",
		"MIN_CORE_TIMEOUT=45m",
		"MIN_CORE_RETRIES=99",
		"MIN_DEBUG=true",
	}
	cfg4, err := runConfigShow(envVars2, "--config-file", emptyConfigPath)
	if err != nil {
		t.Fatalf("Scenario 4 failed: %v", err)
	}
	if cfg4.AdminToken != "env2-token" {
		t.Errorf("expected admin-token = \"env2-token\", got %q", cfg4.AdminToken)
	}
	if cfg4.Core.Timeout != "45m" {
		t.Errorf("expected core.timeout = \"45m\", got %q", cfg4.Core.Timeout)
	}
	if cfg4.Core.Retries != 99 {
		t.Errorf("expected core.retries = 99, got %d", cfg4.Core.Retries)
	}
	if !cfg4.Debug {
		t.Errorf("expected debug = true, got %v", cfg4.Debug)
	}

	// Scenario 5: Subcommand flag default vs. Root config default vs. File/Env specificity
	// 5a. No config, no env -> should use GreetCmd default timeout (10s) instead of Config default (2m)
	out5a, err := runGreet(nil, "--config-file", emptyConfigPath)
	if err != nil {
		t.Fatalf("Scenario 5a failed: %v", err)
	}
	if !strings.Contains(out5a, "timeout setting is 10s") {
		t.Errorf("expected subcommand default timeout 10s, got: %q", out5a)
	}

	// 5b. Config file sets timeout -> should override GreetCmd default timeout (10s)
	out5b, err := runGreet(nil, "--config-file", configPath)
	if err != nil {
		t.Fatalf("Scenario 5b failed: %v", err)
	}
	if !strings.Contains(out5b, "timeout setting is 5m") {
		t.Errorf("expected config file timeout 5m to override, got: %q", out5b)
	}

	// 5c. Env var sets timeout -> should override GreetCmd default timeout (10s)
	out5c, err := runGreet([]string{"MIN_CORE_TIMEOUT=15m"}, "--config-file", emptyConfigPath)
	if err != nil {
		t.Fatalf("Scenario 5c failed: %v", err)
	}
	if !strings.Contains(out5c, "timeout setting is 15m") {
		t.Errorf("expected env timeout 15m to override, got: %q", out5c)
	}

	// Scenario 6: CLI flag overrides both config file and environment variables
	out6, err := runGreet([]string{"MIN_CORE_TIMEOUT=30m"}, "--config-file", configPath, "--core-timeout", "1h")
	if err != nil {
		t.Fatalf("Scenario 6 failed: %v", err)
	}
	if !strings.Contains(out6, "timeout setting is 1h") {
		t.Errorf("expected CLI flag 1h to override both env and config file, got: %q", out6)
	}

	// Scenario 7: Duplicate configuration key validation in the config file
	duplicateConfigPath := filepath.Join(tmpDir, "duplicate.json")
	duplicateJSON := `{
		"core-timeout": "5m",
		"core": {
			"timeout": "10m"
		}
	}`
	if err := os.WriteFile(duplicateConfigPath, []byte(duplicateJSON), 0600); err != nil {
		t.Fatalf("failed to write duplicate config file: %v", err)
	}
	_, err = runGreet(nil, "--config-file", duplicateConfigPath)
	if err == nil {
		t.Errorf("Scenario 7 failed: expected command to fail with duplicate config file, but it succeeded")
	} else {
		if !strings.Contains(err.Error(), "duplicate configuration key") {
			t.Errorf("expected error message to mention duplicate configuration key, got: %v", err)
		}
	}

	// Scenario 8: Config file location resolution priority
	// 8a. Resolution via $MIN_CONFIG_FILE env var (when --config-file is omitted)
	envConfigPath := filepath.Join(tmpDir, "env_config.json")
	envConfigJSON := `{
		"admin-token": "env-resolved-token"
	}`
	if err := os.WriteFile(envConfigPath, []byte(envConfigJSON), 0600); err != nil {
		t.Fatalf("failed to write env config file: %v", err)
	}
	cfg8a, err := runConfigShow([]string{"MIN_CONFIG_FILE=" + envConfigPath})
	if err != nil {
		t.Fatalf("Scenario 8a failed: %v", err)
	}
	if cfg8a.AdminToken != "env-resolved-token" {
		t.Errorf("expected admin-token = \"env-resolved-token\", got %q", cfg8a.AdminToken)
	}

	// 8b. Resolution via local default min.json in working directory (when --config-file and env are omitted)
	localConfigPath := filepath.Join(tmpDir, "min.json")
	localConfigJSON := `{
		"admin-token": "local-json-token"
	}`
	if err := os.WriteFile(localConfigPath, []byte(localConfigJSON), 0600); err != nil {
		t.Fatalf("failed to write local min.json file: %v", err)
	}
	cfg8b, err := runConfigShow(nil)
	if err != nil {
		t.Fatalf("Scenario 8b failed: %v", err)
	}
	if cfg8b.AdminToken != "local-json-token" {
		t.Errorf("expected admin-token = \"local-json-token\", got %q", cfg8b.AdminToken)
	}
}
