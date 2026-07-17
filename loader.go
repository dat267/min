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

// RunCLI resolves configuration from defaults, config file, and env vars,
// parses CLI args via Kong, and executes the active command.
func RunCLI(cli any, cfg any, defaultAppName string, description string) {
	resolveAppName := func() string {
		name := filepath.Base(os.Args[0])
		name = strings.TrimSuffix(name, filepath.Ext(name))
		if name == "" || name == "main" || name == "app" ||
			strings.HasPrefix(name, "go-build") || strings.HasSuffix(name, ".test") {
			return defaultAppName
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

	// String formatting helper (PascalCase to kebab-case).
	kebabCase := func(s string) string {
		var sb strings.Builder
		sb.Grow(len(s) + 4)
		for i, r := range s {
			if i > 0 && r >= 'A' && r <= 'Z' {
				sb.WriteRune('-')
			}
			if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r + ('a' - 'A'))
			} else {
				sb.WriteRune(r)
			}
		}
		return sb.String()
	}

	// Helper to set a reflect.Value from a string.
	setFieldValue := func(fv reflect.Value, s string) {
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(s)
		case reflect.Bool:
			fv.SetBool(s == "true" || s == "1")
		case reflect.Int, reflect.Int64:
			if fv.Type() == reflect.TypeFor[time.Duration]() {
				if d, err := time.ParseDuration(s); err == nil {
					fv.SetInt(int64(d))
				} else if ns, err := strconv.ParseInt(s, 10, 64); err == nil {
					fv.SetInt(ns)
				}
			} else {
				if n, err := strconv.ParseInt(s, 10, 64); err == nil {
					fv.SetInt(n)
				}
			}
		}
	}

	// Recursive loader for environment variables and default tags.
	var loadEnvAndDefaults func(reflect.Value, string, string)
	loadEnvAndDefaults = func(val reflect.Value, prefix string, envPrefix string) {
		val = reflect.Indirect(val)
		if val.Kind() != reflect.Struct {
			return
		}
		t := val.Type()
		for i := 0; i < val.NumField(); i++ {
			fv := val.Field(i)
			ft := t.Field(i)
			if !fv.CanSet() {
				continue
			}

			name := kebabCase(ft.Name)
			if jsonTag := ft.Tag.Get("json"); jsonTag != "" {
				if parts := strings.Split(jsonTag, ","); parts[0] != "" && parts[0] != "-" {
					name = parts[0]
				}
			}

			fullKey := name
			if prefix != "" {
				fullKey = prefix + "-" + name
			}

			envKey := envPrefix + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))

			if fv.Kind() == reflect.Struct && fv.Type() != reflect.TypeFor[time.Duration]() {
				loadEnvAndDefaults(fv, fullKey, envKey+"_")
				continue
			}

			// Env override has higher precedence.
			if val, ok := os.LookupEnv(envKey); ok {
				setFieldValue(fv, val)
				continue
			}

			// Apply struct default tag if value is still zero.
			if defaultVal, ok := ft.Tag.Lookup("default"); ok && fv.IsZero() {
				setFieldValue(fv, defaultVal)
			}
		}
	}

	// Recursive helper to build a flat map of config keys to reflect.Values.
	var buildFlatMap func(reflect.Value, string, map[string]reflect.Value)
	buildFlatMap = func(val reflect.Value, prefix string, dest map[string]reflect.Value) {
		val = reflect.Indirect(val)
		if val.Kind() != reflect.Struct {
			return
		}
		t := val.Type()
		for i := 0; i < val.NumField(); i++ {
			fv := val.Field(i)
			ft := t.Field(i)

			name := kebabCase(ft.Name)
			if jsonTag := ft.Tag.Get("json"); jsonTag != "" {
				if parts := strings.Split(jsonTag, ","); parts[0] != "" && parts[0] != "-" {
					name = parts[0]
				}
			}

			fullKey := name
			if prefix != "" {
				fullKey = prefix + "-" + name
			}

			if fv.Kind() == reflect.Struct && fv.Type() != reflect.TypeFor[time.Duration]() {
				buildFlatMap(fv, fullKey, dest)
			} else {
				dest[fullKey] = fv
				if prefix != "" {
					dest[name] = fv
				}
			}
		}
	}

	// 1. Load config file values.
	if data, err := os.ReadFile(filepath.Clean(configFile)); err == nil {
		_ = json.Unmarshal(data, cfg)
	}

	// 2. Load env overrides and apply struct default tags.
	envPrefix := strings.ToUpper(appName) + "_"
	loadEnvAndDefaults(reflect.ValueOf(cfg), "", envPrefix)

	// 3. Build a flat map of config keys to reflect.Values for the resolver and flag syncing.
	configFields := make(map[string]reflect.Value)
	buildFlatMap(reflect.ValueOf(cfg), "", configFields)

	// Resolver to supply configuration values as defaults for subcommand flags.
	configResolver := kong.ResolverFunc(func(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		if fv, ok := configFields[flag.Name]; ok {
			if fv.Type() == reflect.TypeFor[time.Duration]() {
				return fv.Interface().(time.Duration).String(), nil
			}
			return fmt.Sprintf("%v", fv.Interface()), nil
		}
		return nil, nil
	})

	ctx := kong.Parse(cli,
		kong.Name(appName),
		kong.Description(description),
		kong.UsageOnError(),
		kong.DefaultEnvars(strings.ToUpper(appName)),
		kong.Resolvers(configResolver),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Tree:    true,
		}),
	)

	// Sync any resolved subcommand flags back to the runtime configuration.
	for _, flag := range ctx.Flags() {
		if fv, ok := configFields[flag.Name]; ok {
			fv.Set(flag.Target)
		}
	}

	ctx.Bind(cfg)
	ctx.Bind(ConfigPath(configFile))
	ctx.BindTo(context.Background(), (*context.Context)(nil))

	ctx.FatalIfErrorf(ctx.Run())
}
