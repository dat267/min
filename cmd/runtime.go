package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/kong"
)

// appName is the application name used for config path resolution. It is "min"
// for this binary and is overridden in generated projects via SetAppName.
var appName = "min"

const configFileFlagName = "config-file"

var (
	cfgPathMu sync.RWMutex
	cfgPath   string
)

// SetAppName overrides the application name used for config path resolution.
func SetAppName(name string) {
	if name != "" {
		appName = name
	}
}

// SetConfigPath overrides the config file path used by config commands.
func SetConfigPath(p string) {
	cfgPathMu.Lock()
	defer cfgPathMu.Unlock()
	cfgPath = p
}

// CfgPath returns the configured config file path, resolving a default lazily
// when none has been set.
func CfgPath() string {
	cfgPathMu.RLock()
	p := cfgPath
	cfgPathMu.RUnlock()
	if p != "" {
		return p
	}
	return resolveConfigPath()
}

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
		if arg == "--"+configFileFlagName && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "--"+configFileFlagName+"=") {
			return strings.SplitN(arg, "=", 2)[1]
		}
	}
	return ""
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
