package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

type TestCmd struct {
	Name  string `help:"Name" default:"World" arg:""`
	Times int    `help:"Times" default:"1" short:"t"`
	Shout bool   `help:"Shout" short:"s"`
	Opt   string `help:"Optional"`
	Req   string `help:"Required" required:""`
}

func (t *TestCmd) Run() error {
	msg := fmt.Sprintf("Hello %s x%d shout=%v opt=%s req=%s", t.Name, t.Times, t.Shout, t.Opt, t.Req)
	fmt.Println(msg)
	return nil
}

type NoopCmd struct{}

func (n *NoopCmd) Run() error { return nil }

func runTest(args []string, root any, opts ...Option) error {
	a := New(root, opts...)
	return a.Parse(args)
}

type onlyName struct{ Name string `help:"X"` }
type nameDefault struct{ Name string `help:"X" default:"auto"` }
type nameShort struct{ Name string `help:"X" short:"n"` }
type nameDefaultEnv struct{ Name string `help:"X" default:"auto"` }
type nameDefaultVal2 struct{ Name string `help:"X" default:"default-val"` }
type namePlain struct{ Name string `help:"X"` }
type nameArg struct{ Name string `help:"X" arg:""` }
type timeShout struct {
	Times int  `help:"T" short:"t"`
	Shout bool `help:"S" short:"s"`
	Name  string `help:"X" arg:""`
}
type nameString struct{ Name string `help:"X"` }
type nameDefaultVal3 struct{ Name string `help:"X" default:"default-val"` }
type nameEnv struct{ Name string `help:"X"` }
type nameCoreTimeout struct{ CoreTimeout string `help:"T" default:"10s"` }

func TestFlagParsing(t *testing.T) {
	t.Run("long flag", func(t *testing.T) {
		root := &onlyName{Name: ""}
		err := runTest([]string{"--name", "hello"}, root)
		if err != nil {
			t.Fatal(err)
		}
		if root.Name != "hello" {
			t.Fatalf("got %q", root.Name)
		}
	})
	t.Run("short flag", func(t *testing.T) {
		root := &nameShort{}
		err := runTest([]string{"-n", "hello"}, root)
		if err != nil {
			t.Fatal(err)
		}
		if root.Name != "hello" {
			t.Fatalf("got %q", root.Name)
		}
	})
	t.Run("flag with =", func(t *testing.T) {
		root := &onlyName{}
		err := runTest([]string{"--name=hello"}, root)
		if err != nil {
			t.Fatal(err)
		}
		if root.Name != "hello" {
			t.Fatalf("got %q", root.Name)
		}
	})
	t.Run("bool flag", func(t *testing.T) {
		root := &struct {
			Verbose bool `help:"V" short:"v"`
		}{}
		err := runTest([]string{"-v"}, root)
		if err != nil {
			t.Fatal(err)
		}
		if !root.Verbose {
			t.Fatal("expected verbose=true")
		}
	})
	t.Run("bool flag false", func(t *testing.T) {
		root := &struct {
			Verbose bool `help:"V" short:"v"`
		}{}
		err := runTest([]string{"--verbose=false"}, root)
		if err != nil {
			t.Fatal(err)
		}
		if root.Verbose {
			t.Fatal("expected verbose=false")
		}
	})
	t.Run("default value", func(t *testing.T) {
		root := &nameDefault{}
		err := runTest([]string{}, root)
		if err != nil {
			t.Fatal(err)
		}
		if root.Name != "auto" {
			t.Fatalf("got %q", root.Name)
		}
	})
	t.Run("default overridden by flag", func(t *testing.T) {
		root := &nameDefault{}
		err := runTest([]string{"--name=manual"}, root)
		if err != nil {
			t.Fatal(err)
		}
		if root.Name != "manual" {
			t.Fatalf("got %q", root.Name)
		}
	})
}

