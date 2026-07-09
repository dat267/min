# min

min is a CLI project scaffolding tool. It uses the [Kong](https://github.com/alecthomas/kong) library.

```
Usage: min <command> [flags]

CLI project scaffolding tool

Flags:
  -h, --help                  Show context-sensitive help.
      --config-file=STRING    Config file path

Commands:
  ast scan        Scan Go files, strip bodies, keep signatures
  ast types       Show types only (structs, interfaces, func signatures)
  ast fn          Extract context for a single function
  init            Initialize a new CLI project
  cmd add         Add a new command
  cmd show        List all commands
  cmd edit        Edit a command struct
  config init     Generate a default configuration file
  config path     Show configuration file path
  config show     Print current configuration values
  config set      Set a config value
  config unset    Unset a config value
```

## Quick start

```bash
# Install
go install github.com/dat267/min@latest

# Create a project
min init mycli
cd mycli && go mod tidy && go run . greet
# -> Hello, World!
```

## Config

Config is a JSON file. Config commands create parent directories automatically.

```bash
# Create a config file with struct defaults
min config init
min config show
# -> { }

# Set values
min config set greeting hello
min config set count 5
min config set enabled true
min config show
# -> { "greeting": "hello", "count": 5, "enabled": true }

# Dotted keys create nested objects
min config set core.timeout 5m
# -> { "core": { "timeout": "5m" } }

# Remove a key
min config unset greeting

# Use a custom config file
min --config-file /path/to/config.json config set key value
```

The `--config-file` flag is a root-level flag. It must come before the command name. It does not appear inside the config file.

Config defaults come from struct `default:"..."` tags. Kong loads these values.

Config keys can be flat or nested. Both of these set the same deeply-nested flag:

```json
{ "redash-api-key": "abc" }
{ "redash": { "api-key": "abc" } }
```

## Add a command

```bash
# Create cmd/greet.go and register it in the CLI struct
min cmd add greet

# Use dot notation for nested commands
min cmd add admin.users.list
```

Each command is a struct with a `Run() error` method. Add flags as struct fields.

## Kong struct tags

| Tag | Purpose |
|-----|---------|
| `help:"text"` | Help text |
| `short:"x"` | Short flag name (`-x`) |
| `default:"val"` | Default value |
| `required:""` | Fail if missing |
| `arg:""` | Positional argument |
| `cmd:""` | Subcommand group |
| `choices:"a,b,c"` | Allowed values |
| `hidden:""` | Do not show in help |
| `name:"custom"` | Override flag name |
| `env:"VAR"` | Environment variable |
| `format:"time.RFC3339"` | Time format |

## Generated project structure

```
mycli/
  main.go       -- entry point
  go.mod
  cmd/
    cmd.go      -- CLI struct and config commands
```

## Development

```bash
# Build with version
go build -ldflags="-X main.version=$(git describe --tags --always)" -o min . && ./min --version

# Run and install
go run .
go install .

# Test and lint
go test -race -count=1 ./...
golangci-lint run ./...
```
