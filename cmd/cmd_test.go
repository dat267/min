package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd(); _ = oldWd
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	err := (&InitCmd{Name: "myapp"}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("myapp/main.go"); os.IsNotExist(err) {
		t.Fatal("main.go not created")
	}
	if _, err := os.Stat("myapp/go.mod"); os.IsNotExist(err) {
		t.Fatal("go.mod not created")
	}
}

func TestInitCmdExistingDir(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd(); _ = oldWd
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	_ = os.MkdirAll("myapp", 0755)
	err := (&InitCmd{Name: "myapp"}).Run()
	if err == nil {
		t.Fatal("expected error for existing directory")
	}
}

func TestConfigInit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	if err := (&ConfigInitCmd{}).Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Fatal("config not created")
	}
}

func TestConfigInitForce(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	c := &ConfigInitCmd{}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err == nil {
		t.Fatal("expected error without --overwrite")
	}
	if err := (&ConfigInitCmd{Overwrite: true}).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSetAndShow(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	_ = (&ConfigInitCmd{}).Run()
	if err := (&ConfigSetCmd{Key: "greeting", Value: "hello"}).Run(); err != nil {
		t.Fatal(err)
	}
	if err := (&ConfigSetCmd{Key: "core.timeout", Value: "5m"}).Run(); err != nil {
		t.Fatal(err)
	}
	if err := (&ConfigShowCmd{}).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigUnset(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	_ = (&ConfigInitCmd{}).Run()
	_ = (&ConfigSetCmd{Key: "greeting", Value: "hello"}).Run()
	if err := (&ConfigUnsetCmd{Key: "greeting"}).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigUnsetNested(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	_ = (&ConfigInitCmd{}).Run()
	_ = (&ConfigSetCmd{Key: "core.timeout", Value: "5m"}).Run()
	if err := (&ConfigUnsetCmd{Key: "core.timeout"}).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigShowNoFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	if err := (&ConfigShowCmd{}).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSetAutoCreate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	if err := (&ConfigSetCmd{Key: "x", Value: "1"}).Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Fatal("config not auto-created on set")
	}
}

func TestConfigNestedOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	_ = (&ConfigInitCmd{}).Run()
	_ = (&ConfigSetCmd{Key: "a.b.c", Value: "deep"}).Run()
	if err := (&ConfigSetCmd{Key: "a.b", Value: "replace"}).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigShowAfterSet(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.json")
	SetConfigPath(p)
	defer SetConfigPath("")
	_ = (&ConfigInitCmd{}).Run()
	_ = (&ConfigSetCmd{Key: "greeting", Value: "hello"}).Run()

	out := CaptureOutput(func() {
		_ = (&ConfigShowCmd{}).Run()
	})
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected greeting in output, got %s", out)
	}
}

func CaptureOutput(fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestCmdAddCmd_NestedSingleFile(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	err := (&InitCmd{Name: "myapp"}).Run()
	if err != nil {
		t.Fatalf("init project failed: %v", err)
	}

	_ = os.Chdir("myapp")

	addCmd := &CmdAddCmd{Name: "admin.users.create", Desc: "Create admin user"}
	if err := addCmd.Run(); err != nil {
		t.Fatalf("cmd add admin.users.create failed: %v", err)
	}

	adminPath := filepath.Join("cmd", "admin.go")
	contentBytes, err := os.ReadFile(adminPath)
	if err != nil {
		t.Fatalf("expected %s to be created, got error: %v", adminPath, err)
	}

	content := string(contentBytes)
	if !strings.Contains(content, "type AdminCmd struct") {
		t.Errorf("expected AdminCmd struct in admin.go")
	}
	if !strings.Contains(content, "type UsersCmd struct") {
		t.Errorf("expected UsersCmd struct in admin.go")
	}
	if !strings.Contains(content, "type CreateCmd struct") {
		t.Errorf("expected CreateCmd struct in admin.go")
	}

	if _, err := os.Stat(filepath.Join("cmd", "users.go")); err == nil {
		t.Errorf("cmd/users.go should not exist")
	}
	if _, err := os.Stat(filepath.Join("cmd", "create.go")); err == nil {
		t.Errorf("cmd/create.go should not exist")
	}
}

func TestCmdShowCmd(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	_ = (&InitCmd{Name: "myapp"}).Run()
	_ = os.Chdir("myapp")

	out := CaptureOutput(func() {
		_ = (&CmdShowCmd{}).Run()
	})

	if !strings.Contains(out, "Commands:") {
		t.Errorf("expected Commands: in output, got: %s", out)
	}
}

