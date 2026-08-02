# Config Dependency Injection Design

Date: 2026-08-02

## Context

`min`'s config commands read their config file path from package-global
mutable state in `cmd/runtime.go` (`cfgPath`, guarded by `cfgPathMu`), mutated
via `SetConfigPath`/`CfgPath`. This was noted as a scalability risk during the
earlier refactor and deferred. Global mutable state:

- prevents using the `cmd` package as a library with multiple independent
  config paths in one process;
- forces tests to reset global state instead of constructing fresh state;
- hides the dependency: config command `Run` methods silently depend on state
  wired up by `Execute` at a distance.

## Goal

Replace the global with an explicit `App` value type that owns the resolved
config path, injected into config command `Run` methods via Kong's `Bind`.

## Design

### `cmd/runtime.go`

Remove `cfgPath`, `cfgPathMu`, `SetConfigPath`, and the package-level
`CfgPath()`. Add:

```go
// App carries the resolved config file path to commands.
type App struct {
	cfgPath string
}

// CfgPath returns the config file path, resolving a default lazily when none
// is set. It is nil-safe so callers can never hit a nil dereference.
func (a *App) CfgPath() string {
	if a == nil || a.cfgPath == "" {
		return resolveConfigPath()
	}
	return a.cfgPath
}
```

Keep `appName` + `SetAppName(name)` global: it is set once at startup (or once
in a generated project's `init`), never mutated during a run, and moving it
into `App` would complicate the scaffold's `init()` override for no benefit.

Rewrite `Execute` to construct and bind the `App`:

```go
func Execute(ctx context.Context) {
	app := &App{}
	if cf := resolveConfigFileFlag(); cf != "" {
		app.cfgPath = cf
	}
	activeConfig := app.CfgPath()

	cli := &CLI{}
	options := []kong.Option{
		kong.Name(appName),
		kong.Description("CLI project scaffolding tool"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.Bind(app),
	}

	if f, err := os.Open(activeConfig); err == nil {
		if resolver, err := JSONResolver(f); err == nil {
			options = append(options, kong.Resolvers(resolver))
		}
		_ = f.Close()
	}

	k, err := kong.New(cli, options...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	kongCtx, err := k.Parse(os.Args[1:])
	k.FatalIfErrorf(err)

	app.cfgPath = cli.ConfigFile
	k.FatalIfErrorf(kongCtx.Run())
}
```

`resolveConfigPath`, `resolveConfigFileFlag`, and `JSONResolver` are unchanged.

### `cmd/config.go`

Every Run method gains an `*App` parameter and uses `app.CfgPath()`:

- `func (cmd *ConfigInitCmd) Run(app *App) error`
- `func (cmd *ConfigPathCmd) Run(app *App) error`
- `func (cmd *ConfigShowCmd) Run(app *App) error`
- `func (cmd *ConfigSetCmd) Run(app *App) error`
- `func (cmd *ConfigUnsetCmd) Run(app *App) error`
- `func (cmd *ConfigEditCmd) Run(app *App) error`

Kong injects the bound `*App` by type; the existing `context.Context` binding
(for dev commands) is unaffected.

### Embed constraint

`runtime.go` and `config.go` are embedded byte-identical into generated
projects (`cmd/templates.go`). Both remain self-contained (stdlib + kong only)
and compile identically in scaffolds, so no scaffold template changes are
needed: the copied config commands receive `*App` from the copied `Execute`.

### Tests

`cmd/config_test.go`:

- Replace `setupTestConfig` with `setupTestApp`:
  ```go
  func setupTestApp(t *testing.T) *App {
  	dir := t.TempDir()
  	return &App{cfgPath: filepath.Join(dir, "config.json")}
  }
  ```
- Every test passes the returned `*App` to `Run(app)`.
- Add `TestApp_IndependentPaths`: two `App`s with different paths set and
  read keys without cross-talk.
- `TestApp_CfgPathLazyFallback`: an `App` with an empty `cfgPath` resolves via
  `resolveConfigPath`.

`cmd/root_test.go` is unchanged (`resolveConfigFileFlag`, `JSONResolver`,
`SetAppName` behavior is preserved).

`TestInitCmd_GeneratedProjectRuns` continues to prove the generated project
runs with DI.

## Verification

`go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`,
`golangci-lint run ./...`, and the generated-project end-to-end test.

## Out of Scope

- Moving `appName`/`SetAppName` into `App` (remains global; see rationale).
- Any change to config file format or command behavior.
