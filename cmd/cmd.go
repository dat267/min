package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"min/cli"
)

const AppDescription = "Internal workflows and troubleshooting utility"

type ConfigPath string

type GreetCmd struct {
	Name  string `cli:"help=Name of the person to greet,default=World,arg"`
	Shout bool   `cli:"help=Convert the greeting to uppercase,short=s"`
	Times int    `cli:"help=Number of times to repeat,default=1,short=t"`
}

func (g *GreetCmd) Run(cli *CLI) error {
	msg := fmt.Sprintf("Hello, %s! (Current core timeout setting is %s)", g.Name, cli.CoreTimeout)
	if g.Shout {
		msg = strings.ToUpper(msg)
	}
	for i := 0; i < g.Times; i++ {
		fmt.Println(msg)
	}
	return nil
}

type ConfigCmdGroup struct {
	Init ConfigInitCmd `cli:"help=Generate a default configuration profile template file,cmd"`
	Path ConfigPathCmd `cli:"help=Show the active configuration file path,cmd"`
	Show ConfigShowCmd `cli:"help=Print the active configuration values,cmd"`
	Edit ConfigEditCmd `cli:"help=Open the active configuration file in an editor,cmd"`
}

type ConfigInitCmd struct {
	Force bool `cli:"help=Overwrite existing configuration file,short=f"`
}

type ConfigPathCmd struct{}
type ConfigShowCmd struct{}
type ConfigEditCmd struct{}

func (c *ConfigInitCmd) Run(path ConfigPath, cli *CLI) error {
	p := string(path)
	if _, err := os.Stat(p); err == nil && !c.Force {
		return fmt.Errorf("configuration file already exists at %s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
	}
	cfg := map[string]any{
		"admin-token":  cli.AdminToken,
		"core-timeout": cli.CoreTimeout,
		"core-retries": cli.CoreRetries,
		"debug":        cli.Debug,
		"dry-run":      cli.DryRun,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}
	fmt.Printf("Successfully generated base configuration file at: %s\n", p)
	return nil
}

func (c *ConfigPathCmd) Run(path ConfigPath) error {
	p := string(path)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		fmt.Printf("%s (does not exist)\n", p)
		return nil
	}
	fmt.Println(p)
	return nil
}

func (c *ConfigShowCmd) Run(cli *CLI) error {
	cfg := map[string]any{
		"admin-token":  cli.AdminToken,
		"core-timeout": cli.CoreTimeout,
		"core-retries": cli.CoreRetries,
		"debug":        cli.Debug,
		"dry-run":      cli.DryRun,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func (c *ConfigEditCmd) Run(path ConfigPath) error {
	p := string(path)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		cfg := map[string]any{
			"admin-token":  "",
			"core-timeout": "2m",
			"core-retries": 3,
			"debug":        false,
			"dry-run":      false,
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err == nil {
			_ = os.WriteFile(p, data, 0o600)
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

type CLI struct {
	Yes         bool            `cli:"help=Skip interactive prompts for required parameters,short=y"`
	ConfigFile  string          `cli:"help=Path to config file,placeholder=PATH"`
	AdminToken  string          `cli:"help=Admin token"`
	CoreTimeout string          `cli:"help=Core timeout,default=10s"`
	CoreRetries int             `cli:"help=Core retries,default=3"`
	Debug       bool            `cli:"help=Enable debug mode"`
	DryRun      bool            `cli:"help=Enable dry run mode"`
	Config      ConfigCmdGroup  `cli:"help=Manage application configuration,cmd"`
	Greet       GreetCmd        `cli:"help=Print a personalized greeting message,cmd"`
}

func Run() {
	appName := resolveAppName()
	configPath := resolveConfigPath(appName)

	appCli := &CLI{}
	a := cli.New(appCli,
		cli.WithName(appName),
		cli.WithDesc(AppDescription),
		cli.WithCfg(configPath),
		cli.WithEnv(strings.ToUpper(appName)+"_"),
		cli.WithPrompt(),
	)
	a.Bind(ConfigPath(configPath))
	a.Bind(appCli)

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
