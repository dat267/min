package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dat267/min/internal/scaffold"
)

type CmdGroup struct {
	Init CmdInitCmd `cmd:"" help:"Initialize a new project"`
	Add  CmdAddCmd  `cmd:"" help:"Add a new command"`
	Show CmdShowCmd `cmd:"" help:"List all commands"`
	Edit CmdEditCmd `cmd:"" help:"Edit a command struct"`
}

type CmdInitCmd struct {
	Name   string `help:"Project name (or '.' for current directory)" arg:""`
	Module string `help:"Go module path" default:""`
}

func (c *CmdInitCmd) Run() error {
	name := c.Name
	if name == "" {
		return fmt.Errorf("project name is required")
	}

	var dir string
	if name == "." {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
		name = filepath.Base(wd)
		dir = "."
	} else {
		dir = name
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("directory %q already exists", dir)
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}

	module := c.Module
	if module == "" {
		module = name
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainTmpl(module)), 0644); err != nil {
		return fmt.Errorf("write main.go: %w", err)
	}

	_ = os.MkdirAll(filepath.Join(dir, "cmd"), 0755)
	if err := os.WriteFile(filepath.Join(dir, "cmd", "cmd.go"), []byte(cmdTmpl(name)), 0644); err != nil {
		return fmt.Errorf("write cmd/cmd.go: %w", err)
	}

	modCmd := exec.Command("go", "mod", "init", module)
	modCmd.Dir = dir
	if out, err := modCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod init: %w\n%s", err, out)
	}

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = dir
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("resolve dependencies: %w\n%s", err, out)
	}

	fmt.Printf("Created project %q in %s/\n", name, dir)
	fmt.Printf("  cd %s && go run .\n", dir)
	return nil
}

func mainTmpl(module string) string {
	return fmt.Sprintf(`package main

import (
	"context"

	"%s/cmd"
)

func main() {
	cmd.Execute(context.Background())
}
`, module)
}

