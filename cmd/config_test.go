package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureConfigStdout intercepts standard output to verify CLI print statements.
func captureConfigStdout(f func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

// setupTestConfig configures a temporary path for the config file.
func setupTestConfig(t *testing.T) string {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	SetConfigPath(p)
	t.Cleanup(func() {
		SetConfigPath("")
	})
	return p
}

func TestConfigInitCmd_Run(t *testing.T) {
	p := setupTestConfig(t)

	// 1. Test standard initialization
	initCmd := &ConfigInitCmd{}
	out := captureConfigStdout(func() {
		if err := initCmd.Run(); err != nil {
			t.Fatalf("unexpected error on init: %v", err)
		}
	})
	if !strings.Contains(out, "created at") {
		t.Errorf("expected success message, got: %s", out)
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Fatal("expected config file to be created on disk")
	}

	// 2. Test initialization without overwrite fails
	if err := initCmd.Run(); err == nil {
		t.Fatal("expected error when initializing over existing file without Overwrite=true")
	}

	// 3. Test initialization with overwrite succeeds
	initCmd.Overwrite = true
	if err := initCmd.Run(); err != nil {
		t.Fatalf("unexpected error with Overwrite=true: %v", err)
	}
}

func TestConfigPathCmd_Run(t *testing.T) {
	p := setupTestConfig(t)

	// Test missing file
	out := captureConfigStdout(func() {
		_ = (&ConfigPathCmd{}).Run()
	})
	if !strings.Contains(out, "(does not exist)") {
		t.Errorf("expected '(does not exist)', got: %s", out)
	}

	// Test existing file
	_ = (&ConfigInitCmd{}).Run()
	out = captureConfigStdout(func() {
		_ = (&ConfigPathCmd{}).Run()
	})
	if !strings.Contains(out, p) {
		t.Errorf("expected path %s, got: %s", p, out)
	}
}

func TestConfigShowCmd_Run(t *testing.T) {
	_ = setupTestConfig(t)

	// Test missing file
	out := captureConfigStdout(func() {
		_ = (&ConfigShowCmd{}).Run()
	})
	if !strings.Contains(out, "(does not exist)") {
		t.Errorf("expected '(does not exist)', got: %s", out)
	}

	// Test existing file with data
	_ = (&ConfigInitCmd{}).Run()
	_ = (&ConfigSetCmd{Key: "server.port", Value: "8080"}).Run()
	out = captureConfigStdout(func() {
		_ = (&ConfigShowCmd{}).Run()
	})
	if !strings.Contains(out, `"server"`) || !strings.Contains(out, `"port": 8080`) {
		t.Errorf("expected JSON output containing server.port=8080, got: %s", out)
	}
}

func TestConfigSetCmd_Types(t *testing.T) {
	p := setupTestConfig(t)

	tests := []struct {
		key      string
		valIn    string
		expected any
	}{
		{"string_val", "hello world", "hello world"},
		{"bool_true", "true", true},
		{"bool_false", "false", false},
		{"int_val", "42", float64(42)}, // JSON unmarshals numbers to float64
	}

	for _, tc := range tests {
		cmd := &ConfigSetCmd{Key: tc.key, Value: tc.valIn}
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to set %s: %v", tc.key, err)
		}
	}

	m, err := loadConfigMap(p)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	for _, tc := range tests {
		if val, ok := m[tc.key]; !ok {
			t.Errorf("key %s missing from config map", tc.key)
		} else if val != tc.expected {
			t.Errorf("key %s: expected %v (%T), got %v (%T)", tc.key, tc.expected, tc.expected, val, val)
		}
	}
}

func TestConfigSetCmd_Nested(t *testing.T) {
	p := setupTestConfig(t)

	// Set deep nested value
	_ = (&ConfigSetCmd{Key: "database.master.host", Value: "localhost"}).Run()
	_ = (&ConfigSetCmd{Key: "database.master.port", Value: "5432"}).Run()

	m, _ := loadConfigMap(p)
	db, ok := m["database"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'database' to be map[string]any, got %T", m["database"])
	}
	master, ok := db["master"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'database.master' to be map[string]any, got %T", db["master"])
	}
	if master["host"] != "localhost" {
		t.Errorf("expected host localhost, got %v", master["host"])
	}
	if master["port"] != float64(5432) {
		t.Errorf("expected port 5432, got %v", master["port"])
	}

	// Overwrite intermediate map with scalar
	_ = (&ConfigSetCmd{Key: "database.master", Value: "overwritten"}).Run()
	m, _ = loadConfigMap(p)
	db = m["database"].(map[string]any)
	if db["master"] != "overwritten" {
		t.Errorf("expected 'database.master' to be 'overwritten', got %v", db["master"])
	}

	// Overwrite scalar with map implicitly
	_ = (&ConfigSetCmd{Key: "database.master.new_key", Value: "1"}).Run()
	m, _ = loadConfigMap(p)
	db = m["database"].(map[string]any)
	master, ok = db["master"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'database.master' to revert to map, got %T", db["master"])
	}
	if master["new_key"] != float64(1) {
		t.Errorf("expected new_key to be 1")
	}
}

func TestConfigUnsetCmd(t *testing.T) {
	p := setupTestConfig(t)

	// Setup initial config
	_ = (&ConfigSetCmd{Key: "app.name", Value: "min"}).Run()
	_ = (&ConfigSetCmd{Key: "app.debug", Value: "true"}).Run()
	_ = (&ConfigSetCmd{Key: "version", Value: "v1.0"}).Run()

	// Unset nested key
	out := captureConfigStdout(func() {
		_ = (&ConfigUnsetCmd{Key: "app.debug"}).Run()
	})
	if !strings.Contains(out, "Unset \"app.debug\"") {
		t.Errorf("expected success message, got %s", out)
	}

	m, _ := loadConfigMap(p)
	appMap := m["app"].(map[string]any)
	if _, exists := appMap["debug"]; exists {
		t.Error("expected app.debug to be deleted")
	}
	if appMap["name"] != "min" {
		t.Error("expected app.name to remain intact")
	}

	// Unset missing key
	out = captureConfigStdout(func() {
		_ = (&ConfigUnsetCmd{Key: "app.missing"}).Run()
	})
	if !strings.Contains(out, "not found") {
		t.Errorf("expected not found message, got %s", out)
	}

	// Unset top-level key containing nested maps
	_ = (&ConfigUnsetCmd{Key: "app"}).Run()
	m, _ = loadConfigMap(p)
	if _, exists := m["app"]; exists {
		t.Error("expected top level 'app' to be deleted entirely")
	}
	if m["version"] != "v1.0" {
		t.Error("expected 'version' to remain intact")
	}
}

func TestSaveConfigMap_Formatting(t *testing.T) {
	p := setupTestConfig(t)
	m := map[string]any{"hello": "world"}
	if err := saveConfigMap(p, m); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Verify file ends with newline (POSIX standard)
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Error("expected config file to end with a newline character")
	}
}
