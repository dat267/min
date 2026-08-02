# Scalability Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `min` structurally scalable: single source of truth for the scaffold runtime, gofmt-validated code generation, a thin `cmd` package delegating to internal packages, parallel AST scanning, and no duplicated runtime code.

**Architecture:** Move non-Kong logic out of the monolithic `cmd` package into `internal/naming`, `internal/astscan`, `internal/devgen`, and `internal/scaffold`. `cmd` keeps Kong structs, `Run` methods, and the embeddable runtime/config files (they are copied verbatim into generated projects). Extract the shared CLI runtime from `root.go` into `cmd/runtime.go` and generate scaffold projects from an `embed.FS` of the real source files plus two small templates. Harden the SDK generator with a `go/format.Source` validation gate.

**Tech Stack:** Go 1.26, Kong (github.com/alecthomas/kong v1.16.0), gopkg.in/yaml.v3, stdlib `go/ast`/`go/printer`/`go/format`, `embed.FS`.

## Global Constraints

- Generated scaffold projects are separate Go modules and MUST NOT import `min`'s `internal` packages. `cmd/runtime.go` and `cmd/config.go` must therefore remain in the `cmd` package, self-contained with only stdlib + `github.com/alecthomas/kong` imports, and are byte-identical to the files min itself compiles.
- No import cycle: `cmd` may import `internal/*`; `internal/*` MUST NOT import `cmd`.
- All generated `.go` files from the OpenAPI SDK generator must pass through `go/format.Source` before being written.
- Preserve all existing behavior. Only test deltas allowed: `cmd/dev_test.go:141` and `cmd/dev_test.go:157` lose a trailing space; `TestFindStructInFile_PrefixBoundary` moves from `cmd/cmd_test.go` to `internal/scaffold`.
- All commands must pass `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, and `golangci-lint run ./...` after every task.

---

### Task 1: Create `internal/naming`

Move the identifier helpers currently duplicated across `cmd/dev.go` and `cmd/cmd.go` into a new package.

**Files:**
- Create: `internal/naming/naming.go`
- Create: `internal/naming/naming_test.go`
- Modify: (none — cmd keeps its local copies until Tasks 3-4 remove them)

**Interfaces:**
- Produces:
  - `func ToCamelCase(s string) string` — same behavior as `cmd/dev.go:1115` (`toCamelCase`; returns `"DoRequest"` when result is empty)
  - `func ToLowerCamelCase(s string) string` — same as `cmd/dev.go:1139` (`toLowerCamelCase`; returns `"param"` when empty)
  - `func SanitizeIdentStart(name, fallback string) string` — same as `cmd/dev.go:1149`
  - `func SanitizeFieldName(seg string) string` — same as `cmd/cmd.go:856`; i.e. `SanitizeIdentStart(ToCamelCase(seg), "X")`
  - `func TitleCase(s string) string` — same as `cmd/dev.go:1106`
  - `func LowerParamName(s string) string` — same as `cmd/dev.go:1161`; i.e. `SanitizeIdentStart(ToLowerCamelCase(s), "param")`
  - `func SanitizePkgName(s string) string` — same as `cmd/dev.go:1172`

- [ ] **Step 1: Write the failing test**

`internal/naming/naming_test.go`:

```go
package naming

import "testing"

