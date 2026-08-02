package naming

import "testing"

func TestToCamelCase(t *testing.T) {
	cases := map[string]string{
		"foo-bar":      "FooBar",
		"foo_bar":      "FooBar",
		"/v1/users":    "V1Users",
		"":             "DoRequest",
		"alreadyUpper": "AlreadyUpper",
	}
	for in, want := range cases {
		if got := ToCamelCase(in); got != want {
			t.Errorf("ToCamelCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToLowerCamelCase(t *testing.T) {
	if got := ToLowerCamelCase("foo-bar"); got != "fooBar" {
		t.Errorf("ToLowerCamelCase(foo-bar) = %q, want fooBar", got)
	}
}

func TestSanitizeIdentStart(t *testing.T) {
	if got := SanitizeIdentStart("123foo", "Do"); got != "Do123foo" {
		t.Errorf("SanitizeIdentStart(123foo) = %q, want Do123foo", got)
	}
	if got := SanitizeIdentStart("", "X"); got != "X" {
		t.Errorf("SanitizeIdentStart(empty) = %q, want X", got)
	}
	if got := SanitizeIdentStart("ok", "X"); got != "ok" {
		t.Errorf("SanitizeIdentStart(ok) = %q, want ok", got)
	}
}

func TestSanitizeFieldName(t *testing.T) {
	if got := SanitizeFieldName("admin.users"); got != "AdminUsers" {
		t.Errorf("SanitizeFieldName(admin.users) = %q, want AdminUsers", got)
	}
}

func TestTitleCase(t *testing.T) {
	if got := TitleCase("get"); got != "Get" {
		t.Errorf("TitleCase(get) = %q, want Get", got)
	}
}

func TestSanitizePkgName(t *testing.T) {
	if got := SanitizePkgName("my-custom_sdk!"); got != "mycustom_sdk" {
		t.Errorf("SanitizePkgName = %q, want mycustom_sdk", got)
	}
}
