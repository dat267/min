package astscan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScanDeterministic verifies that scanning a directory of Go files
// produces identical output on repeated runs (parallel processing must not
// reorder results) and covers every file.
func TestScanDeterministic(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		content := "package p\n\nfunc F" + strings.Repeat("x", i) + "() { println(1) }\n"
		if err := os.WriteFile(filepath.Join(dir, "f"+string(rune('a'+i))+".go"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	first := capture(t, func() error { return Scan(dir) })
	second := capture(t, func() error { return Scan(dir) })
	if first != second {
		t.Fatalf("output not deterministic:\n%s\n---\n%s", first, second)
	}
	for i := 0; i < 5; i++ {
		name := "f" + string(rune('a'+i)) + ".go"
		if !strings.Contains(first, name) {
			t.Errorf("output missing %s", name)
		}
	}
}

func TestScanStripsBodies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	content := "package p\n\nfunc Greet(name string) string {\n\treturn \"hi\"\n}\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	out := capture(t, func() error { return Scan(path) })
	if !strings.Contains(out, "func Greet(name string) string") {
		t.Errorf("missing signature: %s", out)
	}
	if strings.Contains(out, "hi") {
		t.Errorf("body not stripped: %s", out)
	}
}

func capture(t *testing.T, fn func() error) string {
	t.Helper()
	old := testOut
	defer func() { testOut = old }()
	var buf bytes.Buffer
	testOut = &buf
	if err := fn(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	return buf.String()
}
