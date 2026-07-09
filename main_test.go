package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var sharedBin string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "min-test-shared-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create shared dir: %v\n", err)
		os.Exit(1)
	}
	sharedBin = filepath.Join(tmpDir, "min")
	buildCmd := exec.Command("go", "build", "-o", sharedBin, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n%s\n", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestInitBasic(t *testing.T) {
	binPath := sharedBin

	t.Run("basic init", func(t *testing.T) {
		dir := t.TempDir()
		oldWd, _ := os.Getwd(); _ = oldWd
		_ = os.Chdir(dir)
		defer func() { _ = os.Chdir(oldWd) }()

		out, err := exec.Command(binPath, "init", "testproj").CombinedOutput()
		if err != nil {
			t.Fatalf("%s", out)
		}
		if !strings.Contains(string(out), "testproj") {
			t.Errorf("got %s", out)
		}
	})
	t.Run("init with module", func(t *testing.T) {
		dir := t.TempDir()
		oldWd, _ := os.Getwd(); _ = oldWd
		_ = os.Chdir(dir)
		defer func() { _ = os.Chdir(oldWd) }()
		out, err := exec.Command(binPath, "init", "--module", "github.com/user/pkg", "modproj").CombinedOutput()
		if err != nil {
			t.Fatalf("%s", out)
		}
		if !strings.Contains(string(out), "modproj") {
			t.Errorf("got %s", out)
		}
	})
}

func TestConfigInitIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := sharedBin
	cfgPath := filepath.Join(tmpDir, "test-config.json")
	cfgEnv := "MIN_CONFIG_FILE=" + cfgPath

	t.Run("init", func(t *testing.T) {
		cmd := exec.Command(binPath, "config", "init")
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
		cmd := exec.Command(binPath, "config", "init")
		cmd.Env = append(os.Environ(), cfgEnv)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected error, got %s", out)
		}
	})
	t.Run("force overwrites", func(t *testing.T) {
		cmd := exec.Command(binPath, "config", "init", "--overwrite")
		cmd.Env = append(os.Environ(), cfgEnv)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s", out)
		}
	})
	t.Run("show", func(t *testing.T) {
		cmd := exec.Command(binPath, "config", "show")
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
		cmd := exec.Command(binPath, "config", "path")
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

	binPath := sharedBin

	t.Run("unknown flag", func(t *testing.T) {
		out, err := exec.Command(binPath, "init", "--unknown").CombinedOutput()
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
		if !strings.Contains(string(out), "config") || !strings.Contains(string(out), "init") {
			t.Errorf("expected available commands, got %s", out)
		}
	})
}

func TestInitBinary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "min-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := sharedBin
	oldWd, _ := os.Getwd(); _ = oldWd
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldWd) }()

	out, err := exec.Command(binPath, "init", "ctxproj").CombinedOutput()
	if err != nil {
		t.Fatalf("%s", out)
	}
	if !strings.Contains(string(out), "ctxproj") {
		t.Errorf("got %s", out)
	}
}
