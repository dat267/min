package main

import (
	"context"
	"encoding"
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

// Duration wraps time.Duration with JSON and text unmarshaling support
// for both duration strings (e.g. "5m") and raw integers (nanoseconds).
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Nanoseconds())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		var ns int64
		if err2 := json.Unmarshal(b, &ns); err2 != nil {
			return err
		}
		d.Duration = time.Duration(ns)
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func (d *Duration) UnmarshalText(b []byte) error {
	parsed, err := time.ParseDuration(string(b))
	if err != nil {
		if ns, err2 := strconv.ParseInt(string(b), 10, 64); err2 == nil {
			d.Duration = time.Duration(ns)
			return nil
		}
		return err
	}
	d.Duration = parsed
	return nil
}

type Config struct {
	AdminToken string     `json:"admin-token"`
	Core       CoreConfig `json:"core"`
	Debug      bool       `json:"debug"`
	DryRun     bool       `json:"dry-run"`
}

type CoreConfig struct {
	Timeout Duration `json:"timeout" default:"2m"`
	Retries int      `json:"retries" default:"3"`
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
		if fv.CanAddr() {
			if u, ok := fv.Addr().Interface().(encoding.TextUnmarshaler); ok {
				_ = u.UnmarshalText([]byte(s))
				return
			}
		}
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(s)
		case reflect.Bool:
			fv.SetBool(s == "true" || s == "1")
		case reflect.Int, reflect.Int64:
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				fv.SetInt(n)
			}
		}
	}

	type configField struct {
		value      reflect.Value
		defaultTag string
	}

	// Recursive helper to build a flat map of config keys to reflect.Values and defaults.
	configFields := make(map[string]configField)
	var buildFlatMap func(reflect.Value, string)
	buildFlatMap = func(val reflect.Value, prefix string) {
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

			if fv.Kind() == reflect.Struct && fv.Type() != reflect.TypeFor[Duration]() {
				buildFlatMap(fv, fullKey)
			} else {
				fieldInfo := configField{value: fv, defaultTag: ft.Tag.Get("default")}
				configFields[fullKey] = fieldInfo
				if prefix != "" {
					configFields[name] = fieldInfo
				}
			}
		}
	}

	runtimeCfg := &Config{}
	explicitlySet := make(map[string]bool)

	// 1. Load config file values.
	var rawMap map[string]any
	if data, err := os.ReadFile(filepath.Clean(configFile)); err == nil {
		_ = json.Unmarshal(data, runtimeCfg)
		_ = json.Unmarshal(data, &rawMap)
	}

	var markExplicit func(map[string]any, string)
	markExplicit = func(m map[string]any, prefix string) {
		for k, v := range m {
			key := k
			if prefix != "" {
				key = prefix + "-" + k
			}
			explicitlySet[key] = true
			explicitlySet[k] = true
			if sub, ok := v.(map[string]any); ok {
				markExplicit(sub, key)
			}
		}
	}
	markExplicit(rawMap, "")

	// 2. Build a flat map of config fields.
	buildFlatMap(reflect.ValueOf(runtimeCfg), "")

	// 3. Load env overrides and apply struct default tags to all fields.
	envPrefix := strings.ToUpper(appName) + "_"
	for key, field := range configFields {
		envKey := envPrefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		if val, ok := os.LookupEnv(envKey); ok {
			setFieldValue(field.value, val)
			explicitlySet[key] = true
			if idx := strings.LastIndex(key, "-"); idx != -1 {
				explicitlySet[key[idx+1:]] = true
			}
		} else if field.defaultTag != "" && field.value.IsZero() {
			setFieldValue(field.value, field.defaultTag)
		}
	}

	// Resolver to supply configuration values as defaults for subcommand flags.
	configResolver := kong.ResolverFunc(func(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		if field, ok := configFields[flag.Name]; ok {
			fv := field.value
			if !explicitlySet[flag.Name] && flag.HasDefault {
				return nil, nil
			}
			if fv.IsZero() {
				return nil, nil
			}
			if fv.Type() == reflect.TypeFor[Duration]() {
				return fv.Interface().(Duration).String(), nil
			}
			return fmt.Sprintf("%v", fv.Interface()), nil
		}
		return nil, nil
	})

	cli := &CLI{}
	ctx := kong.Parse(cli,
		kong.Name(appName),
		kong.Description(AppDescription),
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
		if field, ok := configFields[flag.Name]; ok {
			field.value.Set(flag.Target)
		}
	}

	ctx.Bind(runtimeCfg)
	ctx.Bind(ConfigPath(configFile))
	ctx.BindTo(context.Background(), (*context.Context)(nil))

	ctx.FatalIfErrorf(ctx.Run())
}