func TestToCamelCase(t *testing.T) {
	cases := map[string]string{
		"foo-bar":       "FooBar",
		"foo_bar":       "FooBar",
		"123admin":      "X123admin",
		"/v1/users":     "V1Users",
		"":              "DoRequest",
		"alreadyUpper":  "AlreadyUpper",
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
	if got := SanitizePkgName("my-custom_sdk!"); got != "my_custom_sdk" {
		t.Errorf("SanitizePkgName = %q, want my_custom_sdk", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/naming/`
Expected: FAIL — `undefined: naming.ToCamelCase` (package compiles but functions missing). Note the package does not exist yet, so the run also needs `internal/naming/` to exist; create the empty directory first.

- [ ] **Step 3: Write the implementation**

`internal/naming/naming.go` — copy each function body verbatim from the source locations above, renaming to exported names:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/naming/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/naming
git commit -m "refactor(naming): extract identifier helpers into internal/naming"
```

---

### Task 2: Create `internal/astscan`, parallelize, delegate from `cmd`

Move all of `cmd/ast.go` logic into `internal/astscan`, thread an `io.Writer` through the printing functions so files can be processed in parallel while emitting deterministic output, and make `cmd/ast.go` a thin delegation layer.

**Files:**
- Create: `internal/astscan/astscan.go`
- Create: `internal/astscan/astscan_test.go`
- Rewrite: `cmd/ast.go` (keep `AstCmdGroup`, `AstScanCmd`, `AstTypesCmd`, `AstFnCmd` structs; Run methods delegate)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `func Scan(path string) error`
  - `func Types(path string) error`
  - `func Fn(target, path, typeFilter string) error`

- [ ] **Step 1: Write the failing test**

`internal/astscan/astscan_test.go`:

```go
package astscan

import (
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
	old := stdout
	defer func() { stdout = old }()
	var buf bytes.Buffer
	stdout = &buf
	if err := fn(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	return buf.String()
}
```

Production functions print to a package-level `var stdout io.Writer = os.Stdout` (declared in `astscan.go`); the test swaps that writer with a buffer instead of touching the real process stdout. Imports for the test file: `bytes`, `os`, `path/filepath`, `strings`, `testing`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/astscan/`
Expected: FAIL — package has no non-test Go files (`no non-test Go files`).

- [ ] **Step 3: Create `internal/astscan/astscan.go`**

Copy the logic from `cmd/ast.go`, applying these changes:

1. Package is `astscan`. Every function is unexported except the three entry points.
2. Thread a writer through printing:
   - Add `var stdout io.Writer = os.Stdout` (all output goes through `stdout`).
   - `func Scan(path string) error { return walkPath(path, printScanned) }`
   - `func Types(path string) error { return walkPath(path, printTypes) }`
   - `func Fn(target, path, typeFilter string) error` — the body of `(*AstFnCmd).Run` from `cmd/ast.go:66-199`, with these changes:
     - Read each file once and parse from those bytes (fixes double-read):
       ```go
       content, err := os.ReadFile(p)
       if err != nil {
           continue
       }
       f, err := parser.ParseFile(fset, p, content, parser.ParseComments)
       if err != nil {
           continue
       }
       pf := &parsedFile{path: p, fset: fset, file: f, content: content}
       ```
     - Compute helpers once and reuse (fixes double `collectHelpers`):
       ```go
       helpers := collectHelpers(target.decl, parsed)
       ```
       then use `helpers` in the "Referenced Types" loop and the "Internal Helpers" section instead of calling `collectHelpers` twice.
     - `targetName, targetType := target, typeFilter` at the top; keep the `strings.LastIndex(target, ".")` dot-notation split.
   - Change printing helpers to write to `stdout` instead of `fmt.Println`:
     - `func nodeSource(content []byte, fset *token.FileSet, n ast.Node)` → `fmt.Fprintln(stdout, string(content[start:end]))`
     - `func nodeSurround(...)` → `fmt.Fprintln(stdout, strings.Join(...))`
     - `func printNode(w io.Writer, fset *token.FileSet, node any)` — `printer.Fprint(w, fset, node)`; error-print to `w`:
       ```go
       func printNode(w io.Writer, fset *token.FileSet, node any) {
           var buf bytes.Buffer
           err := printer.Fprint(&buf, fset, node)
           if err == nil {
               fmt.Fprintln(w, buf.String())
           }
       }
       ```
     - `func processFile(w io.Writer, path string, fn func(io.Writer, *ast.File, *token.FileSet) error) error`:
       ```go
       func processFile(w io.Writer, path string, fn func(io.Writer, *ast.File, *token.FileSet) error) error {
           fset := token.NewFileSet()
           f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
           if err != nil {
               return fmt.Errorf("parse %s: %w", path, err)
           }
           fmt.Fprintf(w, "// %s\n", path)
           fmt.Fprintf(w, "package %s\n\n", f.Name)
           return fn(w, f, fset)
       }
       ```
     - `printScanned` and `printTypes` take `(w io.Writer, f *ast.File, fset *token.FileSet)` and call `printNode(w, ...)`.
3. Parallelize `walkPath`:
   - Replace the sequential `filepath.Walk` callback processing with: collect `.go` file paths in the walk, then `processFilesParallel(files, fn)`.
   - `processFilesParallel` buffers each file's output and emits in original order:

```go
func processFilesParallel(files []string, fn func(io.Writer, *ast.File, *token.FileSet) error) error {
	if len(files) == 0 {
		return nil
	}
	type result struct {
		out string
		err error
	}
	results := make([]result, len(files))

	workers := 8
	if len(files) < workers {
		workers = len(files)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				var buf bytes.Buffer
				err := processFile(&buf, files[idx], fn)
				results[idx] = result{out: buf.String(), err: err}
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			return r.err
		}
		fmt.Fprint(stdout, r.out)
	}
	return nil
}
```

   - Keep `walkPath`'s single-file path unchanged (calls `processFile(stdout, path, fn)`).
4. `readFile` is no longer needed in `Fn` (content read directly); delete it.
5. Keep all other helpers verbatim: `receiverTypeName`, `collectHelpers`, `extractCalledName`, `collectTypeRefs`, `parsedFile`, `targetMatch`, `helperInfo`.

- [ ] **Step 4: Rewrite `cmd/ast.go` as a delegation layer**

Keep the struct definitions and tags exactly as they are (`AstCmdGroup`, `AstScanCmd{Path}`, `AstTypesCmd{Path}`, `AstFnCmd{Func,Path,Type}`). Replace the `Run` method bodies and delete all helper functions, leaving:

```go
package cmd

import (
	"github.com/dat267/min/internal/astscan"
)

type AstCmdGroup struct {
	Scan  AstScanCmd  `cmd:"" help:"Scan Go files, strip bodies, keep signatures"`
	Types AstTypesCmd `cmd:"" help:"Show types only (structs, interfaces, func signatures)"`
	Fn    AstFnCmd    `cmd:"" help:"Extract context for a single function"`
}

type AstScanCmd struct {
	Path string `help:"File or directory to scan" arg:"" default:"."`
}

func (c *AstScanCmd) Run() error {
	return astscan.Scan(c.Path)
}

type AstTypesCmd struct {
	Path string `help:"File or directory to scan" arg:"" default:"."`
}

func (c *AstTypesCmd) Run() error {
	return astscan.Types(c.Path)
}

type AstFnCmd struct {
	Func string `arg:"" help:"Name of the function to search for (e.g. 'MyFunc' or 'MyStruct.MyMethod')."`
	Path string `arg:"" help:"Path to file or directory." default:"."`
	Type string `name:"type" help:"Optional receiver type name filter."`
}

func (c *AstFnCmd) Run() error {
	return astscan.Fn(c.Func, c.Path, c.Type)
}
```

- [ ] **Step 5: Run all tests**

Run: `go build ./... && go test ./internal/astscan/ && go test ./cmd/ -run 'TestAst' -v`
Expected: all PASS. `cmd/ast_test.go` exercises the new code through `Run` methods and must still pass.

- [ ] **Step 6: Lint, then commit**

Run: `golangci-lint run ./...`
Expected: no new issues (a `sync` import must be present in astscan.go).

```bash
git add internal/astscan cmd/ast.go
git commit -m "refactor(astscan): extract AST scanning into internal/astscan with parallel IO"
```

---

### Task 3: Create `internal/devgen` with go/format gate, delegate from `cmd`

Move HAR→OpenAPI and OpenAPI→Go SDK generation out of `cmd/dev.go` into `internal/devgen`, rewrite emission to minimize format-string escaping, add a `go/format.Source` validation gate, and update the two pinned assertions.

**Files:**
- Create: `internal/devgen/har.go`
- Create: `internal/devgen/sdk.go`
- Create: `internal/devgen/format.go`
- Create: `internal/devgen/format_test.go`
- Rewrite: `cmd/dev.go` (keep structs; Run methods delegate)
- Modify: `cmd/dev_test.go:141` and `cmd/dev_test.go:157` (trim trailing space)

**Interfaces:**
- Consumes: `internal/naming` (`ToCamelCase`, `ToLowerCamelCase`, `SanitizeIdentStart`, `TitleCase`, `LowerParamName`, `SanitizePkgName`).
- Produces:
  - `func HarToOpenAPI(input, output, title, host, filter string, apiOnly bool) error`
  - `func OpenAPIToGo(input, output, pkg, client string) error`
  - `func FormatSource(src []byte) ([]byte, error)`

- [ ] **Step 1: Write the failing test**

`internal/devgen/format_test.go`:

```go
package devgen

import (
	"testing"
)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devgen/`
Expected: FAIL — `undefined: FormatSource`.

- [ ] **Step 3: Create `internal/devgen/format.go`**

```go
package devgen

import "go/format"

// FormatSource gofmt-normalizes src and validates that it is syntactically
// valid Go. Generators call this before writing any .go file so that broken
// output fails at generation time instead of producing an uncompilable SDK.
func FormatSource(src []byte) ([]byte, error) {
	return format.Source(src)
}
```

Run: `go test ./internal/devgen/`
Expected: PASS.

- [ ] **Step 4: Create `internal/devgen/har.go`**

Copy from `cmd/dev.go`:
- HAR types: `harContainer`, `harLog`, `harEntry`, `harRequest`, `harResponse`, `harHeader`, `harQuery`, `harPostData`, `harContent` (`cmd/dev.go:31-77`) — verbatim.
- `func (c *Har2OpenapiCmd) Run` logic (`cmd/dev.go:79-158`) → `HarToOpenAPI`:

```go
func HarToOpenAPI(input, output, title, host, filter string, apiOnly bool) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read HAR file: %w", err)
	}
	var har harContainer
	if err := json.Unmarshal(data, &har); err != nil {
		return fmt.Errorf("parse HAR json: %w", err)
	}
	spec := generateOpenAPI(har.Log.Entries, title, host, filter, apiOnly)
	// ... (identical merge-with-existing-spec, YAML/JSON marshal, and
	// write-or-stdout logic from cmd/dev.go:92-157, replacing c.X with
	// the function parameters and `c.Output` with `output`)
	return nil
}
```

- `generateOpenAPI(entries []harEntry, title, host, filter string, apiOnly bool) map[string]any` — copy `cmd/dev.go:160-276`, replacing `c.Host`/`c.Filter`/`c.ApiOnly`/`c.Title` with parameters.
- `inferSchemaFromText`, `inferSchema`, `isStaticAsset` — copy verbatim from `cmd/dev.go:278-367`.

- [ ] **Step 5: Create `internal/devgen/sdk.go`**

Copy from `cmd/dev.go`:
- OpenAPI types: `openAPISpec`, `openAPIInfo`, `openAPIOp`, `openAPIParam`, `openAPIReqBody`, `structCollector`, `sdkMethod` (`cmd/dev.go:376-529`) — verbatim.
- `structCollector` methods `getUniqueName`, `getUniqueMethodName`, `buildStructFromSchema`, `mapSchemaToGoType` — verbatim from `cmd/dev.go:419-1095`, but import `internal/naming` and replace uses:
  - `toCamelCase(x)` → `naming.ToCamelCase(x)`
  - `sanitizeIdentStart(x, y)` → `naming.SanitizeIdentStart(x, y)`
- `func (c *Openapi2GoCmd) Run` logic (`cmd/dev.go:451-517`) → `OpenAPIToGo`:

```go
func OpenAPIToGo(input, output, pkg, client string) error {
	// identical logic to cmd/dev.go:451-517, replacing c.Input/c.Output/c.Pkg/c.Client
	// with parameters, and wrapping the two os.WriteFile calls with the
	// format gate:
	//   formatted, err := FormatSource([]byte(code))
	//   if err != nil {
	//       return fmt.Errorf("generated invalid Go for %s: %w", outPath, err)
	//   }
	//   os.WriteFile(outPath, formatted, 0644)
	return nil
}
```

- `collectSDKMethods`, `generateGoSDK`, `generateGoSDKTests` — copy from `cmd/dev.go:533-1030`, then apply the emission cleanup (Step 6).
- `isHTTPMethod`, `selectTypeName` — verbatim from `cmd/dev.go:1097,1165`.
- `detectPeerPackageName` — verbatim from `cmd/dev.go:1182-1230`, replacing `sanitizePkgName` with `naming.SanitizePkgName`.
- Delete `titleCase`, `toCamelCase`, `toLowerCamelCase`, `sanitizeIdentStart`, `lowerParamName`, `sanitizePkgName` — all now live in `internal/naming`. Inside sdk.go, call `naming.TitleCase`, `naming.LowerParamName`, `naming.ToCamelCase`, `naming.SanitizeIdentStart`.

- [ ] **Step 6: Reduce format-string escaping + add format gate in emission**

In `generateGoSDK` and `generateGoSDKTests` (`cmd/dev.go:656-1030`), transform:
1. Every `fmt.Fprintf(&sb, "literal-with-no-format-directives\n")` (no args) → `sb.WriteString("literal-with-no-format-directives\n")` with `%%` unescaped to `%`. This removes the escaping hazard for static lines.
2. Every `fmt.Fprintf(&sb, "fmt:%s", args)` (with args) stays as-is.
3. Add `"go/format"` import; `FormatSource` gate is applied at the call site in `OpenAPIToGo` (Step 5), so no changes inside `generateGoSDK` itself beyond the emission style.
4. `generateGoSDKTests` also emits struct-tag-like lines? No — it emits test functions only; apply the same WriteString transformation.

Note: struct tag lines inside `buildStructFromSchema`/query-params builder keep their exact text (including the trailing space before the closing backtick, which `format.Source` will strip):
```go
fmt.Fprintf(&sb, "\t%s %s `json:\"%s,omitempty\"` \n", fieldName, goType, k)
```
This line has args so it stays a `Fprintf`.

- [ ] **Step 7: Rewrite `cmd/dev.go` as a delegation layer**

Keep `DevCmdGroup`, `Har2OpenapiCmd`, `Openapi2GoCmd` struct definitions verbatim. Replace Run bodies and delete all other code, leaving:

```go
package cmd

import (
	"context"

	"github.com/dat267/min/internal/devgen"
)

type DevCmdGroup struct {
	Har2Openapi Har2OpenapiCmd `cmd:"" help:"Convert HAR capture file to OpenAPI 3.0 specification"`
	Openapi2Go  Openapi2GoCmd  `cmd:"" help:"Generate Go SDK client from OpenAPI specification"`
}

type Har2OpenapiCmd struct {
	Input   string `help:"Path to HAR file" arg:""`
	Output  string `help:"Output OpenAPI file path (json or yaml)" short:"o" default:""`
	Title   string `help:"API title" default:"API Specification"`
	Host    string `help:"Filter requests by host (e.g. api.example.com)" default:""`
	Filter  string `help:"Path prefix filter (e.g. /v1/ or /api/)" default:""`
	ApiOnly bool   `help:"Ignore static assets (js, css, images, fonts, html)" default:"true"`
}

func (c *Har2OpenapiCmd) Run(ctx context.Context) error {
	return devgen.HarToOpenAPI(c.Input, c.Output, c.Title, c.Host, c.Filter, c.ApiOnly)
}

type Openapi2GoCmd struct {
	Input  string `help:"Path to OpenAPI spec file (JSON or YAML)" arg:""`
	Output string `help:"Output Go SDK package directory or file path" short:"o" default:"client.go"`
	Pkg    string `help:"Go package name (defaults to parent directory name of output file)" default:""`
	Client string `help:"Go SDK Client struct name" default:"Client"`
}

func (c *Openapi2GoCmd) Run(ctx context.Context) error {
	return devgen.OpenAPIToGo(c.Input, c.Output, c.Pkg, c.Client)
}
```

- [ ] **Step 8: Update the two pinned assertions**

In `cmd/dev_test.go`:
- Line 141: `"Role string ` + "`" + `json:\"role,omitempty\"` + "`" + ` ` (trailing space) → remove the trailing space: `"Role string ` + "`" + `json:\"role,omitempty\"` + "`" + `"`.
- Line 157: same for `"Username string ` + "`" + `json:\"username,omitempty\"` + "`" + ` `.

- [ ] **Step 9: Run all tests**

Run: `go build ./... && go test ./internal/devgen/ && go test ./cmd/ -run 'TestOpenapi|TestHar|TestPublicAPIs' -v`
Expected: all PASS, including `TestPublicAPIs_EndToEnd` which `go build`s every generated SDK.

- [ ] **Step 10: Lint, then commit**

Run: `golangci-lint run ./...`
Expected: no new issues.

```bash
git add internal/devgen cmd/dev.go cmd/dev_test.go
git commit -m "refactor(devgen): extract HAR/OpenAPI generation into internal/devgen with go/format gate"
```

---

### Task 4: Create `internal/scaffold`, delegate from `cmd`

Move the `cmd add` / `cmd show` / `cmd edit` logic and all AST-editing helpers out of `cmd/cmd.go` into `internal/scaffold`, and delete the now-dead naming helpers from `cmd`.

**Files:**
- Create: `internal/scaffold/scaffold.go`
- Create: `internal/scaffold/scaffold_test.go`
- Rewrite: `cmd/cmd.go` (keep `CmdGroup`, `CmdInitCmd`, `CmdAddCmd`, `CmdShowCmd`, `CmdEditCmd` structs; Run methods delegate; `CmdInitCmd.Run` keeps its dir/module logic but file writing is replaced in Task 5)
- Modify: `cmd/cmd_test.go` (move `TestFindStructInFile_PrefixBoundary` out)

**Interfaces:**
- Consumes: `internal/naming` (`SanitizeFieldName`).
- Produces:
  - `func AddCommand(name, desc string) error`
  - `func ShowCommands() error`
  - `func EditCommand(name string) error`
  - `func DetectCLIFile() string`
  - `func FindStructInFile(filePath, seg string) string`
  - `func StructHasMethod(filePath, structName, methodName string) bool`
  - `func StructExistsInFile(filePath, structName string) bool`
  - `func AppendStructToFile(filePath, structName string, isLeaf bool) error`
  - `func RegisterField(file, structName, fieldName, typeName, helpText string) error`
  - `func EnsureImports(content string, imports ...string) string`

- [ ] **Step 1: Write the failing test**

`internal/scaffold/scaffold_test.go` — move `TestFindStructInFile_PrefixBoundary` from `cmd/cmd_test.go:177-205`, rewriting calls to use the exported name:

```go
package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindStructInFile_PrefixBoundary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")

	adminOnly := `package cmd

type Administrator struct{}
`
	if err := os.WriteFile(file, []byte(adminOnly), 0644); err != nil {
		t.Fatal(err)
	}
	if got := FindStructInFile(file, "admin"); got != "" {
		t.Errorf("expected no match for 'admin', got %q", got)
	}

	adminCmds := `package cmd

type AdminCommands struct{}
`
	if err := os.WriteFile(file, []byte(adminCmds), 0644); err != nil {
		t.Fatal(err)
	}
	if got := FindStructInFile(file, "admin"); got != "AdminCommands" {
		t.Errorf("expected AdminCommands, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scaffold/`
Expected: FAIL — `undefined: FindStructInFile`.

- [ ] **Step 3: Create `internal/scaffold/scaffold.go`**

Copy the following from `cmd/cmd.go` verbatim (renaming to exported, `naming.SanitizeFieldName` for identifier handling):
- `detectCLIFile` (`cmd/cmd.go:580-614`) → `func DetectCLIFile() string`
- `findStructInFile` (`cmd/cmd.go:616-678`) → `func FindStructInFile(filePath, seg string) string`, replacing `sanitizeFieldName` with `naming.SanitizeFieldName`
- `structHasMethod` (`cmd/cmd.go:680-700`) → `func StructHasMethod(filePath, structName, methodName string) bool`
- `structExistsInFile` (`cmd/cmd.go:702-724`) → `func StructExistsInFile(filePath, structName string) bool`
- `appendStructToFile` (`cmd/cmd.go:726-766`) → `func AppendStructToFile(filePath, structName string, isLeaf bool) error`
- `registerField` (`cmd/cmd.go:768-803`) → `func RegisterField(file, structName, fieldName, typeName, helpText string) error`
- `ensureImports` (`cmd/cmd.go:805-852`) → `func EnsureImports(content string, imports ...string) string`

Add the three command-logic functions:

```go
// AddCommand implements `cmd add`. Logic is a verbatim copy of
// (*CmdAddCmd).Run from cmd/cmd.go:436-524 with c.Name->name, c.Desc->desc,
// and the helper calls renamed to the exported forms above.
func AddCommand(name, desc string) error { /* ... */ }

// ShowCommands implements `cmd show`. Copy of (*CmdShowCmd).Run
// (cmd/cmd.go:528-546) with the skip list extended to also exclude
// runtime.go and config.go.
func ShowCommands() error {
	entries, err := os.ReadDir("cmd")
	if err != nil {
		return fmt.Errorf("read cmd/: %w", err)
	}
	fmt.Println("Commands:")
	count := 0
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".go") && name != "root.go" && name != "cmd.go" &&
			name != "runtime.go" && name != "config.go" && !strings.HasSuffix(name, "_test.go") {
			fmt.Printf("  %s\n", strings.TrimSuffix(name, ".go"))
			count++
		}
	}
	if count == 0 {
		fmt.Println("  (no commands yet)")
	}
	return nil
}

// EditCommand implements `cmd edit`. Copy of (*CmdEditCmd).Run
// (cmd/cmd.go:548-578) with c.Name->name.
func EditCommand(name string) error { /* ... */ }
```

Imports for the file: `fmt`, `go/ast`, `go/parser`, `go/token`, `os`, `os/exec`, `path/filepath`, `sort`, `strings`, `unicode`, `unicode/utf8`, `github.com/dat267/min/internal/naming`.

- [ ] **Step 4: Rewrite `cmd/cmd.go`**

Keep `CmdGroup` struct and the four command struct definitions verbatim. Run bodies delegate:

```go
func (c *CmdAddCmd) Run() error {
	return scaffold.AddCommand(c.Name, c.Desc)
}

func (c *CmdShowCmd) Run() error {
	return scaffold.ShowCommands()
}

func (c *CmdEditCmd) Run() error {
	return scaffold.EditCommand(c.Name)
}
```

`CmdInitCmd.Run` keeps its dir/module + `go mod init`/`go mod tidy` logic but file writing is replaced in Task 5; for now it may keep calling a placeholder that writes the files (Task 5 replaces it). If `cmdTmpl`/`mainTmpl` are still referenced by `CmdInitCmd.Run`, keep them in this task (removed in Task 5).

Delete from `cmd/cmd.go`: `detectCLIFile`, `findStructInFile`, `structHasMethod`, `structExistsInFile`, `appendStructToFile`, `registerField`, `ensureImports`, `sanitizeFieldName`. Delete `cmd/dev.go`'s naming helpers ONLY if they are no longer referenced anywhere in `cmd` (Task 3 already removed their uses).

- [ ] **Step 5: Remove the moved test from `cmd/cmd_test.go`**

Delete `TestFindStructInFile_PrefixBoundary` (`cmd/cmd_test.go:177-205`). Verify no remaining use of `findStructInFile` in `cmd`.

- [ ] **Step 6: Run all tests**

Run: `go build ./... && go test ./internal/scaffold/ && go test ./cmd/ -run 'TestCmd|TestFind|TestInit' -v`
Expected: all PASS.

- [ ] **Step 7: Lint, then commit**

Run: `golangci-lint run ./...`
Expected: no new issues (e.g. unused imports removed from cmd/cmd.go).

```bash
git add internal/scaffold cmd/cmd.go cmd/cmd_test.go
git commit -m "refactor(scaffold): extract cmd add/show/edit logic into internal/scaffold"
```

---

### Task 5: Kill template drift — embed real runtime, generate scaffold from it

Extract the runtime from `cmd/root.go` into `cmd/runtime.go`, add `SetAppName`, make `CfgPath` lazy, embed `runtime.go`/`config.go` plus two templates, and rewrite `CmdInitCmd.Run` to write the new project from the embed.

**Files:**
- Create: `cmd/runtime.go`
- Modify: `cmd/root.go` (remove runtime; keep `CLI`, `Version`, `VersionCmd`, build-info `init`)
- Create: `cmd/templates.go`
- Create: `cmd/scaffold_templates/main.go.tmpl`
- Create: `cmd/scaffold_templates/cmd.go.tmpl`
- Modify: `cmd/cmd.go` (`CmdInitCmd.Run` + delete `cmdTmpl`/`mainTmpl`)

**Interfaces:**
- Consumes: none new (all runtime stays in `cmd`).
- Produces:
  - `var appName = "min"`
  - `func SetAppName(name string)`
  - `func SetConfigPath(p string)`, `func CfgPath() string`, `func resolveConfigPath() string`
  - `func resolveConfigFileFlag() string`
  - `const configFileFlagName = "config-file"`
  - `func JSONResolver(r io.Reader) (kong.Resolver, error)`
  - `func Execute(ctx context.Context)`
  - `var scaffoldTemplates embed.FS`
  - `func writeScaffoldProject(dir, name, module string) error`

- [ ] **Step 1: Write the failing test**

The existing `cmd/root_test.go` and `cmd/config_test.go` already pin this behavior (`resolveConfigPath` via env/local file, `resolveConfigFileFlag`, `JSONResolver`, `SetConfigPath`/`CfgPath`). Add one test for `SetAppName`:

`cmd/root_test.go` addition:

```go
func TestSetAppName(t *testing.T) {
	t.Setenv("MYCUSTOMAPP_CONFIG_FILE", "")
	_ = os.Remove("mycustomapp.json")
	defer func() { _ = os.Remove("mycustomapp.json") }()

	SetAppName("mycustomapp")
	defer SetAppName("min")

	got := resolveConfigPath()
	// resolveConfigPath may resolve to a local file or a per-user config dir,
	// but the app name must appear in the path either way.
	if !strings.Contains(got, "mycustomapp.json") {
		t.Errorf("expected path containing mycustomapp.json, got %q", got)
	}
}
```

Run: `go test ./cmd/ -run TestSetAppName -v`
Expected: FAIL — `undefined: SetAppName`.

- [ ] **Step 2: Create `cmd/runtime.go`**

```go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/kong"
)

// appName is the application name used for config path resolution. It is "min"
// for this binary and is overridden in generated projects via SetAppName.
var appName = "min"

const configFileFlagName = "config-file"

var (
	cfgPathMu sync.RWMutex
	cfgPath   string
)

// SetAppName overrides the application name used for config path resolution.
func SetAppName(name string) {
	if name != "" {
		appName = name
	}
}

// SetConfigPath overrides the config file path used by config commands.
func SetConfigPath(p string) {
	cfgPathMu.Lock()
	defer cfgPathMu.Unlock()
	cfgPath = p
}

// CfgPath returns the configured config file path, resolving a default lazily
// when none has been set.
func CfgPath() string {
	cfgPathMu.RLock()
	p := cfgPath
	cfgPathMu.RUnlock()
	if p != "" {
		return p
	}
	return resolveConfigPath()
}

func resolveConfigPath() string {
	envKey := strings.ToUpper(appName) + "_CONFIG_FILE"
	if cf := os.Getenv(envKey); cf != "" {
		return cf
	}
	localFile := appName + ".json"
	if _, err := os.Stat(localFile); err == nil {
		return localFile
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, appName, appName+".json")
	}
	return localFile
}

// Execute is the main entry point called by main.go.
func Execute(ctx context.Context) {
	app := &CLI{}

	if cf := resolveConfigFileFlag(); cf != "" {
		SetConfigPath(cf)
	}
	activeConfig := CfgPath()

	options := []kong.Option{
		kong.Name(appName),
		kong.Description("CLI project scaffolding tool"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.BindTo(ctx, (*context.Context)(nil)),
	}

	if f, err := os.Open(activeConfig); err == nil {
		if resolver, err := JSONResolver(f); err == nil {
			options = append(options, kong.Resolvers(resolver))
		}
		_ = f.Close()
	}

	k, err := kong.New(app, options...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	kongCtx, err := k.Parse(os.Args[1:])
	k.FatalIfErrorf(err)

	SetConfigPath(app.ConfigFile)
	k.FatalIfErrorf(kongCtx.Run())
}

func resolveConfigFileFlag() string {
	for i, arg := range os.Args {
		if arg == "--"+configFileFlagName && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "--"+configFileFlagName+"=") {
			return strings.SplitN(arg, "=", 2)[1]
		}
	}
	return ""
}

// JSONResolver builds a Kong resolver capable of loading both flat and nested JSON configuration.
func JSONResolver(r io.Reader) (kong.Resolver, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	flat := make(map[string]any)

	var flattenNested func(prefix string, m map[string]any)
	flattenNested = func(prefix string, m map[string]any) {
		for k, v := range m {
			key := k
			if prefix != "" {
				key = prefix + "-" + k
			}
			if sub, ok := v.(map[string]any); ok {
				flattenNested(key, sub)
			} else if prefix != "" {
				flat[key] = v
			}
		}
	}
	flattenNested("", raw)

	for k, v := range raw {
		if _, isMap := v.(map[string]any); !isMap {
			flat[k] = v
		}
	}

	return kong.ResolverFunc(func(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		if val, ok := flat[flag.Name]; ok {
			return val, nil
		}
		return nil, nil
	}), nil
}
```

Note the behavior change: `SetConfigPath("")` no longer resolves immediately; `CfgPath()` resolves lazily. The existing `config_test.go`/`root_test.go` still pass (they call `SetConfigPath(p)` with a real path, and `SetConfigPath("")` in cleanup). `resolveConfigFileFlag` now uses `configFileFlagName`.

- [ ] **Step 3: Rewrite `cmd/root.go`**

```go
package cmd

import (
	"fmt"
	"runtime/debug"
)

// CLI is the root CLI struct containing all subcommand groups.
type CLI struct {
	Dev        DevCmdGroup `cmd:"" help:"Developer utilities (cURL, HAR, OpenAPI generator)"`
	ConfigFile string      `help:"Config file path" json:"-"`
	Version    VersionCmd  `cmd:"" help:"Show version"`
	Ast        AstCmdGroup `cmd:"" help:"Parse Go AST for AI context"`
	Cmd        CmdGroup    `cmd:"" help:"Manage commands"`
	Config     ConfigCmdGroup `cmd:"" help:"Manage application configuration"`
}

var Version = "dev"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
}

type VersionCmd struct{}

func (c *VersionCmd) Run() error {
	fmt.Println(Version)
	return nil
}
```

(`appName`, config path, `Execute`, `JSONResolver` all move to runtime.go.)

- [ ] **Step 4: Create the scaffold templates**

`cmd/scaffold_templates/main.go.tmpl`:

```text
package main

import (
	"context"

	"{{.Module}}/cmd"
)

func main() {
	cmd.Execute(context.Background())
}
```

`cmd/scaffold_templates/cmd.go.tmpl`:

```text
package cmd

import (
	"fmt"
	"strings"
)

func init() {
	SetAppName("{{.AppName}}")
}

type CLI struct {
	ConfigFile string         `help:"Config file path" json:"-"`
	Greet      GreetCmd       `cmd:"" help:"Print a greeting"`
	Config     ConfigCmdGroup `cmd:"" help:"Manage configuration"`
}

type GreetCmd struct {
	Name  string `help:"Name to greet" arg:"" default:"World"`
	Count int    `help:"Repeat count" default:"1"`
	Shout bool   `help:"Shout" short:"s"`
}

func (c *GreetCmd) Run() error {
	for i := 0; i < c.Count; i++ {
		msg := fmt.Sprintf("Hello, %s!", c.Name)
		if c.Shout {
			msg = strings.ToUpper(msg)
		}
		fmt.Println(msg)
	}
	return nil
}
```

- [ ] **Step 5: Create `cmd/templates.go`**

```go
package cmd

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed runtime.go config.go scaffold_templates/main.go.tmpl scaffold_templates/cmd.go.tmpl
var scaffoldTemplates embed.FS

type scaffoldData struct {
	AppName string
	Module  string
}

// writeScaffoldProject writes the files of a generated CLI project. The
// runtime and config files are byte-identical copies of the files this binary
// itself compiles, so the generated project cannot drift from min's runtime.
func writeScaffoldProject(dir, name, module string) error {
	data := scaffoldData{AppName: name, Module: module}

	render := func(tmplPath, dest string) error {
		raw, err := scaffoldTemplates.ReadFile(tmplPath)
		if err != nil {
			return err
		}
		t, err := template.New(filepath.Base(tmplPath)).Parse(string(raw))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			return err
		}
		return os.WriteFile(dest, buf.Bytes(), 0644)
	}

	cmdDir := filepath.Join(dir, "cmd")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		return err
	}

	if err := render("scaffold_templates/main.go.tmpl", filepath.Join(dir, "main.go")); err != nil {
		return err
	}
	if err := render("scaffold_templates/cmd.go.tmpl", filepath.Join(cmdDir, "cmd.go")); err != nil {
		return err
	}

	for _, f := range []string{"runtime.go", "config.go"} {
		raw, err := scaffoldTemplates.ReadFile(f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(cmdDir, f), raw, 0644); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 6: Rewrite `CmdInitCmd.Run` and delete `cmdTmpl`/`mainTmpl`**

In `cmd/cmd.go`, replace `CmdInitCmd.Run`:

```go
func (c *CmdInitCmd) Run() error {
	name := c.Name
	if name == "" {
		return fmt.Errorf("project name is required")
	}

	var dir string
	if name == "." {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
		name = filepath.Base(wd)
		dir = "."
	} else {
		dir = name
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("directory %q already exists", dir)
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}

	module := c.Module
	if module == "" {
		module = name
	}

	if err := writeScaffoldProject(dir, name, module); err != nil {
		return fmt.Errorf("write project: %w", err)
	}

	modCmd := exec.Command("go", "mod", "init", module)
	modCmd.Dir = dir
	if out, err := modCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod init: %w\n%s", err, out)
	}

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = dir
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("resolve dependencies: %w\n%s", err, out)
	}

	fmt.Printf("Created project %q in %s/\n", name, dir)
	fmt.Printf("  cd %s && go run .\n", dir)
	return nil
}
```

Delete `mainTmpl` (`cmd/cmd.go:84-97`) and `cmdTmpl` (`cmd/cmd.go:99-429`).

- [ ] **Step 7: Verify generated project compiles**

Add an end-to-end assertion to `cmd/cmd_test.go` `TestInitCmd` (extend the existing test):

```go
	// Generated project must compile and run the greeting command.
	mod := filepath.Join("myapp", "go.mod")
	if _, err := os.Stat(mod); os.IsNotExist(err) {
		t.Fatal("go.mod not created")
	}
