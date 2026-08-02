package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	err := (&CmdInitCmd{Name: "myapp"}).Run()
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
	t.Chdir(dir)

	_ = os.MkdirAll("myapp", 0755)
	err := (&CmdInitCmd{Name: "myapp"}).Run()
	if err == nil {
		t.Fatal("expected error for existing directory")
	}
}

func TestCmdAddCmd_NestedSingleFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	err := (&CmdInitCmd{Name: "myapp"}).Run()
	if err != nil {
		t.Fatalf("init project failed: %v", err)
	}

	t.Chdir("myapp")

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
	if !strings.Contains(content, "type AdminCmdGroup struct") && !strings.Contains(content, "type AdminCmd struct") {
		t.Errorf("expected AdminCmdGroup/AdminCmd struct in admin.go")
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
	t.Chdir(dir)

	_ = (&CmdInitCmd{Name: "myapp"}).Run()
	t.Chdir("myapp")

	out := captureStdout(t, func() {
		_ = (&CmdShowCmd{}).Run()
	})

	if !strings.Contains(out, "Commands:") {
		t.Errorf("expected Commands: in output, got: %s", out)
	}
}
