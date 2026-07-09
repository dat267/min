package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ConfigCmdGroup struct {
	Init ConfigInitCmd `cmd:"" help:"Generate a default configuration profile template file"`
	Path ConfigPathCmd `cmd:"" help:"Show the active configuration file path"`
	Show ConfigShowCmd `cmd:"" help:"Print the active configuration values"`
}

type ConfigInitCmd struct {
	Force bool `short:"f" help:"Overwrite existing configuration file"`
}

type ConfigPathCmd struct{}

type ConfigShowCmd struct{}

func (cmd *ConfigInitCmd) Run(cfg *Config, path ConfigPath) error {
	p := string(path)
	if _, err := os.Stat(p); err == nil && !cmd.Force {
		return fmt.Errorf("configuration file already exists at %s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
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

func (cmd *ConfigPathCmd) Run(path ConfigPath) error {
	p := string(path)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		fmt.Printf("%s (does not exist)\n", p)
		return nil
	}
	fmt.Println(p)
	return nil
}

func (cmd *ConfigShowCmd) Run(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
