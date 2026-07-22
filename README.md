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

Commands and flags are defined with `cli` struct tags:

```go
type GreetCmd struct {
    Name  string `cli:"help=Name to greet,default=World,arg"`
    Shout bool   `cli:"help=Convert to uppercase,short=s"`
    Times int    `cli:"help=Repeat count,default=1,short=t"`
}
```

| Tag | Use |
|-----|-----|
| `help=text` | Help text for `--help` output |
| `short=x` | Short flag alias (`-x`) |
| `default=val` | Default value when none provided |
| `required` | Fail if flag is missing |
| `arg` | Positional argument instead of flag |
| `cmd` | Marks a struct field as a subcommand |
| `placeholder=X` | Placeholder name in help output |

## Subcommands

Nested subcommands are supported at any depth:

```go
type CLI struct {
    Config ConfigCmdGroup  `cli:"help=Manage configuration,cmd"`
    Greet  GreetCmd        `cli:"help=Print a greeting,cmd"`
}
```

A default subcommand can be set with `cli.WithDefaultCmd("name")`.

## Combined short flags

`-abc` expands to `-a -b -c`. Non-bool flags consume the remaining
characters: `-nAlice` sets the `-n` flag to `"Alice"`.

## Slice accumulation

Repeated flags accumulate into `[]string` fields:

```
--tag a --tag b -t c  →  Tags=["a", "b", "c"]
```

## Env vars

Environment variables are auto-derived from flag names with the configured
prefix. `--core-timeout` with prefix `MIN_` becomes `MIN_CORE_TIMEOUT`.

## Config file

JSON file loaded before parsing. Supports both formats:

```json
{"core-timeout": "5m", "core-retries": 3}
```

```json
{"core": {"timeout": "5m", "retries": 3}}
```

Config file path resolved in order:
1. `--config-file` CLI flag
2. `$APPNAME_CONFIG_FILE` env var
3. `appname.json` in current directory
4. XDG config directory

## Interactive prompting

When a required flag is missing on a terminal, the library prompts
interactively for a value. Skip with `-y` / `--yes`.

## "Did you mean?" suggestions

Unknown flag names get Levenshtein-based suggestions:

```
error: unknown flag "--verose", did you mean --verbose?
```

## Library

The entire CLI parser is in `cli/cli.go` — zero external dependencies.
The application commands are in `cmd/`. `main.go` is the entry point:

```go
package main

import "min/cmd"

func main() {
    cmd.Run()
}
```
