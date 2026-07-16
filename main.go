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
	// String & Reflection Helpers
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

	var transformStructType func(t reflect.Type) reflect.Type
	transformStructType = func(t reflect.Type) reflect.Type {
		if t.Kind() == reflect.Pointer {
			return reflect.PointerTo(transformStructType(t.Elem()))
		}
		if t.Kind() != reflect.Struct {
			return t
		}
		if t.PkgPath() == "time" && (t.Name() == "Duration" || t.Name() == "Time") {
			return t
		}

		fields := make([]reflect.StructField, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			sf.Type = transformStructType(sf.Type)

			baseType := sf.Type
			if baseType.Kind() == reflect.Pointer {
				baseType = baseType.Elem()
			}

			if baseType.Kind() == reflect.Struct && (baseType.PkgPath() != "time" || (baseType.Name() != "Duration" && baseType.Name() != "Time")) {
				tagStr := string(sf.Tag)
				if !strings.Contains(tagStr, "embed") {
					prefix := kebabCase(sf.Name)
					if jsonTag := sf.Tag.Get("json"); jsonTag != "" {
						parts := strings.Split(jsonTag, ",")
						if parts[0] != "" && parts[0] != "-" {
							prefix = parts[0]
						}
					}
					sf.Tag = reflect.StructTag(fmt.Sprintf(`%s embed:"" prefix:"%s-"`, tagStr, prefix))
				}
			}
			fields[i] = sf
		}
		return reflect.StructOf(fields)
	}

	var recursivelyCopy func(src, dst reflect.Value)
	recursivelyCopy = func(src, dst reflect.Value) {
		if src.Kind() == reflect.Pointer {
			if src.IsNil() {
				return
			}
			if dst.IsNil() {
				dst.Set(reflect.New(dst.Type().Elem()))
			}
			recursivelyCopy(src.Elem(), dst.Elem())
			return
		}
		if src.Kind() != reflect.Struct {
			dst.Set(src)
			return
		}
		for i := 0; i < src.NumField(); i++ {
			recursivelyCopy(src.Field(i), dst.Field(i))
		}
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

	var flattenMap func(raw map[string]any, prefix string, out map[string]any)
	flattenMap = func(raw map[string]any, prefix string, out map[string]any) {
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

	buildFlatCache := func(configFile string, cfg any) map[string]any {
		flat := make(map[string]any)
		data, err := os.ReadFile(filepath.Clean(configFile))
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

	// CLI Orchestrator
	ExecuteCLI := func(cli any, cfg any, description string) {
		appName := resolveAppName()
		configFile := resolveConfigFile(appName)

		applyStructDefaults(cfg)

		flatCache := buildFlatCache(configFile, cfg)

		jsonResolver := kong.ResolverFunc(func(_ *kong.Context, _ *kong.Path, flag *kong.Flag) (any, error) {
			for _, env := range flag.Envs {
				if _, ok := os.LookupEnv(env); ok {
					return nil, nil
				}
			}
			if val, ok := flatCache[flag.Name]; ok {
				return val, nil
			}
			return nil, nil
		})

		type1 := reflect.TypeOf(cli).Elem()
		type2 := reflect.TypeOf(cfg).Elem()
		transformedConfigType := transformStructType(type2)

		combinedType := reflect.StructOf([]reflect.StructField{
			{Name: "CLI", Type: type1, Anonymous: true},
			{Name: "Config", Type: transformedConfigType, Anonymous: true},
		})
		combinedVal := reflect.New(combinedType)
		combinedVal.Elem().Field(0).Set(reflect.ValueOf(cli).Elem())
		recursivelyCopy(reflect.ValueOf(cfg).Elem(), combinedVal.Elem().Field(1))

		ctx := kong.Parse(combinedVal.Interface(),
			kong.Name(appName),
			kong.Description(description),
			kong.UsageOnError(),
			kong.ConfigureHelp(kong.HelpOptions{
				Compact: true,
				Tree:    true,
			}),
			kong.Configuration(kong.JSON),
			kong.DefaultEnvars(strings.ToUpper(appName)),
			kong.Resolvers(jsonResolver),
		)

		reflect.ValueOf(cli).Elem().Set(combinedVal.Elem().Field(0))
		recursivelyCopy(combinedVal.Elem().Field(1), reflect.ValueOf(cfg).Elem())

		ctx.Bind(cfg)
		ctx.Bind(ConfigPath(configFile))
		ctx.BindTo(context.Background(), (*context.Context)(nil))

		ctx.FatalIfErrorf(ctx.Run())
	}

	ExecuteCLI(&CLI{}, &Config{}, AppDescription)
}
