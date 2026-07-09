package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/alecthomas/kong"
)

// CLI is the root CLI struct containing all subcommand groups.
type CLI struct {
	Dev        DevCmdGroup    `cmd:"" help:"Developer utilities (cURL, HAR, OpenAPI generator)"`
	ConfigFile string         `help:"Config file path" json:"-"`

	Version  VersionCmd     `cmd:"" help:"Show version"`
	Ast      AstCmdGroup    `cmd:"" help:"Parse Go AST for AI context"`
	Init     InitCmd        `cmd:"" help:"Initialize a new CLI project"`
	Cmd      CommandGroup   `cmd:"" help:"Manage commands"`
	Config   ConfigCmdGroup `cmd:"" help:"Manage application configuration"`
}

const appName = "min"

var Version = "dev"
var cfgPath string

func init() {
	cfgPath = resolveConfigPath()
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
}

// SetConfigPath overrides the config file path used by config commands.
func SetConfigPath(p string) {
	if p == "" {
		cfgPath = resolveConfigPath()
	} else {
		cfgPath = p
	}
}

func CfgPath() string { return cfgPath }

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
		if (arg == "-c" || arg == "--config-file") && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "-c=") || strings.HasPrefix(arg, "--config-file=") {
			parts := strings.SplitN(arg, "=", 2)
			return parts[1]
		}
	}
	return ""
}

type VersionCmd struct{}

func (c *VersionCmd) Run() error {
	fmt.Println(Version)
	return nil
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
