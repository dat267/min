# min

Zero-dependency CLI framework with struct-tag parsing, env/config resolution, and interactive prompting.

```
Usage: min <command> [flags]

Commands:
   config    Manage application configuration
  greet     Print a personalized greeting message
  edge      Showcase all flag types and edge cases

Flags:
  -h, --help                  Print help
  -y, --yes                   Skip interactive prompts
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
| `choices:"a,b,c"` | Comma-separated allowed values |
| `hidden:""` | Hide from `--help` output |

## Config schema

Config values exposed via `json:"key"` tags on `Cmd`. `ConfigFields()`
and `ConfigDefaults()` use reflect to read the schema — no repetition.

```go
type Cmd struct {
    AdminToken  string `help:"Admin token" json:"admin-token"`
    CoreTimeout string `help:"Core timeout" default:"10s" json:"core-timeout"`
    CoreRetries int    `help:"Core retries" default:"3" json:"core-retries"`
    Debug       bool   `help:"Enable debug mode" json:"debug"`
    DryRun      bool   `help:"Enable dry run mode" json:"dry-run"`
    Config      ConfigCmdGroup `cmd:"" help:"Manage configuration"`
    Greet       GreetCmd       `cmd:"" help:"Print a greeting"`
    Edge        EdgeCmd        `cmd:"" help:"Showcase all flag types"`
}
```

## Subcommands

Nested subcommands supported at any depth. Default subcommand via
`cli.WithDefaultCmd("name")`. Root executable by adding `Run()` to `Cmd`.

## Interactive prompting

Required flags (`required:""`) prompt interactively on a terminal, like
PowerShell's `[Parameter(Mandatory=$true)]`. Skip with `-y` / `--yes`.

```
$ min edge --help
```

Piped stdin or CI skips prompting automatically — shows help + error instead.

## Other features

- **Combined short flags** — `-abc` = `-a -b -c`, `-nVal` sets value
- **Slice accumulation** — `--tag a --tag b` → `Tags=["a","b"]`
- **Env vars** — auto-derived from flag names with prefix
- **Global flags** — shared flags on `Cmd`, single-use flags on subcommands
- **Built-in flags** — `-y`/`--yes`, `--config-file`, `-h`/`--help`
- **Choices validation** — `choices:"a,b,c"` rejects invalid values with default fallback
- **Hidden flags/commands** — `hidden:""` suppresses display in help output

## Structure

```
main.go          — entry point: calls cmd.Execute()
cli/cli.go       — generic parser, zero dependencies, ~890 lines
cli/cli_test.go  — parser unit tests
cmd/cmd.go       — Cmd struct, Execute(), config schema helpers
cmd/greet.go     — GreetCmd
cmd/config.go    — Config command group
cmd/edge.go      — EdgeCmd
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
