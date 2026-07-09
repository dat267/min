package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat267/min/cmd"
)

func TestAstScanCmd(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.go")

	content := `package sample

import "fmt"

type User struct {
	Name string
}

func Greet(name string) string {
	msg := fmt.Sprintf("Hello %s", name)
	return msg
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write sample.go failed: %v", err)
	}

	out := cmd.CaptureOutput(func() {
		scanCmd := &cmd.AstScanCmd{Path: filePath}
		if err := scanCmd.Run(); err != nil {
			t.Fatalf("AstScanCmd failed: %v", err)
		}
	})

	if !strings.Contains(out, "func Greet(name string) string") {
		t.Errorf("expected Greet function signature in output, got:\n%s", out)
	}
}

func TestAstTypesCmd(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.go")

	content := `package sample

type Account struct {
	ID string
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write sample.go failed: %v", err)
	}

	out := cmd.CaptureOutput(func() {
		typesCmd := &cmd.AstTypesCmd{Path: filePath}
		if err := typesCmd.Run(); err != nil {
			t.Fatalf("AstTypesCmd failed: %v", err)
		}
	})

	if !strings.Contains(out, "Account") {
		t.Errorf("expected Account type in output, got:\n%s", out)
	}
}

func TestAstFnCmd(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.go")

	content := `package sample

type Server struct{}

func (s *Server) Start() error {
	return nil
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write sample.go failed: %v", err)
	}

	out := cmd.CaptureOutput(func() {
		fnCmd := &cmd.AstFnCmd{
			Func: "Start",
			Path: filePath,
			Type: "Server",
		}
		if err := fnCmd.Run(); err != nil {
			t.Fatalf("AstFnCmd failed: %v", err)
		}
	})

	if !strings.Contains(out, "func (s *Server) Start()") {
		t.Errorf("expected Start method in output, got:\n%s", out)
	}
}
