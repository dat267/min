package cmd

import (
	"go/parser"
	"go/token"
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

func TestCmdAddCmd_HelpTextEscaping(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := (&CmdInitCmd{Name: "myapp"}).Run(); err != nil {
		t.Fatalf("init project failed: %v", err)
	}
	t.Chdir("myapp")

	desc := "Create `user`\nwith \"quotes\""
	addCmd := &CmdAddCmd{Name: "admin.users.create", Desc: desc}
	if err := addCmd.Run(); err != nil {
		t.Fatalf("cmd add failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join("cmd", "admin.go"))
	if err != nil {
		t.Fatalf("read admin.go: %v", err)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "admin.go", content, 0); err != nil {
		t.Fatalf("generated admin.go is not valid Go:\n%v\n%s", err, content)
	}
}

func TestCmdAddCmd_SubgroupUnderLeafCommand(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := (&CmdInitCmd{Name: "myapp"}).Run(); err != nil {
		t.Fatalf("init project failed: %v", err)
	}
	t.Chdir("myapp")

	if err := (&CmdAddCmd{Name: "admin"}).Run(); err != nil {
		t.Fatalf("cmd add admin failed: %v", err)
	}
	err := (&CmdAddCmd{Name: "admin.users.list"}).Run()
	if err == nil {
		t.Fatal("expected error when adding a subgroup under a leaf command")
	}
}

func TestCmdAddCmd_InvalidIdentifiers(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := (&CmdInitCmd{Name: "myapp"}).Run(); err != nil {
		t.Fatalf("init project failed: %v", err)
	}
	t.Chdir("myapp")

	parseAll := func(t *testing.T) {
		t.Helper()
		entries, err := os.ReadDir("cmd")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join("cmd", e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			if _, err := parser.ParseFile(fset, e.Name(), data, 0); err != nil {
				t.Fatalf("generated %s is not valid Go:\n%v\n%s", e.Name(), err, data)
			}
		}
	}

	for _, name := range []string{"foo-bar", "123admin"} {
		if err := (&CmdAddCmd{Name: name}).Run(); err != nil {
			t.Fatalf("cmd add %q failed: %v", name, err)
		}
		parseAll(t)
	}
}

func TestFindStructInFile_PrefixBoundary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")

	// A struct whose name merely starts with the same letters as the
	// segment must not be treated as a match.
	adminOnly := `package cmd

type Administrator struct{}
`
	if err := os.WriteFile(file, []byte(adminOnly), 0644); err != nil {
		t.Fatal(err)
	}
	if got := findStructInFile(file, "admin"); got != "" {
		t.Errorf("expected no match for 'admin', got %q", got)
	}

	// A struct whose name starts at a camelCase word boundary still matches.
	adminCmds := `package cmd

type AdminCommands struct{}
`
	if err := os.WriteFile(file, []byte(adminCmds), 0644); err != nil {
		t.Fatal(err)
	}
	if got := findStructInFile(file, "admin"); got != "AdminCommands" {
		t.Errorf("expected AdminCommands, got %q", got)
	}
}
