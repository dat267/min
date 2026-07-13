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
	Debug   bool          `json:"debug" help:"Enable verbose debug logging."`
	DryRun  bool          `json:"dry-run" help:"Simulate execution without side effects."`
}

type CLI struct {
	ConfigFile string `help:"Path to config file" type:"path" name:"config"`
	JSON       bool   `help:"Output results in JSON format." short:"j"`
	AppConfig  Config `kong:"embed"`

	Config ConfigCmdGroup `cmd:"" help:"Manage application configuration"`
	Greet  GreetCmd       `cmd:"" help:"Print a personalized greeting message"`
}

func main() {
	var cli CLI

	appName := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	if appName == "" || appName == "main" || appName == "app" || strings.HasPrefix(appName, "go-build") || strings.HasSuffix(appName, ".test") {
		appName = DefaultAppName
	}

	configFile := os.Getenv(strings.ToUpper(appName) + "_CONFIG")
	if configFile == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			configFile = filepath.Join(dir, appName, appName+".json")
		} else {
			configFile = appName + ".json"
		}
	}
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			configFile = os.Args[i+1]
		}
		if after, found := strings.CutPrefix(arg, "--config="); found {
			configFile = after
		}
	}

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

	if data, err := os.ReadFile(configFile); err == nil {
		var rawMap map[string]any
		if err := json.Unmarshal(data, &rawMap); err == nil {
			if err := flattenMap(rawMap, ""); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	}

	jsonResolver := kong.ResolverFunc(func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		if val, ok := flatCache[flag.Name]; ok {
			return val, nil
		}
		return nil, nil
	})

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
	)

	ctx.Bind(&cli.AppConfig)
	ctx.Bind(ConfigPath(configFile))
	ctx.BindTo(context.Background(), (*context.Context)(nil))
	ctx.FatalIfErrorf(ctx.Run())
}