func cmdTmpl(name string) string {
	return fmt.Sprintf(`package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
)

var cfgPath string

func init() {
	cfgPath = resolveConfigPath()
}

func resolveConfigPath() string {
	appName := "%s"
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

type CLI struct {
	ConfigFile string    `+"`"+`help:"Config file path" json:"-"`+"`"+`
	Greet      GreetCmd  `+"`"+`cmd:"" help:"Print a greeting"`+"`"+`
	Config     ConfigCmd `+"`"+`cmd:"" help:"Manage configuration"`+"`"+`
}

func SetConfigPath(p string) {
	if p == "" {
		cfgPath = resolveConfigPath()
	} else {
		cfgPath = p
	}
}

func CfgPath() string { return cfgPath }

func Execute(ctx context.Context) {
	app := &CLI{}

	if cf := resolveConfigFileFlag(); cf != "" {
		SetConfigPath(cf)
	}
	activeConfig := CfgPath()

	options := []kong.Option{
		kong.Name("%[1]s"),
		kong.Description("A CLI application"),
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
		fmt.Fprintf(os.Stderr, "error: %%v\n", err)
		os.Exit(1)
	}

	kongCtx, err := k.Parse(os.Args[1:])
	k.FatalIfErrorf(err)

	SetConfigPath(app.ConfigFile)
	k.FatalIfErrorf(kongCtx.Run())
}

func resolveConfigFileFlag() string {
	for i, arg := range os.Args {
		if arg == "--config-file" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "--config-file=") {
			parts := strings.SplitN(arg, "=", 2)
			return parts[1]
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


type GreetCmd struct {
	Name  string `+"`"+`help:"Name to greet" arg:"" default:"World"`+"`"+`
	Count int    `+"`"+`help:"Repeat count" default:"1"`+"`"+`
	Shout bool   `+"`"+`help:"Shout" short:"s"`+"`"+`
}

func (c *GreetCmd) Run() error {
	for i := 0; i < c.Count; i++ {
		msg := fmt.Sprintf("Hello, %%s!", c.Name)
		if c.Shout {
			msg = strings.ToUpper(msg)
		}
		fmt.Println(msg)
	}
	return nil
}

type ConfigCmd struct {
	Init  ConfigInitCmd  `+"`"+`cmd:"" help:"Generate a default configuration file"`+"`"+`
	Path  ConfigPathCmd  `+"`"+`cmd:"" help:"Show configuration file path"`+"`"+`
	Show  ConfigShowCmd  `+"`"+`cmd:"" help:"Print current configuration values"`+"`"+`
	Set   ConfigSetCmd   `+"`"+`cmd:"" help:"Set configuration key"`+"`"+`
	Unset ConfigUnsetCmd `+"`"+`cmd:"" help:"Unset configuration key"`+"`"+`
	Edit  ConfigEditCmd  `+"`"+`cmd:"" help:"Edit configuration file"`+"`"+`
}

type ConfigInitCmd struct {
	Overwrite bool `+"`"+`help:"Overwrite existing configuration file"`+"`"+`
}

func (c *ConfigInitCmd) Run() error {
	p := CfgPath()
	if _, err := os.Stat(p); err == nil && !c.Overwrite {
		return fmt.Errorf("configuration file already exists at %%s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("failed to create configuration directory: %%w", err)
	}
	data, err := json.MarshalIndent(map[string]any{}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %%w", err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %%w", err)
	}
	fmt.Printf("Configuration file created at %%s\n", p)
	return nil
}

type ConfigPathCmd struct{}

func (c *ConfigPathCmd) Run() error {
	p := CfgPath()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		fmt.Printf("%%s (does not exist)\n", p)
		return nil
	}
	fmt.Println(p)
	return nil
}

type ConfigShowCmd struct{}

func (c *ConfigShowCmd) Run() error {
	p := CfgPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("%%s (does not exist)\n", p)
			return nil
		}
		return fmt.Errorf("failed to read configuration file: %%w", err)
	}
	fmt.Println(string(data))
	return nil
}

type ConfigSetCmd struct {
	Key   string `+"`"+`help:"Configuration key" arg:""`+"`"+`
	Value string `+"`"+`help:"Configuration value" arg:""`+"`"+`
}

func (c *ConfigSetCmd) Run() error {
	p := CfgPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %%w", err)
	}
	var raw map[string]any
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &raw)
	}
	if raw == nil {
		raw = make(map[string]any)
	}

	parts := strings.Split(c.Key, ".")
	curr := raw
	for i, part := range parts {
		if i == len(parts)-1 {
			curr[part] = c.Value
			break
		}
		next, ok := curr[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			curr[part] = next
		}
		curr = next
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %%w", err)
	}
	return os.WriteFile(p, data, 0644)
}

type ConfigUnsetCmd struct {
	Key string `+"`"+`help:"Configuration key" arg:""`+"`"+`
}

func (c *ConfigUnsetCmd) Run() error {
	p := CfgPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read configuration file: %%w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %%w", err)
	}

	parts := strings.Split(c.Key, ".")
	var unsetFunc func(m map[string]any, keys []string) bool
	unsetFunc = func(m map[string]any, keys []string) bool {
		if len(keys) == 0 {
			return false
		}
		if len(keys) == 1 {
			delete(m, keys[0])
			return len(m) == 0
		}
		if sub, ok := m[keys[0]].(map[string]any); ok {
			if unsetFunc(sub, keys[1:]) {
				delete(m, keys[0])
			}
		}
		return len(m) == 0
	}
	unsetFunc(raw, parts)

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %%w", err)
	}
	return os.WriteFile(p, out, 0644)
}

type ConfigEditCmd struct{}

func (c *ConfigEditCmd) Run() error {
	p := CfgPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %%w", err)
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		if err := os.WriteFile(p, []byte("{}\n"), 0644); err != nil {
			return fmt.Errorf("failed to write default config: %%w", err)
		}
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	ecmd := exec.Command(editor, p)
	ecmd.Stdin = os.Stdin
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr
	return ecmd.Run()
}
`, name)
}

type CmdAddCmd struct {
	Name string `help:"Command name (use dots for nesting, e.g. admin.users.create)" arg:""`
	Desc string `help:"Description for the command"`
}

func (c *CmdAddCmd) Run() error {
	return scaffold.AddCommand(c.Name, c.Desc)
}

type CmdShowCmd struct{}

func (c *CmdShowCmd) Run() error {
	return scaffold.ShowCommands()
}

type CmdEditCmd struct {
	Name string `help:"Command name" arg:""`
}

func (c *CmdEditCmd) Run() error {
	return scaffold.EditCommand(c.Name)
}
