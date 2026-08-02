package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestVersionCmd(t *testing.T) {
	out := captureStdout(t, func() {
		_ = (&VersionCmd{}).Run()
	})
	if strings.TrimSpace(out) != Version {
		t.Errorf("expected version %q, got %q", Version, out)
	}
}

func TestResolveConfigPath_EnvVar(t *testing.T) {
	expected := "/tmp/custom_min.json"
	t.Setenv("MIN_CONFIG_FILE", expected)

	if got := resolveConfigPath(); got != expected {
		t.Errorf("expected %s via env var, got %s", expected, got)
	}
}

func TestResolveConfigPath_LocalFile(t *testing.T) {
	// Ensure env var is clear
	t.Setenv("MIN_CONFIG_FILE", "")

	// Create a dummy local file named "min.json" in the current test working directory
	localFile := appName + ".json"
	if err := os.WriteFile(localFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create local config file: %v", err)
	}
	defer func() { _ = os.Remove(localFile) }()

	if got := resolveConfigPath(); got != localFile {
		t.Errorf("expected local file %s, got %s", localFile, got)
	}
}

func TestResolveConfigFileFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"Long flag space", []string{"app", "--config-file", "mycfg.json", "other"}, "mycfg.json"},
		{"Long flag equals", []string{"app", "command", "--config-file=mycfg.json"}, "mycfg.json"},
		{"Missing value", []string{"app", "--config-file"}, ""},
		{"No config flag", []string{"app", "--help"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Backup and restore os.Args
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = tt.args

			if got := resolveConfigFileFlag(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestJSONResolver(t *testing.T) {
	jsonConfig := `{
		"string_key": "hello",
		"bool_key": true,
		"int_key": 42,
		"nested": {
			"sub_key": "world",
			"deeply": {
				"nested_key": 100
			}
		}
	}`

	resolver, err := JSONResolver(strings.NewReader(jsonConfig))
	if err != nil {
		t.Fatalf("JSONResolver initialization failed: %v", err)
	}

	tests := []struct {
		flagName string
		expected any
	}{
		{"string_key", "hello"},
		{"bool_key", true},
		{"int_key", float64(42)}, // JSON unmarshals numbers to float64 natively
		{"nested-sub_key", "world"},
		{"nested-deeply-nested_key", float64(100)},
		{"non_existent", nil},
	}

	ctx, _ := kong.New(&CLI{})
	kongCtx := kong.Context{Kong: ctx}
	dummyPath := &kong.Path{}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := &kong.Flag{
				Value: &kong.Value{
					Name: tt.flagName,
				},
			}

			val, err := resolver.Resolve(&kongCtx, dummyPath, flag)
			if err != nil {
				t.Fatalf("unexpected error resolving flag: %v", err)
			}

			if val != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, val)
			}
		})
	}
}

func TestJSONResolver_MalformedJSON(t *testing.T) {
	_, err := JSONResolver(strings.NewReader(`{malformed_json`))
	if err == nil {
		t.Error("expected error parsing malformed JSON, got nil")
	}
}
