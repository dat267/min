# Scalability Refactor Design

Date: 2026-08-02

## Context

`min` is a Go CLI toolkit (Kong-based) for project scaffolding, AST analysis,
config file handling, and HAR/OpenAPI dev utilities. A scalability assessment
found four critical and three moderate structural risks:

1. **Template drift**: `cmdTmpl()` in `cmd/cmd.go` re-embeds ~200 lines of the
   CLI runtime as a `fmt.Sprintf` string, duplicating the live implementation
   in `root.go` and `config.go`.
2. **String-based code generation**: `generateGoSDK`/`generateGoSDKTests` in
   `cmd/dev.go` build Go code via `%%`-escaped format strings. Git history shows
   recurring "always-compiling" fixes.
3. **String-surgery on user files**: `cmd add` edits Go files with
   `strings.Index` brace searching (`registerField`, `appendStructToFile`).
4. **Monolithic `cmd` package**: 4,578 lines across 5 files mixing command
   definitions, generators, and AST helpers.
5. Sequential, double-read file I/O in AST commands.
6. Magic string flag pre-scan in `resolveConfigFileFlag`.
7. Global mutable `cfgPath`.

## Goal

Make `min` structurally scalable: single source of truth for the scaffold
runtime, gofmt-validated code generation, thin `cmd` package delegating to
internal packages, parallel AST scanning, and no duplicated runtime code.

## Non-goals (deferred)

- ~~Config dependency injection~~ — now implemented; see
  `docs/specs/2026-08-02-config-di-design.md`.
- Full `go/printer` AST rewrite of the SDK generator. Replaced by the
  `go/format.Source` validation gate (Section 4) which delivers the same
  correctness guarantees at a fraction of the churn.

## Design

### 1. Single source of truth for the scaffold runtime

**New file `cmd/runtime.go`** (package `cmd`), extracted from `root.go:32-173`:

- `var appName = "min"` (was `const appName`)
- `func SetAppName(name string)` — sets `appName` (used by generated projects)
- `cfgPath` var + mutex, `SetConfigPath(p)`, `CfgPath()` — `CfgPath()` resolves
  lazily (returns `cfgPath` if set, else `resolveConfigPath()`), removing the
  eager `init()` that broke app-name customization for generated projects
- `resolveConfigPath()`, `resolveConfigFileFlag()`, `JSONResolver(r)`
- `Execute(ctx context.Context)` — unchanged behavior; references `CLI` and
  `ConfigFile` which both `root.go` (min) and the scaffold stub define

**`cmd/root.go`** keeps: `CLI` struct, `Version` var, `VersionCmd`, and the
`init()` that sets `Version` from `debug.ReadBuildInfo()`.

**New file `cmd/templates.go`**:

- `//go:embed runtime.go config.go scaffold_templates/main.go.tmpl scaffold_templates/cmd.go.tmpl`
  exposes an `embed.FS`.
- `scaffold_templates/main.go.tmpl` — `package main` + `module/cmd` import,
  templated with module path.
- `scaffold_templates/cmd.go.tmpl` — `package cmd`, `func init() { SetAppName("<name>") }`,
  `CLI` struct (ConfigFile, Greet, Config group), `GreetCmd` + `Run`. Templated
  with app name.
- Scaffold writer helper(s) used by `CmdInitCmd.Run`.

**`CmdInitCmd.Run`** (`cmd/cmd.go`) writes into the new project:
- `main.go` ← templated `main.go.tmpl`
- `cmd/cmd.go` ← templated `cmd.go.tmpl`
- `cmd/runtime.go` ← verbatim copy of embedded `runtime.go`
- `cmd/config.go` ← verbatim copy of embedded `config.go`
- then `go mod init` + `go mod tidy` (unchanged)

Delete `cmdTmpl()` and `mainTmpl()` from `cmd/cmd.go` entirely. The generated
project is now byte-identical to min's own runtime for the shared parts.

**`CmdShowCmd.Run`**: extend the skip list to exclude `runtime.go` and
`config.go` so they are not listed as commands.

### 2. Scaffold files must stay in `cmd`

The generated project is a separate Go module and cannot import `min`'s
`internal` packages. Therefore the embedded runtime (`runtime.go`) and config
commands (`config.go`) remain in the `cmd` package, self-contained with only
stdlib + kong imports. This also avoids an import cycle: `cmd` imports
`internal/scaffold`, and `internal/scaffold` never imports `cmd`.

### 3. Subpackage split

Target structure:

