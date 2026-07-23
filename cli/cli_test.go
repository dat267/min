package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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
type nameDefaultVal3 struct{ Name string `help:"X" default:"default-val"` }
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
	if err := os.WriteFile(path, []byte(`{"name": "from-config"}`), 0600); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(path, []byte(`{"core": {"timeout": "5m"}}`), 0600); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(path, []byte(`{"core-timeout": "30m"}`), 0600); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(path, []byte(`{"name": "from-config"}`), 0600); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(path, []byte(`{"name": null}`), 0600); err != nil {
		t.Fatal(err)
	}
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

func TestTimeDurationParsing(t *testing.T) {
	root := &struct {
		Interval time.Duration `help:"Poll interval" default:"5s"`
	}{}
	err := runTest([]string{"--interval=15m"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Interval != 15*time.Minute {
		t.Fatalf("got %v", root.Interval)
	}
}

type hasCtx struct{ captured context.Context }

func (h *hasCtx) Run(ctx context.Context) error {
	h.captured = ctx
	return nil
}

func TestPositionalSliceAccumulation(t *testing.T) {
	root := &struct {
		Files []string `arg:"" help:"Files to process"`
	}{}
	err := runTest([]string{"a.txt", "b.txt", "c.txt"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Files) != 3 {
		t.Fatalf("len=%d: %v", len(root.Files), root.Files)
	}
}

func TestMixedPositionals(t *testing.T) {
	root := &struct {
		Name  string   `help:"Name" arg:""`
		Files []string `help:"Files" arg:""`
	}{}
	err := runTest([]string{"Alice", "a.txt", "b.txt"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "Alice" {
		t.Fatalf("name=%q", root.Name)
	}
	if len(root.Files) != 2 || root.Files[0] != "a.txt" || root.Files[1] != "b.txt" {
		t.Fatalf("files=%v", root.Files)
	}
}

func TestInterfaceBinding(t *testing.T) {
	root := &hasCtx{}
	a := New(root, WithName("test"))
	a.Bind(context.Background())
	if err := a.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	if root.captured == nil {
		t.Fatal("context was not injected via interface")
	}
}

func TestKebabCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"CoreTimeout", "core-timeout"},
		{"AdminToken", "admin-token"},
		{"HTTPServer", "http-server"},
		{"MyJSONParser", "my-json-parser"},
		{"UserID", "user-id"},
		{"URLBuilder", "url-builder"},
		{"GetURL", "get-url"},
		{"ConfigPath", "config-path"},
		{"Debug", "debug"},
		{"DryRun", "dry-run"},
		{"Name", "name"},
		{"", ""},
		{"lowercase", "lowercase"},
		{"ALLUPPER", "allupper"},
	}
	for _, tc := range tests {
		got := kebab(tc.in)
		if got != tc.want {
			t.Errorf("kebab(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExplicitZeroNotOverriddenByEnv(t *testing.T) {
	t.Setenv("TEST_COUNT", "99")
	root := &struct {
		Count int `help:"C"`
	}{}
	err := runTest([]string{"--count=0"}, root, WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Count != 0 {
		t.Fatalf("explicit 0 overridden by env: got %d", root.Count)
	}
}

func TestExplicitFalseNotOverriddenByEnv(t *testing.T) {
	t.Setenv("TEST_VERBOSE", "true")
	root := &struct {
		Verbose bool `help:"V"`
	}{}
	err := runTest([]string{"--verbose=false"}, root, WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Verbose {
		t.Fatal("explicit false overridden by env")
	}
}

func TestExplicitZeroNotOverriddenByConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	if err := os.WriteFile(path, []byte(`{"count": 99}`), 0600); err != nil {
		t.Fatal(err)
	}
	root := &struct {
		Count int `help:"C" default:"42"`
	}{}
	err := runTest([]string{"--count=0"}, root, WithCfg(path))
	if err != nil {
		t.Fatal(err)
	}
	if root.Count != 0 {
		t.Fatalf("explicit 0 overridden by config/default: got %d", root.Count)
	}
}

func TestExplicitEmptyNotOverriddenByDefault(t *testing.T) {
	root := &struct {
		Name string `help:"N" default:"fallback"`
	}{}
	err := runTest([]string{"--name="}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "" {
		t.Fatalf("explicit empty string overridden by default: got %q", root.Name)
	}
}

func TestExplicitEmptyArgNotOverriddenByDefault(t *testing.T) {
	root := &struct {
		Name string `help:"N" default:"World" arg:""`
	}{}
	err := runTest([]string{""}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "" {
		t.Fatalf("explicit empty arg overridden by default: got %q", root.Name)
	}
}

func TestUintType(t *testing.T) {
	root := &struct {
		Port uint16 `help:"P" default:"8080"`
	}{}
	err := runTest([]string{"--port=3000"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Port != 3000 {
		t.Fatalf("got %d", root.Port)
	}
}

func TestFloatType(t *testing.T) {
	root := &struct {
		Ratio float64 `help:"R" default:"0.5"`
	}{}
	err := runTest([]string{"--ratio=1.25"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Ratio != 1.25 {
		t.Fatalf("got %v", root.Ratio)
	}
}

func TestExplicitZeroBoolCombined(t *testing.T) {
	root := &struct {
		Verbose bool `help:"V" short:"v"`
	}{}
	err := runTest([]string{"-vfalse"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Verbose {
		t.Fatal("explicit false via combined short flag not respected")
	}
}

func TestNilPointerCmdField(t *testing.T) {
	root := &struct {
		Optional *NoopCmd `cmd:""`
	}{}
	err := runTest([]string{}, root)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExplicitZeroCombinedShortNotOverriddenByEnv(t *testing.T) {
	t.Setenv("TEST_VERBOSE", "true")
	root := &struct {
		Verbose bool `help:"V" short:"v"`
	}{}
	err := runTest([]string{"-vfalse"}, root, WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Verbose {
		t.Fatal("explicit false via -vfalse overridden by env")
	}
}

func TestExplicitZeroOverridesDefaultForIntArg(t *testing.T) {
	root := &struct {
		Count int `help:"C" default:"5" arg:""`
	}{}
	err := runTest([]string{"0"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Count != 0 {
		t.Fatalf("explicit 0 arg overridden by default: got %d", root.Count)
	}
}

func TestUintDefault(t *testing.T) {
	root := &struct {
		Size uint `help:"S" default:"1024"`
	}{}
	err := runTest([]string{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Size != 1024 {
		t.Fatalf("got %d", root.Size)
	}
}

func TestExplicitInt64ZeroNotOverriddenByDefault(t *testing.T) {
	root := &struct {
		Limit int64 `help:"L" default:"100"`
	}{}
	err := runTest([]string{"--limit=0"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Limit != 0 {
		t.Fatalf("got %d", root.Limit)
	}
}

func TestDefaultSubcommandFlagDefaults(t *testing.T) {
	type sub struct {
		Name string `help:"Name" default:"World"`
		Run  func() error
	}
	subCmd := &sub{}
	subCmd.Run = func() error { return nil }
	root := &struct {
		Sub *sub `cmd:""`
	}{Sub: subCmd}
	err := runTest([]string{}, root, WithDefaultCmd("sub"))
	if err != nil {
		t.Fatal(err)
	}
	if subCmd.Name != "World" {
		t.Fatalf("default subcommand flag default not resolved: got %q", subCmd.Name)
	}
}

func TestDefaultSubcommandFlagFromEnv(t *testing.T) {
	t.Setenv("TEST_NAME", "from-env")
	type sub struct {
		Name string `help:"Name" default:"World"`
		Run  func() error
	}
	subCmd := &sub{}
	subCmd.Run = func() error { return nil }
	root := &struct {
		Sub *sub `cmd:""`
	}{Sub: subCmd}
	err := runTest([]string{}, root, WithDefaultCmd("sub"), WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if subCmd.Name != "from-env" {
		t.Fatalf("default subcommand flag env not resolved: got %q", subCmd.Name)
	}
}

func TestNewPanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil root")
		}
	}()
	New(nil)
}

func TestNewPanicsOnNonPointer(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-pointer root")
		}
	}()
	type s struct{}
	New(s{})
}

func TestVersionFlag(t *testing.T) {
	root := &namePlain{}
	err := runTest([]string{"--version"}, root, WithVersion("1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestConfigFileAlwaysInHelp(t *testing.T) {
	a := New(&namePlain{}, WithName("test"))
	r := a.build(a.root, nil)
	old := os.Stdout
	r2, w, _ := os.Pipe()
	os.Stdout = w
	a.help(r)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r2)
	out := buf.String()
	if !strings.Contains(out, "--config-file") {
		t.Fatalf("config-file help missing: %s", out)
	}
}

func TestKebabWithDigits(t *testing.T) {
	tests := []struct{ in, want string }{
		{"HTTP2Server", "http2-server"},
		{"UserID123", "user-id123"},
		{"Version2", "version2"},
		{"A1B2C3", "a1-b2-c3"},
	}
	for _, tc := range tests {
		got := kebab(tc.in)
		if got != tc.want {
			t.Errorf("kebab(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseEmptyArgs(t *testing.T) {
	root := &nameDefault{}
	err := runTest([]string{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "auto" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestParseOnlyDoubleDash(t *testing.T) {
	root := &struct {
		Name string `help:"N" arg:"" default:"World"`
	}{}
	err := runTest([]string{"--", "Alice"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "Alice" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestFlagWithSpaceValue(t *testing.T) {
	root := &onlyName{}
	err := runTest([]string{"--name", "hello world"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "hello world" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestResolveRespectsExplicitAfterAllLevels(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	if err := os.WriteFile(path, []byte(`{"count": 99}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_COUNT", "88")
	root := &struct {
		Count int `help:"C" default:"42"`
	}{}
	err := runTest([]string{"--count=0"}, root, WithCfg(path), WithEnv("TEST_"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Count != 0 {
		t.Fatalf("explicit CLI zero overridden by env/config/default: got %d", root.Count)
	}
}

func TestMixedFlagAndArgWithDefaults(t *testing.T) {
	root := &struct {
		Name  string `help:"N" default:"World" arg:""`
		Times int    `help:"T" default:"1"`
	}{}
	err := runTest([]string{"--times=5"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Times != 5 {
		t.Fatalf("times=%d", root.Times)
	}
	if root.Name != "World" {
		t.Fatalf("name=%q (arg default should apply)", root.Name)
	}
}

func TestBoolFlagShortAndLong(t *testing.T) {
	root := &struct {
		Verbose bool `help:"V" short:"v"`
		Debug   bool `help:"D" short:"d"`
	}{}
	err := runTest([]string{"-v", "--debug"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !root.Verbose || !root.Debug {
		t.Fatal("expected both true")
	}
}

func TestNewPanicsOnPointerToNonStruct(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for pointer to non-struct")
		}
	}()
	i := 42
	New(&i)
}

func TestHelpShowsBinNameWhenNoWithName(t *testing.T) {
	a := New(&namePlain{})
	r := a.build(a.root, nil)
	old := os.Stdout
	r2, w, _ := os.Pipe()
	os.Stdout = w
	a.help(r)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r2)
	out := buf.String()
	bin := filepath.Base(os.Args[0])
	if !strings.Contains(out, "Usage: "+bin) {
		t.Fatalf("expected Usage: %s, got: %s", bin, out)
	}
}

func TestVersionFlagWithoutVersion(t *testing.T) {
	root := &onlyName{}
	err := runTest([]string{"--version"}, root)
	if err == nil {
		t.Fatal("expected error for --version without WithVersion")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown flag, got %v", err)
	}
}

func TestFlagEqualsSignInValue(t *testing.T) {
	root := &onlyName{}
	err := runTest([]string{"--name=a=b=c"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "a=b=c" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestShortFlagWithEqualsSign(t *testing.T) {
	root := &nameShort{}
	err := runTest([]string{"-n=hello"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "hello" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestSetReturnValue(t *testing.T) {
	s := struct {
		Int   int
		Uint  uint
		Float float64
		Str   string
		Bool  bool
	}{}
	rv := reflect.ValueOf(&s).Elem()

	vInt := rv.FieldByName("Int")
	if !set(vInt, "42") {
		t.Fatal("set(int, 42) should succeed")
	}
	if s.Int != 42 {
		t.Fatalf("got %d", s.Int)
	}
	if set(vInt, "abc") {
		t.Fatal("set(int, abc) should fail")
	}

	vUint := rv.FieldByName("Uint")
	if !set(vUint, "99") {
		t.Fatal("set(uint, 99) should succeed")
	}
	if s.Uint != 99 {
		t.Fatalf("got %d", s.Uint)
	}
	if set(vUint, "-1") {
		t.Fatal("set(uint, -1) should fail")
	}

	vFloat := rv.FieldByName("Float")
	if !set(vFloat, "3.14") {
		t.Fatal("set(float, 3.14) should succeed")
	}
	if s.Float != 3.14 {
		t.Fatalf("got %v", s.Float)
	}
	if set(vFloat, "not-a-number") {
		t.Fatal("set(float, not-a-number) should fail")
	}

	vStr := rv.FieldByName("Str")
	if !set(vStr, "hello") {
		t.Fatal("set(string, hello) should succeed")
	}
	if s.Str != "hello" {
		t.Fatalf("got %q", s.Str)
	}

	vBool := rv.FieldByName("Bool")
	if !set(vBool, "true") {
		t.Fatal("set(bool, true) should succeed")
	}
	if !s.Bool {
		t.Fatal("expected true")
	}
	if !set(vBool, "false") {
		t.Fatal("set(bool, false) should succeed")
	}
	if s.Bool {
		t.Fatal("expected false")
	}
}

func TestYesFlagLongAndShort(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"--yes", []string{"--yes"}},
		{"-y", []string{"-y"}},
		{"--yes=1", []string{"--yes=1"}},
		{"--yes=false", []string{"--yes=false"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := &struct{ Name string `help:"N"` }{}
			err := runTest(tc.args, root, WithPrompt())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfigFileFlagWithEquals(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/mycfg.json"
	if err := os.WriteFile(path, []byte(`{"name": "from-cfg"}`), 0600); err != nil {
		t.Fatal(err)
	}
	root := &nameDefaultVal3{}
	err := runTest([]string{"--config-file=" + path}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "from-cfg" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestConfigFileFlagSpaceSeparated(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/mycfg.json"
	if err := os.WriteFile(path, []byte(`{"name": "from-cfg-space"}`), 0600); err != nil {
		t.Fatal(err)
	}
	root := &nameDefaultVal3{}
	err := runTest([]string{"--config-file", path}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "from-cfg-space" {
		t.Fatalf("got %q", root.Name)
	}
}

func TestConfigFileReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "cfg.json")
	root := &nameDefaultVal3{}
	err := runTest([]string{}, root, WithCfg(path))
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "default-val" {
		t.Fatalf("got %q (default should apply when config path inaccessible)", root.Name)
	}
}

func TestConfigFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	if err := os.WriteFile(path, []byte(`not json`), 0600); err != nil {
		t.Fatal(err)
	}
	root := &nameDefaultVal3{}
	err := runTest([]string{}, root, WithCfg(path))
	if err != nil {
		t.Fatal(err)
	}
	if root.Name != "default-val" {
		t.Fatalf("got %q (default should apply when config is invalid JSON)", root.Name)
	}
}

func TestSubcommandWithoutRunMethod(t *testing.T) {
	root := &struct {
		Sub struct {
			Name string `help:"N" default:"World"`
		} `cmd:""`
	}{}
	err := runTest([]string{"sub"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Sub.Name != "World" {
		t.Fatalf("expected default applied, got %q", root.Sub.Name)
	}
}

func TestDispatchAutoNewPointerParam(t *testing.T) {
	root := &struct {
		Sub struct {
			Name string `help:"N"`
		} `cmd:""`
	}{}
	err := runTest([]string{"sub"}, root)
	if err != nil {
		t.Fatalf("subcommand without Run should succeed: %v", err)
	}
}

func TestHelpWithDescription(t *testing.T) {
	a := New(&namePlain{}, WithName("test"), WithDesc("A test tool"))
	r := a.build(a.root, nil)
	old := os.Stdout
	r2, w, _ := os.Pipe()
	os.Stdout = w
	a.help(r)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r2)
	out := buf.String()
	if !strings.Contains(out, "A test tool") {
		t.Fatalf("expected description in help: %s", out)
	}
}

func TestHelpWithPrompt(t *testing.T) {
	a := New(&namePlain{}, WithName("test"), WithPrompt())
	r := a.build(a.root, nil)
	old := os.Stdout
	r2, w, _ := os.Pipe()
	os.Stdout = w
	a.help(r)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r2)
	out := buf.String()
	if !strings.Contains(out, "--yes") {
		t.Fatalf("expected -y/--yes in help: %s", out)
	}
}

func TestInvalidSingleDashFlag(t *testing.T) {
	root := &onlyName{}
	err := runTest([]string{"-"}, root)
	if err == nil {
		t.Fatal("expected error for bare '-'")
	}
	if !strings.Contains(err.Error(), "invalid flag") {
		t.Fatalf("got %v", err)
	}
}

func TestSubcommandWithNoRunShowsHelp(t *testing.T) {
	type group struct {
		Sub struct {
			Name string `help:"N"`
		} `cmd:"" help:"nested"`
	}
	root := &group{}
	err := runTest([]string{"sub"}, root)
	if err != nil {
		t.Fatalf("expected nil for subcommand group with no Run, got %v", err)
	}
}

func TestFlagConsumesConfigFile(t *testing.T) {
	a := New(&onlyName{})
	deep := a.allFlagsDeep(a.build(a.root, nil))
	got := a.flagConsumesNext("--config-file", []string{"--config-file", "path.json"}, 0, deep)
	if !got {
		t.Fatal("config-file should consume next arg")
	}
}

func TestFlagNoConsumeNextIsDashDash(t *testing.T) {
	a := New(&onlyName{})
	deep := a.allFlagsDeep(a.build(a.root, nil))
	got := a.flagConsumesNext("--name", []string{"--name", "--"}, 0, deep)
	if got {
		t.Fatal("should not consume when next is --")
	}
}

func TestSetSliceUnsupportedTypeReturnsFalse(t *testing.T) {
	s := struct {
		Nums []int
	}{}
	rv := reflect.ValueOf(&s).Elem()
	if set(rv.FieldByName("Nums"), "42") {
		t.Fatal("set([]int, ...) should return false")
	}
}

func TestResolveWithInvalidConfigValue(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.json"
	if err := os.WriteFile(path, []byte(`{"count": "not-a-number"}`), 0600); err != nil {
		t.Fatal(err)
	}
	root := &struct {
		Count int `help:"C" default:"42"`
	}{}
	err := runTest([]string{}, root, WithCfg(path))
	if err != nil {
		t.Fatal(err)
	}
	if root.Count != 0 {
		t.Fatalf("expected 0 (invalid config value shadows default), got %d", root.Count)
	}
}

func TestHelpWithArgsDisplay(t *testing.T) {
	a := New(&struct {
		Optional string `help:"opt" arg:""`
		Required string `help:"req" required:"" arg:""`
		Default  string `help:"def" default:"x" arg:""`
	}{}, WithName("test"))
	r := a.build(a.root, nil)
	old := os.Stdout
	rd, wr, _ := os.Pipe()
	os.Stdout = wr
	a.help(r)
	_ = wr.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(rd)
	out := buf.String()
	if !strings.Contains(out, "[OPTIONAL]") {
		t.Errorf("missing optional arg: %s", out)
	}
	if !strings.Contains(out, "<REQUIRED>") {
		t.Errorf("missing required arg: %s", out)
	}
	if !strings.Contains(out, "[DEFAULT=x]") {
		t.Errorf("missing default arg: %s", out)
	}
}

func TestHelpWithSubcommands(t *testing.T) {
	a := New(&struct {
		Sub1 struct{} `cmd:"" help:"first subcommand"`
		Sub2 struct{} `cmd:"" help:"second subcommand"`
	}{}, WithName("test"))
	r := a.build(a.root, nil)
	old := os.Stdout
	rd, wr, _ := os.Pipe()
	os.Stdout = wr
	a.help(r)
	_ = wr.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(rd)
	out := buf.String()
	if !strings.Contains(out, "sub1") || !strings.Contains(out, "sub2") {
		t.Errorf("missing subcommands: %s", out)
	}
	if !strings.Contains(out, "first subcommand") || !strings.Contains(out, "second subcommand") {
		t.Errorf("missing subcommand help: %s", out)
	}
}

type runErrorCmd struct{}

func (r *runErrorCmd) Run() error { return fmt.Errorf("run error") }

func TestDispatchRunMethodReturnsError(t *testing.T) {
	c := &runErrorCmd{}
	a := New(c, WithName("test"))
	err := a.Parse([]string{})
	if err == nil || !strings.Contains(err.Error(), "run error") {
		t.Fatalf("expected run error, got %v", err)
	}
}

type autoParamCmd struct {
	capturedStr *string
	capturedInt *int
}

func (c *autoParamCmd) Run(s string, n *int) error {
	c.capturedStr = &s
	c.capturedInt = n
	return nil
}

func TestDispatchAutoParams(t *testing.T) {
	c := &autoParamCmd{}
	a := New(c, WithName("test"))
	if err := a.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	if c.capturedStr == nil || *c.capturedStr != "" {
		t.Fatalf("expected zero string, got %v", c.capturedStr)
	}
	if c.capturedInt == nil || *c.capturedInt != 0 {
		t.Fatalf("expected auto-allocated zero int, got %v", c.capturedInt)
	}
}

func TestInjectConfigPathUpdatesBind(t *testing.T) {
	type ConfigPath string
	type CfgCmd struct {
		ConfigPath ConfigPath `json:"-"`
		Name       string     `help:"N"`
	}
	root := &CfgCmd{}
	a := New(root, WithName("test"), WithCfg("/some/path.json"), WithConfigField("ConfigPath"))
	a.Bind(ConfigPath(""))
	if err := a.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	if string(root.ConfigPath) != "/some/path.json" {
		t.Errorf("expected ConfigPath updated, got %q", root.ConfigPath)
	}
}

func TestHelpWithFlagEnvVar(t *testing.T) {
	a := New(&struct {
		Token string `help:"API token"`
	}{}, WithName("test"), WithEnv("MY_"))
	r := a.build(a.root, nil)
	old := os.Stdout
	rd, wr, _ := os.Pipe()
	os.Stdout = wr
	a.help(r)
	_ = wr.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(rd)
	out := buf.String()
	if !strings.Contains(out, "MY_TOKEN") {
		t.Errorf("expected MY_TOKEN env in help: %s", out)
	}
}
