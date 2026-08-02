// Package naming contains Go identifier and name-mangling helpers shared by
// the scaffold and OpenAPI SDK generators.
package naming

import (
	"strings"
	"unicode"
)

// ToCamelCase converts a string to UpperCamelCase, treating non-alphanumeric
// characters as word boundaries. Returns "DoRequest" when the result is empty.
func ToCamelCase(s string) string {
	var sb strings.Builder
	capitalize := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			capitalize = true
			continue
		}
		if capitalize {
			sb.WriteRune(unicode.ToUpper(r))
			capitalize = false
		} else {
			sb.WriteRune(r)
		}
	}
	res := sb.String()
	if res == "" {
		return "DoRequest"
	}
	return res
}

// ToLowerCamelCase converts a string to lowerCamelCase. Returns "param" when
// the result is empty.
func ToLowerCamelCase(s string) string {
	res := ToCamelCase(s)
	if res == "" {
		return "param"
	}
	return strings.ToLower(res[:1]) + res[1:]
}

// SanitizeIdentStart ensures an identifier does not begin with a digit (or is
// empty), prefixing it with the given fallback text when it would.
func SanitizeIdentStart(name, fallback string) string {
	if name == "" {
		return fallback
	}
	r := []rune(name)
	if unicode.IsDigit(r[0]) {
		return fallback + name
	}
	return name
}

// SanitizeFieldName converts a command segment into a valid, exported Go
// identifier (e.g. "foo-bar" -> "FooBar", "123admin" -> "X123admin").
func SanitizeFieldName(seg string) string {
	return SanitizeIdentStart(ToCamelCase(seg), "X")
}

// TitleCase upper-cases the first rune and lower-cases the rest.
func TitleCase(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(strings.ToLower(s))
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// LowerParamName produces a Go-legal, lower-camel-case parameter name.
func LowerParamName(s string) string {
	return SanitizeIdentStart(ToLowerCamelCase(s), "param")
}

// SanitizePkgName reduces a string to a valid Go package name.
func SanitizePkgName(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
