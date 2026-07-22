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

	runConfigShow := func(env []string, args ...string) (map[string]any, error) {
		cmdArgs := append([]string{"config", "show"}, args...)
		cmd := exec.Command(binPath, cmdArgs...)
		cmd.Dir = tmpDir
		cmd.Env = append(os.Environ(), env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed: %w (stderr: %s)", err, stderr.String())
		}
		var m map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &m); err != nil {
			return nil, fmt.Errorf("json: %w (output: %s)", err, stdout.String())
		}
		return m, nil
	}

	runGreet := func(env []string, args ...string) (string, error) {
		cmdArgs := append([]string{"greet"}, args...)
		cmd := exec.Command(binPath, cmdArgs...)
		cmd.Dir = tmpDir
		cmd.Env = append(os.Environ(), env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("greet failed: %w (stderr: %s)", err, stderr.String())
		}
		return stdout.String(), nil
	}

	empty := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(empty, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	// Scenario 1: Default values
	m1, err := runConfigShow(nil, "--config-file", empty)
	if err != nil {
		t.Fatalf("Scenario 1: %v", err)
	}
	if m1["core-retries"] != float64(3) {
		t.Errorf("expected core-retries=3, got %v", m1["core-retries"])
	}
	if m1["admin-token"] != "" {
		t.Errorf("expected admin-token=\"\", got %q", m1["admin-token"])
	}

	// Scenario 2: Config file overrides defaults
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{
		"admin-token": "config-token",
		"core-timeout": "5m",
		"core-retries": 10,
		"debug": true,
		"dry-run": true
	}`), 0600); err != nil {
		t.Fatal(err)
	}
	m2, err := runConfigShow(nil, "--config-file", cfgPath)
	if err != nil {
		t.Fatalf("Scenario 2: %v", err)
	}
	if m2["admin-token"] != "config-token" {
		t.Errorf("admin-token=%v", m2["admin-token"])
	}
	if m2["core-retries"] != float64(10) {
		t.Errorf("core-retries=%v", m2["core-retries"])
	}
	if m2["debug"] != true {
		t.Errorf("debug=%v", m2["debug"])
	}

	// Scenario 3: Env overrides config file
	env := []string{"MIN_ADMIN_TOKEN=env-token", "MIN_CORE_TIMEOUT=30m"}
	m3, err := runConfigShow(env, "--config-file", cfgPath)
	if err != nil {
		t.Fatalf("Scenario 3: %v", err)
	}
	if m3["admin-token"] != "env-token" {
		t.Errorf("admin-token=%v", m3["admin-token"])
	}
	if m3["core-retries"] != float64(10) {
		t.Errorf("core-retries=%v", m3["core-retries"])
	}
	if m3["core-timeout"] != "30m" {
		t.Errorf("core-timeout=%v", m3["core-timeout"])
	}

	// Scenario 4: Env full override
	env2 := []string{"MIN_ADMIN_TOKEN=env2", "MIN_CORE_TIMEOUT=45m", "MIN_CORE_RETRIES=99", "MIN_DEBUG=true"}
	m4, err := runConfigShow(env2, "--config-file", empty)
	if err != nil {
		t.Fatalf("Scenario 4: %v", err)
	}
	if m4["admin-token"] != "env2" {
		t.Errorf("admin-token=%v", m4["admin-token"])
	}
	if m4["core-timeout"] != "45m" {
		t.Errorf("core-timeout=%v", m4["core-timeout"])
	}
	if m4["core-retries"] != float64(99) {
		t.Errorf("core-retries=%v", m4["core-retries"])
	}
	if m4["debug"] != true {
		t.Errorf("debug=%v", m4["debug"])
	}

	// Scenario 5a: Subcommand flag default (10s) wins over config default
	out5a, err := runGreet(nil, "--config-file", empty)
	if err != nil {
		t.Fatalf("Scenario 5a: %v", err)
	}
	if !strings.Contains(out5a, "10s") {
		t.Errorf("expected 10s default, got %q", out5a)
	}

	// Scenario 5b: Config file overrides subcommand default
	out5b, err := runGreet(nil, "--config-file", cfgPath)
	if err != nil {
		t.Fatalf("Scenario 5b: %v", err)
	}
	if !strings.Contains(out5b, "5m") {
		t.Errorf("expected 5m from config, got %q", out5b)
	}

	// Scenario 5c: Env overrides subcommand default
	out5c, err := runGreet([]string{"MIN_CORE_TIMEOUT=15m"}, "--config-file", empty)
	if err != nil {
		t.Fatalf("Scenario 5c: %v", err)
	}
	if !strings.Contains(out5c, "15m") {
		t.Errorf("expected 15m from env, got %q", out5c)
	}

	// Scenario 6: CLI flag overrides both
	out6, err := runGreet([]string{"MIN_CORE_TIMEOUT=30m"}, "--config-file", cfgPath, "--core-timeout", "1h")
	if err != nil {
		t.Fatalf("Scenario 6: %v", err)
	}
	if !strings.Contains(out6, "1h") {
		t.Errorf("expected 1h, got %q", out6)
	}

	// Scenario 8a: Config file from env var
	envCfg := filepath.Join(tmpDir, "env_config.json")
	if err := os.WriteFile(envCfg, []byte(`{"admin-token": "env-resolved"}`), 0600); err != nil {
		t.Fatal(err)
	}
	m8a, err := runConfigShow([]string{"MIN_CONFIG_FILE=" + envCfg})
	if err != nil {
		t.Fatalf("Scenario 8a: %v", err)
	}
	if m8a["admin-token"] != "env-resolved" {
		t.Errorf("admin-token=%v", m8a["admin-token"])
	}

	// Scenario 8b: Local min.json file
	localCfg := filepath.Join(tmpDir, "min.json")
	if err := os.WriteFile(localCfg, []byte(`{"admin-token": "local-json-token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	m8b, err := runConfigShow(nil)
	if err != nil {
		t.Fatalf("Scenario 8b: %v", err)
	}
	if m8b["admin-token"] != "local-json-token" {
		t.Errorf("admin-token=%v", m8b["admin-token"])
	}

	// Scenario 9a: Explicit zero values preserved
	zeroCfg := filepath.Join(tmpDir, "zero.json")
	if err := os.WriteFile(zeroCfg, []byte(`{"core-timeout": "", "core-retries": 0}`), 0600); err != nil {
		t.Fatal(err)
	}
	m9a, err := runConfigShow(nil, "--config-file", zeroCfg)
	if err != nil {
		t.Fatalf("Scenario 9a: %v", err)
	}
	if m9a["core-timeout"] != "" {
		t.Errorf("expected empty timeout, got %q", m9a["core-timeout"])
	}
	if m9a["core-retries"] != float64(0) {
		t.Errorf("expected 0 retries, got %v", m9a["core-retries"])
	}

	// Scenario 9b: Explicit empty env vars preserved
	m9b, err := runConfigShow([]string{"MIN_CORE_TIMEOUT=", "MIN_CORE_RETRIES=0"}, "--config-file", empty)
	if err != nil {
		t.Fatalf("Scenario 9b: %v", err)
	}
	if m9b["core-timeout"] != "" {
		t.Errorf("expected empty timeout, got %q", m9b["core-timeout"])
	}
	if m9b["core-retries"] != float64(0) {
		t.Errorf("expected 0 retries, got %v", m9b["core-retries"])
	}

	// Scenario 9c: null values fall back to defaults
	nullCfg := filepath.Join(tmpDir, "null.json")
	if err := os.WriteFile(nullCfg, []byte(`{"core-timeout": null, "core-retries": null}`), 0600); err != nil {
		t.Fatal(err)
	}
	m9c, err := runConfigShow(nil, "--config-file", nullCfg)
	if err != nil {
		t.Fatalf("Scenario 9c: %v", err)
	}
	if m9c["core-timeout"] != "10s" {
		t.Errorf("expected default 10s, got %q", m9c["core-timeout"])
	}
	if m9c["core-retries"] != float64(3) {
		t.Errorf("expected default 3, got %v", m9c["core-retries"])
	}
}

func TestYesFlagGlobal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := filepath.Join(tmpDir, "min")
	_ = exec.Command("go", "build", "-o", binPath, ".").Run()

	empty := filepath.Join(tmpDir, "empty.json")
_ = os.WriteFile(empty, []byte("{}"), 0600)
	env := append(os.Environ(), "MIN_CONFIG_FILE="+empty)

	subs := []string{"-y before", "-y after", "--yes", "with config show"}
	args := [][]string{{"-y", "greet"}, {"greet", "-y"}, {"--yes", "greet"}, {"-y", "config", "show"}}
	for i := range subs {
		t.Run(subs[i], func(t *testing.T) {
			cmd := exec.Command(binPath, args[i]...)
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			if !strings.Contains(string(out), "Hello") && !strings.Contains(string(out), "admin-token") {
				t.Errorf("unexpected output: %s", out)
			}
		})
	}
}

func TestGreetCommandIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := filepath.Join(tmpDir, "min")
	_ = exec.Command("go", "build", "-o", binPath, ".").Run()

	empty := filepath.Join(tmpDir, "empty.json")
_ = os.WriteFile(empty, []byte("{}"), 0600)
	env := append(os.Environ(), "MIN_CONFIG_FILE="+empty)

	t.Run("default", func(t *testing.T) {
		out, err := exec.Command(binPath, "greet").CombinedOutput()
		if err != nil {
			t.Fatalf("%s", out)
		}
		if !strings.Contains(string(out), "Hello, World!") {
			t.Errorf("got %s", out)
		}
	})
	t.Run("name", func(t *testing.T) {
		out, _ := exec.Command(binPath, "greet", "Alice").CombinedOutput()
		if !strings.Contains(string(out), "Alice") {
			t.Errorf("got %s", out)
		}
	})
	t.Run("shout", func(t *testing.T) {
		out, _ := exec.Command(binPath, "greet", "-s", "Bob").CombinedOutput()
		if string(out) != strings.ToUpper(string(out)) {
			t.Errorf("got %s", out)
		}
	})
	t.Run("times", func(t *testing.T) {
		out, _ := exec.Command(binPath, "greet", "-t", "3", "Alice").CombinedOutput()
		if lines := strings.Split(strings.TrimSpace(string(out)), "\n"); len(lines) != 3 {
			t.Errorf("got %d lines", len(lines))
		}
	})
	t.Run("all flags", func(t *testing.T) {
		cmd := exec.Command(binPath, "greet", "-s", "-t", "2", "--core-timeout", "1h", "Bob")
		cmd.Env = env
		out, _ := cmd.CombinedOutput()
		s := string(out)
		if s != strings.ToUpper(s) {
			t.Errorf("not uppercase: %s", s)
		}
		if !strings.Contains(strings.ToUpper(s), "1H") {
			t.Errorf("missing 1h: %s", s)
		}
		if lines := strings.Split(strings.TrimSpace(s), "\n"); len(lines) != 2 {
			t.Errorf("expected 2 lines, got %d", len(lines))
		}
	})
}

func TestConfigInitIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := filepath.Join(tmpDir, "min")
	_ = exec.Command("go", "build", "-o", binPath, ".").Run()

	cfgPath := filepath.Join(tmpDir, "test-config.json")
	cfgEnv := "MIN_CONFIG_FILE=" + cfgPath

	t.Run("init", func(t *testing.T) {
		cmd := exec.Command(binPath, "-y", "config", "init")
		cmd.Env = append(os.Environ(), cfgEnv)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s", out)
		}
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			t.Error("config not created")
		}
	})
	t.Run("no force fails", func(t *testing.T) {
		cmd := exec.Command(binPath, "-y", "config", "init")
		cmd.Env = append(os.Environ(), cfgEnv)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected error, got %s", out)
		}
	})
	t.Run("force overwrites", func(t *testing.T) {
		cmd := exec.Command(binPath, "-y", "config", "init", "--force")
		cmd.Env = append(os.Environ(), cfgEnv)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s", out)
		}
	})
	t.Run("show", func(t *testing.T) {
		cmd := exec.Command(binPath, "-y", "config", "show")
		cmd.Env = append(os.Environ(), cfgEnv)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s", out)
		}
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out)
		}
	})
	t.Run("path", func(t *testing.T) {
		cmd := exec.Command(binPath, "-y", "config", "path")
		cmd.Env = append(os.Environ(), cfgEnv)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s", out)
		}
		if !strings.Contains(string(out), cfgPath) {
			t.Errorf("expected path %q, got %s", cfgPath, out)
		}
	})
}

func TestErrorHandling(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := filepath.Join(tmpDir, "min")
	_ = exec.Command("go", "build", "-o", binPath, ".").Run()

	t.Run("unknown flag", func(t *testing.T) {
		out, err := exec.Command(binPath, "greet", "--unknown").CombinedOutput()
		if err == nil {
			t.Errorf("expected error, got %s", out)
		}
	})
	t.Run("unknown cmd", func(t *testing.T) {
		out, err := exec.Command(binPath, "badcmd").CombinedOutput()
		if err == nil {
			t.Errorf("expected error, got %s", out)
		}
	})
	t.Run("no args", func(t *testing.T) {
		out, err := exec.Command(binPath).CombinedOutput()
		if err == nil {
			t.Errorf("expected error, got %s", out)
		}
		if !strings.Contains(string(out), "config") || !strings.Contains(string(out), "greet") {
			t.Errorf("expected available commands, got %s", out)
		}
	})
}

func TestAppNameResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	customBin := filepath.Join(tmpDir, "myapp")
	_ = exec.Command("go", "build", "-o", customBin, ".").Run()

_ = os.WriteFile(filepath.Join(tmpDir, "myapp.json"), []byte(`{"admin-token": "custom-app-token"}`), 0600)

	cmd := exec.Command(customBin, "config", "show")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s", out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if m["admin-token"] != "custom-app-token" {
		t.Errorf("admin-token=%v", m["admin-token"])
	}
}

func TestContextBinding(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := filepath.Join(tmpDir, "min")
	_ = exec.Command("go", "build", "-o", binPath, ".").Run()

	out, err := exec.Command(binPath, "greet", "World").CombinedOutput()
	if err != nil {
		t.Fatalf("%s", out)
	}
	if !strings.Contains(string(out), "Hello, World!") {
		t.Errorf("got %s", out)
	}
}
