package devgen

import "testing"

func TestFormatSourceValid(t *testing.T) {
	src := []byte("package p\nfunc  X( ){}\n")
	out, err := FormatSource(src)
	if err != nil {
		t.Fatalf("FormatSource returned error for valid code: %v", err)
	}
	if string(out) != "package p\n\nfunc X() {}\n" {
		t.Errorf("unexpected formatted output:\n%s", out)
	}
}

func TestFormatSourceInvalid(t *testing.T) {
	_, err := FormatSource([]byte("package p\nfunc {"))
	if err == nil {
		t.Error("expected error for invalid Go, got nil")
	}
}
