package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	Debug      bool       `json:"debug" help:"Enable verbose debug logging."`
	DryRun     bool       `json:"dry-run" help:"Simulate execution without side effects."`
}

type CoreConfig struct {
	Timeout time.Duration `json:"timeout" default:"2m"`
	Retries int           `json:"retries" default:"3"`
}

type CLI struct {
	ConfigFile kong.ConfigFlag `help:"Path to config file." placeholder:"PATH"`

	Config ConfigCmdGroup `cmd:"" help:"Manage application configuration"`
	Greet  GreetCmd       `cmd:"" help:"Print a personalized greeting message"`
}

// Execute wires up the met CLI and runs it.
// To reuse this pattern in another project, call ExecuteCLI with your own cli and cfg structs.
func main() {
	ExecuteCLI(&CLI{}, &Config{}, AppDescription)
}

// ExecuteCLI is a generic, project-agnostic entry point for any kong-based CLI.
//
// Usage in any project:
//
//	type MyCLI struct {
//	    Foo FooCmdGroup `cmd:"" help:"..."`
//	}
//	type MyConfig struct {
//	    Token string `json:"token" default:"..."`
//	}
//	func main() { cmd.ExecuteCLI(&MyCLI{}, &MyConfig{}, "My tool description") }
//
// Config fields tagged with `default:"<value>"` are applied before config-file / env resolution.
// Config is loaded from a JSON file at one of these locations (first match wins):
//  1. --config-file flag
//  2. <APP>_CONFIG environment variable
//  3. os.UserConfigDir()/<appName>/<appName>.json
//
// All config keys are also available as env vars prefixed with the uppercased app name.
func ExecuteCLI(cli any, cfg any, description string) {
	appName := resolveAppName()
	configFile := resolveConfigFile(appName)

	applyStructDefaults(cfg)

	flatCache := buildFlatCache(configFile, cfg)

	jsonResolver := kong.ResolverFunc(func(_ *kong.Context, _ *kong.Path, flag *kong.Flag) (any, error) {
		if val, ok := flatCache[flag.Name]; ok {
			return val, nil
		}
		return nil, nil
	})

	ctx := kong.Parse(cli,
		kong.Name(appName),
		kong.Description(description),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Tree:    true,
		}),
		kong.DefaultEnvars(strings.ToUpper(appName)),
		kong.Resolvers(jsonResolver),
	)

	ctx.Bind(cfg)
	ctx.Bind(ConfigPath(configFile))
	ctx.BindTo(context.Background(), (*context.Context)(nil))

	ctx.FatalIfErrorf(ctx.Run())
}

// resolveAppName derives the app name from the running executable,
// falling back to DefaultAppName during development / testing.
func resolveAppName() string {
	name := filepath.Base(os.Args[0])
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" || name == "main" || name == "app" ||
		strings.HasPrefix(name, "go-build") || strings.HasSuffix(name, ".test") {
		return DefaultAppName
	}
	return name
}

// resolveConfigFile returns the config file path by checking (in order):
// --config-file flag, <APP>_CONFIG env var, default OS config dir.
func resolveConfigFile(appName string) string {
	for i, arg := range os.Args {
		if arg == "--config-file" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if after, found := strings.CutPrefix(arg, "--config-file="); found {
			return after
		}
	}
	envKey := fmt.Sprintf("%s_CONFIG", strings.ToUpper(appName))
	if configFile := os.Getenv(envKey); configFile != "" {
		return configFile
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, appName, appName+".json")
	}
	return appName + ".json"
}

// buildFlatCache reads the JSON config file, populates cfg, and returns
// a flat key→value map used as a kong.Resolver for flag defaults.
func buildFlatCache(configFile string, cfg any) map[string]any {
	flat := make(map[string]any)
	data, err := os.ReadFile(configFile)
	if err != nil {
		return flat
	}
	var rawMap map[string]any
	if err := json.Unmarshal(data, &rawMap); err == nil {
		flattenMap(rawMap, "", flat)
	}
	_ = json.Unmarshal(data, cfg)
	return flat
}

// flattenMap recursively flattens a nested JSON map into dot-joined keys
// with dashes as separators (matching kong's flag naming convention).
func flattenMap(raw map[string]any, prefix string, out map[string]any) {
	for key, val := range raw {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "-" + key
		}
		if subMap, ok := val.(map[string]any); ok {
			flattenMap(subMap, fullKey, out)
		} else {
			out[fullKey] = val
		}
	}
}

// applyStructDefaults walks any struct and sets fields to their `default:"..."` tag
// value when the field is still at its zero value. Recurses into nested structs.
func applyStructDefaults(s any) {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fv := v.Field(i)
		ft := t.Field(i)
		if !fv.CanSet() {
			continue
		}
		if fv.Kind() == reflect.Struct {
			if fv.CanAddr() {
				applyStructDefaults(fv.Addr().Interface())
			}
			continue
		}
		defaultVal, ok := ft.Tag.Lookup("default")
		if !ok || !fv.IsZero() {
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(defaultVal)
		case reflect.Bool:
			fv.SetBool(defaultVal == "true")
		case reflect.Int:
			if n, err := strconv.Atoi(defaultVal); err == nil {
				fv.SetInt(int64(n))
			}
		case reflect.Int64:
			if fv.Type() == reflect.TypeOf(time.Duration(0)) {
				if d, err := time.ParseDuration(defaultVal); err == nil {
					fv.SetInt(int64(d))
				}
			} else {
				if n, err := strconv.ParseInt(defaultVal, 10, 64); err == nil {
					fv.SetInt(n)
				}
			}
		}
	}
}
