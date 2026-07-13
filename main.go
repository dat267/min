package main

import (
	"context"
	"encoding/json"
	"io"
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
	Debug      bool       `json:"debug" help:"Enable verbose debug logging."`
	DryRun     bool       `json:"dry-run" help:"Simulate execution without side effects."`
}

type CoreConfig struct {
	Timeout time.Duration `json:"timeout" default:"2m"`
	Retries int           `json:"retries" default:"3"`
}

type CLI struct {
	ConfigFile kong.ConfigFlag `help:"Path to config file." placeholder:"PATH"`
	AppConfig  Config          `kong:"embed"`

	Config ConfigCmdGroup `cmd:"" help:"Manage application configuration"`
	Greet  GreetCmd       `cmd:"" help:"Print a personalized greeting message"`
}

func main() {
	var cli CLI

	appName := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	if appName == "" || appName == "main" || appName == "app" || strings.HasPrefix(appName, "go-build") || strings.HasSuffix(appName, ".test") {
		appName = DefaultAppName
	}

	defaultConfigFile := os.Getenv(strings.ToUpper(appName) + "_CONFIG")
	if defaultConfigFile == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			defaultConfigFile = filepath.Join(dir, appName, appName+".json")
		} else {
			defaultConfigFile = appName + ".json"
		}
	}

	jsonLoader := func(r io.Reader) (kong.Resolver, error) {
		var raw map[string]any
		if err := json.NewDecoder(r).Decode(&raw); err != nil {
			return nil, err
		}

		flatCache := make(map[string]any)
		var flatten func(m map[string]any, prefix string)
		flatten = func(m map[string]any, prefix string) {
			for k, v := range m {
				key := k
				if prefix != "" {
					key = prefix + "-" + k
				}
				if sub, ok := v.(map[string]any); ok {
					flatten(sub, key)
				} else {
					flatCache[key] = v
				}
			}
		}
		flatten(raw, "")

		return kong.ResolverFunc(func(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
			return flatCache[flag.Name], nil
		}), nil
	}

	ctx := kong.Parse(&cli,
		kong.Name(appName),
		kong.Description(AppDescription),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Tree:    true,
		}),
		kong.DefaultEnvars(strings.ToUpper(appName)),
		kong.Configuration(jsonLoader, defaultConfigFile),
	)

	configFile := string(cli.ConfigFile)
	if configFile == "" {
		configFile = defaultConfigFile
	}

	ctx.Bind(&cli.AppConfig)
	ctx.Bind(ConfigPath(configFile))
	ctx.BindTo(context.Background(), (*context.Context)(nil))
	ctx.FatalIfErrorf(ctx.Run())
}
