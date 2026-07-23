// Package cmd implements the min CLI commands: greet, config init/show/path/edit.
// Execute() is the entry point called from main().
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"min/cli"
)

const AppDescription = "Internal workflows and troubleshooting utility"

type ConfigPath string

type Cmd struct {
	ConfigPath  ConfigPath      `json:"-"`
	AdminToken  string          `help:"Admin token" json:"admin-token"`
	CoreTimeout string          `help:"Core timeout" default:"10s" json:"core-timeout"`
	CoreRetries int             `help:"Core retries" default:"3" json:"core-retries"`
	Debug       bool            `help:"Enable debug mode" json:"debug"`
	DryRun      bool            `help:"Enable dry run mode" json:"dry-run"`
	Config      ConfigCmdGroup  `cmd:"" help:"Manage application configuration"`
	Greet       GreetCmd        `cmd:"" help:"Print a personalized greeting message"`
	Edge        EdgeCmd         `cmd:"" help:"Showcase all flag types and edge cases"`
}

func (c *Cmd) ConfigFields() map[string]any {
	m := map[string]any{}
	v := reflect.ValueOf(c).Elem()
	t := v.Type()
	for i, n := 0, t.NumField(); i < n; i++ {
		ft := t.Field(i)
		if j := ft.Tag.Get("json"); j != "" && j != "-" {
			m[j] = v.Field(i).Interface()
		}
	}
	return m
}

func (Cmd) ConfigDefaults() map[string]any {
	m := map[string]any{}
	t := reflect.TypeFor[Cmd]()
	for i, n := 0, t.NumField(); i < n; i++ {
		ft := t.Field(i)
		j := ft.Tag.Get("json")
		if j == "" || j == "-" {
			continue
		}
		d := ft.Tag.Get("default")
		if d == "" {
			switch ft.Type.Kind() {
			case reflect.String:
				m[j] = ""
			case reflect.Bool:
				m[j] = false
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				m[j] = int64(0)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				m[j] = uint64(0)
			case reflect.Float32, reflect.Float64:
				m[j] = float64(0)
			}
		} else {
			switch ft.Type.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				n := int64(0)
				if _, err := fmt.Sscanf(d, "%d", &n); err == nil {
					m[j] = n
				} else {
					m[j] = d
				}
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				n := uint64(0)
				if _, err := fmt.Sscanf(d, "%d", &n); err == nil {
					m[j] = n
				} else {
					m[j] = d
				}
			case reflect.Float32, reflect.Float64:
				f := float64(0)
				if _, err := fmt.Sscanf(d, "%f", &f); err == nil {
					m[j] = f
				} else {
					m[j] = d
				}
			default:
				m[j] = d
			}
		}
	}
	return m
}

func Execute() {
	appName := resolveAppName()
	configPath := resolveConfigPath(appName)

	appCmd := &Cmd{ConfigPath: ConfigPath(configPath)}
	a := cli.New(appCmd,
		cli.WithName(appName),
		cli.WithDesc(AppDescription),
		cli.WithCfg(configPath),
		cli.WithEnv(strings.ToUpper(appName)+"_"),
		cli.WithPrompt(),
		cli.WithConfigField("ConfigPath"),
	)
	a.Bind(ConfigPath(configPath))
	a.Bind(appCmd)

	if err := a.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func resolveAppName() string {
	name := filepath.Base(os.Args[0])
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" || name == "main" || name == "app" ||
		strings.HasPrefix(name, "go-build") || strings.HasSuffix(name, ".test") {
		return "min"
	}
	return name
}

func resolveConfigPath(appName string) string {
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
