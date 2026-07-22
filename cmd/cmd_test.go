package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(fn func()) (string, error) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	out := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		out <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-out, nil
}

func TestGreetCmdDefaults(t *testing.T) {
	cmd := &Cmd{}
	g := &GreetCmd{Name: "World", Times: 1}
	output, err := captureStdout(func() { _ = g.Run(cmd) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Hello, World!") {
		t.Fatalf("got %q", output)
	}
	if !strings.Contains(output, cmd.CoreTimeout) {
		t.Fatalf("missing timeout in %q", output)
	}
}

func TestGreetCmdCustomName(t *testing.T) {
	g := &GreetCmd{Name: "Alice", Times: 1}
	output, err := captureStdout(func() { _ = g.Run(&Cmd{}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Hello, Alice!") {
		t.Fatalf("got %q", output)
	}
}

func TestGreetCmdShout(t *testing.T) {
	g := &GreetCmd{Name: "Alice", Times: 1, Shout: true}
	output, err := captureStdout(func() { _ = g.Run(&Cmd{}) })
	if err != nil {
		t.Fatal(err)
	}
	if output != strings.ToUpper(output) {
		t.Fatalf("got %q", output)
	}
}

func TestGreetCmdTimes(t *testing.T) {
	g := &GreetCmd{Name: "Alice", Times: 3}
	output, err := captureStdout(func() { _ = g.Run(&Cmd{}) })
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(output), "\n"); len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestGreetCmdCoreTimeout(t *testing.T) {
	cmd := &Cmd{CoreTimeout: "30m"}
	g := &GreetCmd{Name: "Alice", Times: 1}
	output, err := captureStdout(func() { _ = g.Run(cmd) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "30m") {
		t.Fatalf("got %q", output)
	}
}

func TestConfigInitCmdGenerate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.json")
	cmd := &Cmd{AdminToken: "tok", CoreTimeout: "5m", CoreRetries: 7}
	if err := (&ConfigInitCmd{}).Run(ConfigPath(path), cmd); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["admin-token"] != "tok" {
		t.Errorf("admin-token=%v", m["admin-token"])
	}
	if m["core-timeout"] != "5m" {
		t.Errorf("core-timeout=%v", m["core-timeout"])
	}
	if m["core-retries"] != float64(7) {
		t.Errorf("core-retries=%v", m["core-retries"])
	}
}

func TestConfigInitCmdForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.json")
	cmd := &Cmd{}
	c := &ConfigInitCmd{}
	if err := c.Run(ConfigPath(path), cmd); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(ConfigPath(path), cmd); err == nil {
		t.Fatal("expected error without --force")
	}
	if err := (&ConfigInitCmd{Force: true}).Run(ConfigPath(path), cmd); err != nil {
		t.Fatal(err)
	}
}

func TestConfigPathCmd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	output, err := captureStdout(func() { _ = (&ConfigPathCmd{}).Run(ConfigPath(path)) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "(does not exist)") {
		t.Fatalf("got %q", output)
	}

	path2 := filepath.Join(dir, "exists.json")
	os.WriteFile(path2, []byte("{}"), 0600)
	output2, _ := captureStdout(func() { _ = (&ConfigPathCmd{}).Run(ConfigPath(path2)) })
	if strings.TrimSpace(output2) != path2 {
		t.Fatalf("got %q", output2)
	}
}

func TestConfigShowCmd(t *testing.T) {
	cmd := &Cmd{AdminToken: "test-token", CoreTimeout: "5m", CoreRetries: 7, Debug: true}
	output, err := captureStdout(func() { _ = (&ConfigShowCmd{}).Run(cmd) })
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(output), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output)
	}
	if m["admin-token"] != "test-token" {
		t.Errorf("admin-token=%v", m["admin-token"])
	}
	if m["core-timeout"] != "5m" {
		t.Errorf("core-timeout=%v", m["core-timeout"])
	}
	if m["core-retries"] != float64(7) {
		t.Errorf("core-retries=%v", m["core-retries"])
	}
	if m["debug"] != true {
		t.Errorf("debug=%v", m["debug"])
	}
}
