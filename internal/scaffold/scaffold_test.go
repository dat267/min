package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindStructInFile_PrefixBoundary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")

	adminOnly := `package cmd

type Administrator struct{}
`
	if err := os.WriteFile(file, []byte(adminOnly), 0644); err != nil {
		t.Fatal(err)
	}
	if got := FindStructInFile(file, "admin"); got != "" {
		t.Errorf("expected no match for 'admin', got %q", got)
	}

	adminCmds := `package cmd

type AdminCommands struct{}
`
	if err := os.WriteFile(file, []byte(adminCmds), 0644); err != nil {
		t.Fatal(err)
	}
	if got := FindStructInFile(file, "admin"); got != "AdminCommands" {
		t.Errorf("expected AdminCommands, got %q", got)
	}
}
