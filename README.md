# min

Zero-dependency CLI framework with struct-tag parsing, env/config resolution, and interactive prompting.

```
Usage: min <command> [flags]

Commands:
  config    Manage application configuration
  greet     Print a personalized greeting message

Flags:
  -h, --help                  Print help
  -y, --yes                   Skip interactive prompts [env: MIN_YES]
      --config-file <PATH>    Path to config file [env: MIN_CONFIG_FILE]
      --admin-token <ADMIN-TOKEN>  Admin token [env: MIN_ADMIN_TOKEN]
      --core-timeout <CORE-TIMEOUT>  Core timeout [default: 10s] [env: MIN_CORE_TIMEOUT]
      --core-retries <CORE-RETRIES>  Core retries [default: 3] [env: MIN_CORE_RETRIES]
      --debug                 Enable debug mode [env: MIN_DEBUG]
      --dry-run               Enable dry run mode [env: MIN_DRY_RUN]
```

## Resolution order

```
CLI args  >  env vars  >  config file  >  struct defaults
```

Every source overrides the next. Config file supports both nested
(`{"core":{"timeout":"5m"}}`) and flat (`{"core-timeout":"5m"}`) keys.

## Struct tags

Flags and arguments use Go struct tags:

```go
type GreetCmd struct {
    Name  string `help:"Name to greet" default:"World" arg:""`
    Shout bool   `help:"Convert to uppercase" short:"s"`
    Times int    `help:"Repeat count" default:"1" short:"t"`
}
```

| Tag | Use |
|-----|-----|
| `help:"text"` | Help text for `--help` output |
| `short:"x"` | Short flag alias (`-x`) |
| `default:"val"` | Default value when none provided |
| `required:""` | Fail if flag is missing |
| `arg:""` | Positional argument instead of flag |
| `cmd:""` | Marks a struct field as a subcommand |
| `placeholder:"X"` | Placeholder name in help output |

## Config schema

Config values exposed via `json:"key"` tags. The `Cmd` struct is the
single source of truth — `ConfigFields()` and `ConfigDefaults()` use
reflect to read the schema:

```go
type Cmd struct {
    AdminToken  string `help:"Admin token" json:"admin-token"`
    CoreTimeout string `help:"Core timeout" default:"10s" json:"core-timeout"`
    CoreRetries int    `help:"Core retries" default:"3" json:"core-retries"`
    Debug       bool   `help:"Enable debug mode" json:"debug"`
    DryRun      bool   `help:"Enable dry run mode" json:"dry-run"`
    Config      ConfigCmdGroup `help:"Manage configuration" cmd:""`
    Greet       GreetCmd       `help:"Print a greeting" cmd:"""`
}
```

## Subcommands

Nested subcommands are supported at any depth. A default subcommand
can be set with `cli.WithDefaultCmd("name")`.

## Combined short flags

`-abc` expands to `-a -b -c`. Non-bool flags consume the remaining
characters: `-nAlice` sets the `-n` flag to `"Alice"`.

## Slice accumulation

Repeated flags accumulate into `[]string` fields:

```
--tag a --tag b -t c  →  Tags=["a", "b", "c"]
```

## Env vars

Auto-derived from flag names with the configured prefix:
`--core-timeout` with prefix `MIN_` becomes `MIN_CORE_TIMEOUT`.

## Config file

JSON file loaded before parsing. Path resolved in order:
1. `--config-file` CLI flag
2. `$APPNAME_CONFIG_FILE` env var
3. `appname.json` in current directory
4. XDG config directory

## Interactive prompting

When a required flag is missing on a terminal, the library prompts
interactively. Skip with `-y` / `--yes`.

## Errors

All parse errors show the relevant help first, then the specific
problem. No "did you mean?" suggestions — just help + error.

## Structure

```
main.go          — entry point: calls cmd.Execute()
cli/cli.go       — generic parser, zero dependencies, ~640 lines
cli/cli_test.go  — parser unit tests
cmd/cmd.go       — Cmd struct, Execute(), config schema helpers
cmd/greet.go     — GreetCmd
cmd/config.go    — Config command group
cmd/cmd_test.go  — command unit tests
main_test.go     — integration tests
```

```go
package main

import "min/cmd"

func main() {
    cmd.Execute()
}
```
