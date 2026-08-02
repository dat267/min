package cmd

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
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
	// The scaffold runtime and config files are copied verbatim.
	for _, f := range []string{"cmd/cmd.go", "cmd/runtime.go", "cmd/config.go"} {
		if _, err := os.Stat(filepath.Join("myapp", f)); os.IsNotExist(err) {
			t.Fatalf("%s not created", f)
		}
	}
}

func TestInitCmd_GeneratedProjectRuns(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := (&CmdInitCmd{Name: "myapp"}).Run(); err != nil {
		t.Fatalf("init project failed: %v", err)
	}

	var out bytes.Buffer
	cmd := exec.Command("go", "run", ".", "greet")
	cmd.Dir = filepath.Join(dir, "myapp")
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("generated project failed to run: %v", err)
	}
	if !strings.Contains(out.String(), "Hello, World!") {
		t.Errorf("expected greeting, got: %s", out.String())
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

func TestCmdAddCmd_GroupFlag(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := (&CmdInitCmd{Name: "myapp"}).Run(); err != nil {
		t.Fatalf("init project failed: %v", err)
	}
	t.Chdir("myapp")

	if err := (&CmdAddCmd{Name: "z", Group: true}).Run(); err != nil {
		t.Fatalf("cmd add --group z failed: %v", err)
	}

	zData, err := os.ReadFile(filepath.Join("cmd", "z.go"))
	if err != nil {
		t.Fatalf("expected cmd/z.go: %v", err)
	}
	zContent := string(zData)
	if !strings.Contains(zContent, "type ZCmd struct {") {
		t.Errorf("expected ZCmd struct in cmd/z.go:\n%s", zContent)
	}
	if strings.Contains(zContent, "func (c *ZCmd) Run()") {
		t.Errorf("group ZCmd must not have a Run method:\n%s", zContent)
	}

	cmdData, err := os.ReadFile(filepath.Join("cmd", "cmd.go"))
	if err != nil {
		t.Fatalf("read cmd.go: %v", err)
	}
	if !strings.Contains(string(cmdData), `Z ZCmd `+"`"+`cmd:"" help:"z command group"`+"`") {
		t.Errorf("expected registered group field with help \"z command group\":\n%s", cmdData)
	}

	// A leaf can now be added under the group.
	if err := (&CmdAddCmd{Name: "z.s"}).Run(); err != nil {
		t.Fatalf("cmd add z.s under group z failed: %v", err)
	}

	// Re-adding an existing group is a silent no-op success.
	if err := (&CmdAddCmd{Name: "z", Group: true}).Run(); err != nil {
		t.Fatalf("re-adding existing group z failed: %v", err)
	}
}

func TestCmdAddCmd_GroupFlagErrorsOnLeaf(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := (&CmdInitCmd{Name: "myapp"}).Run(); err != nil {
		t.Fatalf("init project failed: %v", err)
	}
	t.Chdir("myapp")

	if err := (&CmdAddCmd{Name: "z"}).Run(); err != nil {
		t.Fatalf("cmd add z failed: %v", err)
	}
	err := (&CmdAddCmd{Name: "z", Group: true}).Run()
	if err == nil {
		t.Fatal("expected error adding group over an existing leaf")
	}
	if !strings.Contains(err.Error(), "cannot add group") {
		t.Errorf("expected 'cannot add group' error, got: %v", err)
	}
}

func TestCmdAddCmd_GroupFlagNestedAndDesc(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := (&CmdInitCmd{Name: "myapp"}).Run(); err != nil {
		t.Fatalf("init project failed: %v", err)
	}
	t.Chdir("myapp")

	if err := (&CmdAddCmd{Name: "a.b", Group: true, Desc: "B ops"}).Run(); err != nil {
		t.Fatalf("cmd add --group a.b failed: %v", err)
	}

	aData, err := os.ReadFile(filepath.Join("cmd", "a.go"))
	if err != nil {
		t.Fatalf("expected cmd/a.go: %v", err)
	}
	aContent := string(aData)
	if !strings.Contains(aContent, "type ACmd struct {") {
		t.Errorf("expected group ACmd in cmd/a.go:\n%s", aContent)
	}
	if !strings.Contains(aContent, "type BCmd struct {") {
		t.Errorf("expected group BCmd in cmd/a.go:\n%s", aContent)
	}
	if strings.Contains(aContent, "func (c *BCmd) Run()") {
		t.Errorf("group BCmd must not have a Run method:\n%s", aContent)
	}

	cmdData, err := os.ReadFile(filepath.Join("cmd", "cmd.go"))
	if err != nil {
		t.Fatalf("read cmd.go: %v", err)
	}
	if !strings.Contains(string(cmdData), `A ACmd `+"`"+`cmd:"" help:"a command group"`+"`") {
		t.Errorf("expected ACmd registered with default group help:\n%s", cmdData)
	}
	// BCmd lives in a.go registered under ACmd with the desc help text.
	if !strings.Contains(aContent, `B BCmd `+"`"+`cmd:"" help:"B ops"`+"`") {
		t.Errorf("expected BCmd registered with desc help \"B ops\":\n%s", aContent)
	}
}

