package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
)

const (
	DefaultAppName = "min"
	AppDescription = "Internal workflows and troubleshooting utility"
)

type ConfigPath string

type Config struct {
	AdminToken string     `json:"admin-token"`
	Core       CoreConfig `json:"core" kong:"embed,prefix=core-"`
}

type CoreConfig struct {
	Timeout time.Duration `json:"timeout" default:"2m"`
	Retries int           `json:"retries" default:"3"`
}

type CLI struct {
	ConfigFile string `help:"Path to config file" type:"path" name:"config"`
	AppConfig  Config `kong:"embed"`

	ConfigCmd ConfigCmdGroup `cmd:"" name:"config" help:"Manage application configuration"`
	Greet     GreetCmd       `cmd:"" help:"Print a personalized greeting message"`
}

func main() {
	var cli CLI

	// Determine binary execution name, falling back to default during local development/testing
	appName := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	if appName == "" || appName == "main" || appName == "app" || strings.HasPrefix(appName, "go-build") || strings.HasSuffix(appName, ".test") {
		appName = DefaultAppName
	}

	// Resolve configuration target path: Envar -> OS User Config Dir -> Current Dir
	configFile := os.Getenv(strings.ToUpper(appName) + "_CONFIG")
	if configFile == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			configFile = filepath.Join(dir, appName, appName+".json")
		} else {
			configFile = appName + ".json"
		}
	}
	// Direct string inspection to capture the config file override before Kong parses arguments
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			configFile = os.Args[i+1]
		}
		if after, found := strings.CutPrefix(arg, "--config="); found {
			configFile = after
		}
	}

	// Recursively flatten nested JSON maps into hyphen-delimited cache keys with collision checking
	flatCache := make(map[string]any)
	var flattenMap func(raw map[string]any, prefix string) error
	flattenMap = func(raw map[string]any, prefix string) error {
		for key, val := range raw {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "-" + key
			}
			if subMap, ok := val.(map[string]any); ok {
				if err := flattenMap(subMap, fullKey); err != nil {
					return err
				}
			} else {
				if _, collision := flatCache[fullKey]; collision {
					return fmt.Errorf("in %s: duplicate configuration key collision detected: %q", configFile, fullKey)
				}
				flatCache[fullKey] = val
			}
		}
		return nil
	}

	// Extract external file values into the flat target cache if the profile exists
	if data, err := os.ReadFile(configFile); err == nil {
		var rawMap map[string]any
		if err := json.Unmarshal(data, &rawMap); err == nil {
			if err := flattenMap(rawMap, ""); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Feed matching keys from the flat JSON map back into Kong's fallback resolution stack
	jsonResolver := kong.ResolverFunc(func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		if val, ok := flatCache[flag.Name]; ok {
			return val, nil
		}
		return nil, nil
	})

	// Initialize configuration parsing tree, resolving variables and adjusting help menus
	ctx := kong.Parse(&cli,
		kong.Name(appName),
		kong.Description(AppDescription),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Tree:    true,
		}),
		kong.DefaultEnvars(strings.ToUpper(appName)),
		kong.Resolvers(jsonResolver),
		kong.PostBuild(func(k *kong.Kong) error {
			// Walk flag and argument metadata models to dynamically inject default text indicators
			var appendDefaults func(*kong.Node)
			appendDefaults = func(n *kong.Node) {
				if n == nil {
					return
				}
				for _, f := range n.Flags {
					if f.Default != "" && !strings.Contains(strings.ToLower(f.Help), "default:") {
						f.Help = fmt.Sprintf("%s (default: %s)", f.Help, f.Default)
					}
				}
				for _, p := range n.Positional {
					if p.Default != "" && !strings.Contains(strings.ToLower(p.Help), "default:") {
						p.Help = fmt.Sprintf("%s (default: %s)", p.Help, p.Default)
					}
				}
				for _, child := range n.Children {
					appendDefaults(child)
				}
			}
			appendDefaults(k.Model.Node)
			return nil
		}),
	)

	// Bind global parameters and runtime pointers securely into the environment context by explicit type
	ctx.Bind(&cli.AppConfig)
	ctx.Bind(ConfigPath(configFile))
	ctx.BindTo(context.Background(), (*context.Context)(nil))
	ctx.FatalIfErrorf(ctx.Run())
}
