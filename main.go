package main

import (
	"context"
	"encoding/json"
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
	Debug      bool       `json:"debug"`
	DryRun     bool       `json:"dry-run"`
}

type CoreConfig struct {
	Timeout time.Duration `json:"timeout" default:"2m"`
	Retries int           `json:"retries" default:"3"`
}

type CLI struct {
	ConfigFile string `help:"Path to config file." placeholder:"PATH"`

	Config ConfigCmdGroup `cmd:"" help:"Manage application configuration"`
	Greet  GreetCmd       `cmd:"" help:"Print a personalized greeting message"`
}

// main wires up the CLI and runs it.
func main() {
	// String Helper
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

	// Application Config Helpers
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

	var applyStructDefaults func(s any)
	applyStructDefaults = func(s any) {
		v := reflect.Indirect(reflect.ValueOf(s))
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
				if fv.Type() == reflect.TypeFor[time.Duration]() {
					if d, err := time.ParseDuration(defaultVal); err == nil {
						fv.SetInt(int64(d))
					}
				} else if n, err := strconv.ParseInt(defaultVal, 10, 64); err == nil {
					fv.SetInt(n)
				}
			}
		}
	}

	// loadConfig unmarshals a JSON config file into cfg, silently ignoring missing files.
	loadConfig := func(configFile string, cfg any) {
		data, err := os.ReadFile(filepath.Clean(configFile))
		if err != nil {
			return
		}
		_ = json.Unmarshal(data, cfg)
	}

	// applyEnvOverrides walks cfg and overrides any field whose env var (e.g. MIN_ADMIN_TOKEN) is set.
	// Precedence: env var > config file value already in cfg.
	var applyEnvOverrides func(s any, prefix string)
	applyEnvOverrides = func(s any, prefix string) {
		v := reflect.Indirect(reflect.ValueOf(s))
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
			// Derive env var key from json tag, falling back to kebab-case field name.
			name := kebabCase(ft.Name)
			if jsonTag := ft.Tag.Get("json"); jsonTag != "" {
				if parts := strings.SplitN(jsonTag, ",", 2); parts[0] != "" && parts[0] != "-" {
					name = parts[0]
				}
			}
			envKey := prefix + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
			if fv.Kind() == reflect.Struct {
				if fv.CanAddr() {
					applyEnvOverrides(fv.Addr().Interface(), envKey+"_")
				}
				continue
			}
			val, ok := os.LookupEnv(envKey)
			if !ok {
				continue
			}
			switch fv.Kind() {
			case reflect.String:
				fv.SetString(val)
			case reflect.Bool:
				fv.SetBool(val == "true" || val == "1")
			case reflect.Int:
				if n, err := strconv.Atoi(val); err == nil {
					fv.SetInt(int64(n))
				}
			case reflect.Int64:
				if fv.Type() == reflect.TypeFor[time.Duration]() {
					if d, err := time.ParseDuration(val); err == nil {
						fv.SetInt(int64(d))
					}
				} else if n, err := strconv.ParseInt(val, 10, 64); err == nil {
					fv.SetInt(n)
				}
			}
		}
	}

	cli := &CLI{}
	cfg := &Config{}

	appName := resolveAppName()
	configFile := resolveConfigFile(appName)

	// Resolve config in order: default → config file → env var
	applyStructDefaults(cfg)
	loadConfig(configFile, cfg)
	applyEnvOverrides(cfg, strings.ToUpper(appName)+"_")

	ctx := kong.Parse(cli,
		kong.Name(appName),
		kong.Description(AppDescription),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Tree:    true,
		}),
	)

	ctx.Bind(cfg)
	ctx.Bind(ConfigPath(configFile))
	ctx.BindTo(context.Background(), (*context.Context)(nil))

	ctx.FatalIfErrorf(ctx.Run())
}
