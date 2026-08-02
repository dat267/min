package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	out := captureStdout(t, func() {
		scanCmd := &AstScanCmd{Path: filePath}
		if err := scanCmd.Run(); err != nil {
			t.Fatalf("AstScanCmd failed: %v", err)
		}
	})

	// Verify the body is stripped but the signature remains
	if !strings.Contains(out, "func Greet(name string) string") {
		t.Errorf("expected Greet function signature in output, got:\n%s", out)
	}
	if strings.Contains(out, "fmt.Sprintf") {
		t.Errorf("function body was not stripped:\n%s", out)
	}
	if !strings.Contains(out, "type User struct") {
		t.Errorf("expected User struct in output:\n%s", out)
	}
}

func TestAstTypesCmd(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.go")

	content := `package sample

const Version = "1.0.0"

type Account struct {
	ID string
}

func IgnoreMe() {}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write sample.go failed: %v", err)
	}

	out := captureStdout(t, func() {
		typesCmd := &AstTypesCmd{Path: filePath}
		if err := typesCmd.Run(); err != nil {
			t.Fatalf("AstTypesCmd failed: %v", err)
		}
	})

	// Verify only types are printed, not funcs or consts
	if !strings.Contains(out, "Account") {
		t.Errorf("expected Account type in output, got:\n%s", out)
	}
	if strings.Contains(out, "IgnoreMe") {
		t.Errorf("did not expect func IgnoreMe in types output:\n%s", out)
	}
	if strings.Contains(out, "Version") {
		t.Errorf("did not expect const Version in types output:\n%s", out)
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

	out := captureStdout(t, func() {
		fnCmd := &AstFnCmd{
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

func TestAstFnCmd_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.go")

	content := `package sample

type A struct{}
func (a *A) Process() {}

type B struct{}
func (b *B) Process() {}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write sample.go failed: %v", err)
	}

	t.Run("ambiguous without type", func(t *testing.T) {
		fnCmd := &AstFnCmd{Func: "Process", Path: filePath}
		err := fnCmd.Run()
		if err == nil {
			t.Fatal("expected error for ambiguous function name, got nil")
		}
		if !strings.Contains(err.Error(), "ambiguous matches") {
			t.Errorf("expected ambiguous matches error, got: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		fnCmd := &AstFnCmd{Func: "DoesNotExist", Path: filePath}
		err := fnCmd.Run()
		if err == nil {
			t.Fatal("expected error for missing function name, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not found error, got: %v", err)
		}
	})

	t.Run("resolve via dot notation", func(t *testing.T) {
		fnCmd := &AstFnCmd{Func: "B.Process", Path: filePath}
		if err := fnCmd.Run(); err != nil {
			t.Fatalf("failed to resolve B.Process using dot-notation: %v", err)
		}
	})
}