```

Add a new test that runs the generated binary:

```go
func TestInitCmd_GeneratedProjectRuns(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := (&CmdInitCmd{Name: "myapp"}).Run(); err != nil {
		t.Fatalf("init project failed: %v", err)
	}

	out := captureStdout(t, func() {
		cmd := exec.Command("go", "run", ".", "greet")
		cmd.Dir = filepath.Join(dir, "myapp")
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("generated project failed to run: %v", err)
		}
	})
	if !strings.Contains(out, "Hello, World!") {
		t.Errorf("expected greeting, got: %s", out)
	}
}
```

Add imports `os/exec` and `strings` to `cmd/cmd_test.go` as needed.

- [ ] **Step 8: Run all tests**

Run: `go build ./... && go vet ./... && go test -race -count=1 ./cmd/ -run 'TestInit|TestSetAppName|TestResolve|TestConfig' -v`
Expected: all PASS. Then full: `go test -race -count=1 ./...`

- [ ] **Step 9: Lint, then commit**

Run: `golangci-lint run ./...`
Expected: no new issues (watch for `staticcheck` complaints about `cfgPathMu` — the mutex is still used by `SetConfigPath`/`CfgPath`).

```bash
git add cmd/runtime.go cmd/root.go cmd/templates.go cmd/scaffold_templates cmd/cmd.go cmd/root_test.go cmd/cmd_test.go
git commit -m "feat(scaffold): generate projects from embedded runtime source, killing template drift"
```

---

### Task 6: Full verification pass

**Files:**
- Modify: `README.md` if the usage/help block or examples changed (verify against freshly built binary)

- [ ] **Step 1: Verify release checklist per AGENTS.md**

Run:
```bash
go build ./...
go vet ./...
go test -race -count=1 ./...
golangci-lint run ./...
```
Expected: all clean.

- [ ] **Step 2: Verify README matches the binary**

Build `./min` and run `./min --help`; diff the output against the README usage block (`README.md:5-31`). Also verify `./min config show`, `./min version`, and the generated-project greeting from the Quick start (`README.md:35-43`) still match.

- [ ] **Step 3: Commit**

```bash
git status   # confirm tree state is intentional
git add README.md   # only if README changed
git commit -m "docs: sync README with refactored CLI output"
```

(If no README change is needed, make no commit.)
