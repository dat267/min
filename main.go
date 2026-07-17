package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	Core       CoreConfig `json:"core"`
	Debug      bool       `json:"debug"`
	DryRun     bool       `json:"dry-run"`
}

type CoreConfig struct {
	Timeout time.Duration `json:"timeout"`
	Retries int           `json:"retries"`
}

type CLI struct {
	ConfigFile string `help:"Path to config file." placeholder:"PATH"`

	Config ConfigCmdGroup `cmd:"" help:"Manage application configuration"`
	Greet  GreetCmd       `cmd:"" help:"Print a personalized greeting message"`
}

func main() {
	resolveAppName := func() string {
		name := filepath.Base(os.Args[0])
		name = strings.TrimSuffix(name, filepath.Ext(name))
		if name == "" || name == "main" || name == "app" ||
			strings.HasPrefix(name, "go-build") || strings.HasSuffix(name, ".test") {
			return DefaultAppName
		}
		return name
	}

	resolveConfigFile := func(appName string) string {
		for i, arg := range os.Args {
			if arg == "--config-file" && i+1 < len(os.Args) {
				return os.Args[i+1]
			}
			if after, found := strings.CutPrefix(arg, "--config-file="); found {
				return after
			}
		}
		envKey := strings.ToUpper(appName) + "_CONFIG"
		if configFile := os.Getenv(envKey); configFile != "" {
			return configFile
		}
		if dir, err := os.UserConfigDir(); err == nil {
			return filepath.Join(dir, appName, appName+".json")
		}
		return appName + ".json"
	}

	appName := resolveAppName()
	configFile := resolveConfigFile(appName)

	// 1. Initialize runtime config with hardcoded defaults.
	runtimeCfg := &Config{
		Core: CoreConfig{
			Timeout: 2 * time.Minute,
			Retries: 3,
		},
	}

	// 2. Override with JSON config file values if present.
	if data, err := os.ReadFile(filepath.Clean(configFile)); err == nil {
		_ = json.Unmarshal(data, runtimeCfg)
	}

	// 3. Override with environment variables.
	getEnv := func(keys ...string) (string, bool) {
		for _, k := range keys {
			if val, ok := os.LookupEnv(k); ok {
				return val, true
			}
		}
		return "", false
	}

	appNameUpper := strings.ToUpper(appName)
	if val, ok := getEnv(appNameUpper + "_ADMIN_TOKEN"); ok {
		runtimeCfg.AdminToken = val
	}
	if val, ok := getEnv(appNameUpper+"_CORE_TIMEOUT", appNameUpper+"_TIMEOUT"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			runtimeCfg.Core.Timeout = d
		} else if ns, err := strconv.ParseInt(val, 10, 64); err == nil {
			runtimeCfg.Core.Timeout = time.Duration(ns)
		}
	}
	if val, ok := getEnv(appNameUpper+"_CORE_RETRIES", appNameUpper+"_RETRIES"); ok {
		if r, err := strconv.Atoi(val); err == nil {
			runtimeCfg.Core.Retries = r
		}
	}
	if val, ok := getEnv(appNameUpper + "_DEBUG"); ok {
		runtimeCfg.Debug = (val == "true" || val == "1")
	}
	if val, ok := getEnv(appNameUpper + "_DRY_RUN"); ok {
		runtimeCfg.DryRun = (val == "true" || val == "1")
	}

	resolveKeys := func(name string) []string {
		if suffix, found := strings.CutPrefix(name, "core-"); found {
			return []string{name, suffix}
		}
		return []string{name, "core-" + name}
	}

	// Helper to get resolved config values for a given flag name.
	resolveConfigValue := func(name string) (any, bool) {
		for _, key := range resolveKeys(name) {
			switch key {
			case "core-timeout":
				return runtimeCfg.Core.Timeout.String(), true
			case "core-retries":
				return fmt.Sprintf("%d", runtimeCfg.Core.Retries), true
			case "admin-token":
				return runtimeCfg.AdminToken, true
			case "debug":
				return fmt.Sprintf("%t", runtimeCfg.Debug), true
			case "dry-run":
				return fmt.Sprintf("%t", runtimeCfg.DryRun), true
			}
		}
		return nil, false
	}

	// Resolver to supply configuration values as defaults for subcommand flags.
	configResolver := kong.ResolverFunc(func(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		if val, ok := resolveConfigValue(flag.Name); ok {
			return val, nil
		}
		return nil, nil
	})

	cli := &CLI{}
	ctx := kong.Parse(cli,
		kong.Name(appName),
		kong.Description(AppDescription),
		kong.UsageOnError(),
		kong.Resolvers(configResolver),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Tree:    true,
		}),
	)

	// Sync any resolved subcommand flags back to the runtime configuration.
	for _, flag := range ctx.Flags() {
		for _, key := range resolveKeys(flag.Name) {
			switch key {
			case "core-timeout":
				if d, ok := flag.Target.Interface().(time.Duration); ok {
					runtimeCfg.Core.Timeout = d
				}
			case "core-retries":
				if r, ok := flag.Target.Interface().(int); ok {
					runtimeCfg.Core.Retries = r
				}
			case "admin-token":
				if s, ok := flag.Target.Interface().(string); ok {
					runtimeCfg.AdminToken = s
				}
			case "debug":
				if b, ok := flag.Target.Interface().(bool); ok {
					runtimeCfg.Debug = b
				}
			case "dry-run":
				if b, ok := flag.Target.Interface().(bool); ok {
					runtimeCfg.DryRun = b
				}
			}
		}
	}

	ctx.Bind(runtimeCfg)
	ctx.Bind(ConfigPath(configFile))
	ctx.BindTo(context.Background(), (*context.Context)(nil))

	ctx.FatalIfErrorf(ctx.Run())
}