```
cmd/            Kong structs + Run methods + embeddable runtime/config + templates
  root.go       CLI struct, VersionCmd, Version
  runtime.go    Execute + config runtime (embedded)
  config.go     Config commands (embedded)
  cmd.go        CmdGroup + Run methods (init/add/show/edit)
  dev.go        DevCmdGroup + Run methods
  ast.go        AstCmdGroup + Run methods
  templates.go  go:embed FS + scaffold writer
internal/
  naming/       identifier helpers (toCamelCase, toLowerCamelCase,
                sanitizeIdentStart, sanitizeFieldName, titleCase,
                lowerParamName, sanitizePkgName)
  scaffold/     AST-based project editing (detectCLIFile, findStructInFile,
                structHasMethod, structExistsInFile, appendStructToFile,
                registerField, ensureImports) + cmd add/show/edit logic
  astscan/      walk/scan/types/fn logic (parallelized, single read)
  devgen/       HAR->OpenAPI + OpenAPI->Go SDK generation
```

All command structs and `Run` methods stay in `cmd` (Kong requires them there);
they delegate to exported functions in the internal packages. The tests call
`Run` methods via structs, so they survive the move unchanged, with one
exception: `TestFindStructInFile_PrefixBoundary` (`cmd/cmd_test.go:177`) calls
`findStructInFile` directly and moves to `internal/scaffold`.

`internal/config` is intentionally NOT created: the config helpers
(`loadConfigMap`, `saveConfigMap`, `setNestedMap`, `unsetNestedMap`) must stay
in `cmd/config.go` because that file is embedded into generated projects.

### 4. Harden SDK code generation with go/format validation

Rewrite `generateGoSDK`, `generateGoSDKTests`, and the struct/params builders in
`internal/devgen`:

- Static output lines via `strings.Builder` `WriteString` (no format-string
  escaping); `fmt.Fprintf` only at interpolation points. Backticks in struct
  tags are legal inside double-quoted Go string literals, so no raw-string
  limitation.
- Before writing any `.go` file, run `go/format.Source` on the full output. On
  error, return a generation error (fail loudly at generation time instead of
  shipping a broken SDK).
- The formatted output is always gofmt-clean and syntactically valid.

Behavior (method names, structs, interfaces, RequestEditor loop, path
formatting, compile-validity) is unchanged and verified by the existing
`dev_test.go` suite including `go build` on generated SDKs.

**Test updates required**: `dev_test.go:141` and `dev_test.go:157` assert a
trailing space inside struct tag lines (e.g. `` `json:"role,omitempty"` `` +
trailing space). `go/format` strips trailing whitespace, so these two
assertions are trimmed of the trailing space.

### 5. Parallel AST scanning with single read

In `internal/astscan`:

- `walkPath`: collect `.go` files, process with a bounded worker pool, buffer
  per-file output, and emit in original walk order so output stays
  deterministic.
- `ast fn`: parse each file once and reuse the read bytes (currently parses
  then re-reads via `readFile`), and call `collectHelpers` once, reusing the
  result (currently called twice at `cmd/ast.go:137` and `:158`).

### 6. Config flag const

In `cmd/runtime.go`, hoist `"config-file"` into a named const
`configFileFlagName`. Behavior unchanged; `TestResolveConfigFileFlag` stays.

## Testing

- Existing suites continue to pass: `cmd`, `cmd/root_test`, `cmd/config_test`,
  `cmd/cmd_test` (minus the moved `findStructInFile` test), `cmd/dev_test`
  (minus the two trimmed assertions), `cmd/ast_test`.
- New unit tests in `internal/astscan` for parallel scanning determinism and
  single-read behavior; in `internal/devgen` for go/format validation (bad
  output errors, valid output is gofmt-clean); in `internal/scaffold` for the
  moved `findStructInFile` test.
- Full gates per `AGENTS.md` before completion: `go build ./...`,
  `go vet ./...`, `go test -race -count=1 ./...`, `golangci-lint run ./...`.

## Risks

- **Embed vs. compile consistency**: the embedded `runtime.go`/`config.go` are
  the exact files min compiles, so no drift by construction.
- **Parallel output ordering**: buffered emission in walk order keeps output
  deterministic; tests verify.
- **go/format normalization**: only the two trailing-space assertions change;
  all other pinned substrings (`fmt.Sprintf` path lines, interface signatures)
  are format-stable.
- **Import cycle**: avoided because `cmd` imports `internal/*` and the internal
  packages never import `cmd`; scaffold runtime/config remain in `cmd`.
