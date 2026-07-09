package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//
// Quick-start template for testing a command's Run method.
//
// 1. Copy this function.
// 2. Replace the command struct and fields.
// 3. If the command uses config, call SetConfigPath first.
//

func TestXxxCmd_Run(t *testing.T) {
	SetConfigPath(filepath.Join(t.TempDir(), "cfg.json"))

	cmd := &ConfigInitCmd{
		Overwrite: true,
	}

	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

// Template for a command that writes to stdout.
func TestXxxCmd_Stdout(t *testing.T) {
	SetConfigPath(filepath.Join(t.TempDir(), "cfg.json"))

	_ = (&ConfigInitCmd{}).Run()
	_ = (&ConfigSetCmd{Key: "greeting", Value: "hello"}).Run()

	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w

	err := (&ConfigShowCmd{}).Run()

	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatal("expected greeting in output")
	}
}

//
// Existing tests ------------------------------------------------------------
//
