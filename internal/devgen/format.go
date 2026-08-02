// Package devgen implements the `min dev` commands: converting HAR capture
// files to OpenAPI and generating Go SDK clients from OpenAPI specs.
package devgen

import "go/format"

// FormatSource gofmt-normalizes src and validates that it is syntactically
// valid Go. Generators call this before writing any .go file so that broken
// output fails at generation time instead of producing an uncompilable SDK.
func FormatSource(src []byte) ([]byte, error) {
	return format.Source(src)
}
