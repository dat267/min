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
	Yes         bool            `help:"Skip interactive prompts for required parameters" short:"y"`
	ConfigFile  string          `help:"Path to config file" placeholder:"PATH"`
	AdminToken  string          `help:"Admin token" json:"admin-token"`
	CoreTimeout string          `help:"Core timeout" default:"10s" json:"core-timeout"`
	CoreRetries int             `help:"Core retries" default:"3" json:"core-retries"`
	Debug       bool            `help:"Enable debug mode" json:"debug"`
	DryRun      bool            `help:"Enable dry run mode" json:"dry-run"`
	Config      ConfigCmdGroup  `help:"Manage application configuration" cmd:""`
	Greet       GreetCmd        `help:"Print a personalized greeting message" cmd:""`
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
			case reflect.Int, reflect.Int64:
				m[j] = 0
			}
		} else {
			switch ft.Type.Kind() {
			case reflect.Int, reflect.Int64:
				n := int64(0)
				fmt.Sscanf(d, "%d", &n)
				m[j] = n
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

	appCmd := &Cmd{}
	a := cli.New(appCmd,
		cli.WithName(appName),
		cli.WithDesc(AppDescription),
		cli.WithCfg(configPath),
		cli.WithEnv(strings.ToUpper(appName)+"_"),
		cli.WithPrompt(),
	)
	a.Bind(ConfigPath(configPath))
	a.Bind(appCmd)

	if err := a.Parse(os.Args[1:]); err != nil {
		if err.Error() != "" {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
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
	for i, arg := range os.Args {
		if arg == "--config-file" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if after, found := strings.CutPrefix(arg, "--config-file="); found {
			return after
		}
	}
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