func TestEnvVarResolution(t *testing.T) {
	t.Setenv("TEST_NAME", "from-env")
	root := &nameDefaultEnv{}
	err := runTest([]string{}, root, WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "from-env" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestEnvOverridesDefault(t *testing.T) {
	t.Setenv("TEST_NAME", "env-val")
	root := &nameDefaultVal2{}
	err := runTest([]string{}, root, WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "env-val" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestFlagOverridesEnv(t *testing.T) {
	t.Setenv("TEST_NAME", "env-val")
	root := &namePlain{}
	err := runTest([]string{"--name=cli-val"}, root, WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "cli-val" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestPositionalArg(t *testing.T) {
	root := &nameArg{}
	err := runTest([]string{"hello"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "hello" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestSubcommand(t *testing.T) {
	root := &struct{ Greet TestCmd `cmd:""` }{}
	err := runTest([]string{"greet", "--req=abc"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Greet.Req != "abc" {
		t.Fatalf("req=%q", root.Greet.Req)
	}
}

func TestSubcommandPositional(t *testing.T) {
	root := &struct{ Greet TestCmd `cmd:""` }{}
	err := runTest([]string{"greet", "Alice", "--req=abc"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Greet.Name != "Alice" {
		t.Fatalf("got %q", root.Greet.Name)
	}
	if root.Greet.Req != "abc" {
		t.Fatalf("req=%q", root.Greet.Req)
	}
}

func TestInterleavedFlagsAndPositionals(t *testing.T) {
	root := &timeShout{}
	err := runTest([]string{"-t", "3", "Alice", "-s"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "Alice" {
		t.Fatalf("name=%q", root.Name)
	}
	if root.Times != 3 {
		t.Fatalf("times=%d", root.Times)
	}
	if !root.Shout {
		t.Fatal("expected shout=true")
	}
}

func TestUnknownFlag(t *testing.T) {
	root := &namePlain{}
	err := runTest([]string{"--unknown"}, root)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("got %v", err)
	}
}

func TestFlagValueConsumesNextArg(t *testing.T) {
	root := &struct {
		Name string `help:"X"`
		Arg  string `help:"A" arg:""`
	}{}
	err := runTest([]string{"--name", "value", "pos"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "value" {
		t.Fatalf("name=%q", root.Name)
	}
	if root.Arg != "pos" {
		t.Fatalf("arg=%q", root.Arg)
	}
}

func TestFlagValueLooksLikeFlag(t *testing.T) {
	root := &struct {
		Name string `help:"X"`
		Arg  string `help:"A" arg:""`
	}{}
	err := runTest([]string{"--name", "-value", "pos"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "-value" {
		t.Fatalf("name=%q", root.Name)
	}
	if root.Arg != "pos" {
		t.Fatalf("arg=%q", root.Arg)
	}
}

func TestDoubleDash(t *testing.T) {
	root := &struct {
		Name string `help:"X"`
		Arg  string `help:"A" arg:""`
	}{}
	err := runTest([]string{"--", "--name", "hidden"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "" {
		t.Fatalf("name=%q", root.Name)
	}
	if root.Arg != "--name" {
		t.Fatalf("arg=%q", root.Arg)
	}
}

func TestGlobalFlagInSubcommand(t *testing.T) {
	root := &struct {
		Verbose bool     `help:"V" short:"v"`
		Greet   TestCmd  `cmd:""`
	}{}
	err := runTest([]string{"greet", "--verbose", "--req=abc"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !root.Verbose {
		t.Fatal("expected verbose=true from global flag in subcommand")
	}
}

func TestGlobalFlagBeforeSubcommand(t *testing.T) {
	root := &struct {
		Verbose bool     `help:"V" short:"v"`
		Greet   TestCmd  `cmd:""`
	}{}
	err := runTest([]string{"--verbose", "greet", "--req=abc"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !root.Verbose {
		t.Fatal("expected verbose=true from global flag before subcommand")
	}
}

func TestConfigFileResolution(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	os.WriteFile(path, []byte(`{"name": "from-config"}`), 0600)
	root := &nameDefaultVal3{}
	err := runTest([]string{}, root, WithCfg(path))
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "from-config" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestConfigNestedResolution(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	os.WriteFile(path, []byte(`{"core": {"timeout": "5m"}}`), 0600)
	root := &nameCoreTimeout{}
	err := runTest([]string{}, root, WithCfg(path))
	if err != nil {
		t.Fatal(err)
	}
	if root.CoreTimeout != "5m" {
		t.Fatalf("got %q", root.CoreTimeout)
	}
}

func TestConfigFlatResolution(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	os.WriteFile(path, []byte(`{"core-timeout": "30m"}`), 0600)
	root := &nameCoreTimeout{}
	err := runTest([]string{}, root, WithCfg(path))
	if err != nil {
		t.Fatal(err)
	}
	if root.CoreTimeout != "30m" {
		t.Fatalf("got %q", root.CoreTimeout)
	}
}

func TestEnvOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	os.WriteFile(path, []byte(`{"name": "from-config"}`), 0600)
	t.Setenv("TEST_NAME", "from-env")
	root := &nameDefaultVal3{}
	err := runTest([]string{}, root, WithCfg(path), WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "from-env" {
		t.Fatalf("got %q (expected env to override config)", root.Name)
	}
}

func TestNullInConfigFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	os.WriteFile(path, []byte(`{"name": null}`), 0600)
	root := &nameDefaultVal3{}
	err := runTest([]string{}, root, WithCfg(path))
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "default-val" {
		t.Fatalf("got %q (expected null to fall back to default)", root.Name)
	}
}

func TestCombinedShortFlags(t *testing.T) {
	root := &struct {
		Verbose bool `help:"V" short:"v"`
		All     bool `help:"A" short:"a"`
		Debug   bool `help:"D" short:"d"`
	}{}
	err := runTest([]string{"-vad"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !root.Verbose || !root.All || !root.Debug {
		t.Fatal("expected all flags true")
	}
}

func TestCombinedShortWithValue(t *testing.T) {
	root := &struct {
		Name string `help:"N" short:"n"`
	}{}
	err := runTest([]string{"-nAlice"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "Alice" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestCombinedShortMixed(t *testing.T) {
	root := &struct {
		Shout bool   `help:"S" short:"s"`
		Name  string `help:"N" short:"n"`
		Times int    `help:"T" short:"t"`
	}{}
	err := runTest([]string{"-snAlice", "-t", "3"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !root.Shout {
		t.Fatal("expected shout=true")
	}
	if root.Name != "Alice" {
		t.Fatalf("name=%q", root.Name)
	}
	if root.Times != 3 {
		t.Fatalf("times=%d", root.Times)
	}
}

func TestSliceAccumulation(t *testing.T) {
	root := &struct {
		Names []string `help:"N" short:"n"`
	}{}
	err := runTest([]string{"--names", "a", "--names", "b", "-n", "c"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Names) != 3 {
		t.Fatalf("len=%d: %v", len(root.Names), root.Names)
	}
	if root.Names[0] != "a" || root.Names[1] != "b" || root.Names[2] != "c" {
		t.Fatalf("got %v", root.Names)
	}
}

type runTracker struct{ ran bool }

func (r *runTracker) Run() error { r.ran = true; return nil }

func TestDefaultSubcommand(t *testing.T) {
	rt := &runTracker{}
	root := &struct {
		Default *runTracker `cmd:""`
		Other   NoopCmd     `cmd:""`
	}{Default: rt}
	err := runTest([]string{}, root, WithDefaultCmd("default"))
	if err != nil {
		t.Fatal(err)
	}
	if !rt.ran {
		t.Fatal("default subcommand was not executed")
	}
}

func TestDefaultSubcommandWithArgs(t *testing.T) {
	rt := &runTracker{}
	root := &struct {
		Default *runTracker `cmd:""`
		Other   NoopCmd     `cmd:""`
	}{Default: rt}
	// Explicit subcommand should still work
	err := runTest([]string{"other"}, root, WithDefaultCmd("default"))
	if err != nil {
		t.Fatal(err)
	}
	if rt.ran {
		t.Fatal("default should not run when explicit subcommand given")
	}
}

type DeepCmd struct {
	Level int
}

func (d *DeepCmd) Run() error { d.Level++; return nil }

func TestDeeplyNestedSubcommands(t *testing.T) {
	dc := &DeepCmd{}
	root := &struct {
		A struct {
			B struct {
				C *DeepCmd `cmd:""`
			} `cmd:""`
		} `cmd:""`
	}{}
	root.A.B.C = dc
	err := runTest([]string{"a", "b", "c"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if dc.Level != 1 {
		t.Fatal("deep nested command not executed")
	}
}

func TestDeeplyNestedHelp(t *testing.T) {
	root := &struct {
		A struct {
			B struct {
				C struct {
					D string `help:"deep flag"`
				} `cmd:""`
			} `cmd:""`
		} `cmd:""`
	}{}
	err := runTest([]string{"a", "b", "c", "-h"}, root)
	if err != nil {
		t.Fatalf("expected nil (help shown), got %v", err)
	}
}
