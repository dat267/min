package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type ConfigCmdGroup struct {
	Init ConfigInitCmd `help:"Generate a default configuration profile template file" cmd:""`
	Path ConfigPathCmd `help:"Show the active configuration file path" cmd:""`
	Show ConfigShowCmd `help:"Print the active configuration values" cmd:""`
	Edit ConfigEditCmd `help:"Open the active configuration file in an editor" cmd:""`
}

type ConfigInitCmd struct {
	Force bool `help:"Overwrite existing configuration file" short:"f"`
}

type ConfigPathCmd struct{}
type ConfigShowCmd struct{}
type ConfigEditCmd struct{}

func (c *ConfigInitCmd) Run(path ConfigPath, cmd *Cmd) error {
	p := string(path)
	if _, err := os.Stat(p); err == nil && !c.Force {
		return fmt.Errorf("configuration file already exists at %s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
	}
	data, err := json.MarshalIndent(cmd.ConfigFields(), "", "  ")
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

func (c *ConfigShowCmd) Run(cmd *Cmd) error {
	data, err := json.MarshalIndent(cmd.ConfigFields(), "", "  ")
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
		data, err := json.MarshalIndent(Cmd{}.ConfigDefaults(), "", "  ")
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
