package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
)

type ConfigCmdGroup struct {
	Init ConfigInitCmd `cmd:"" help:"Generate a default configuration profile template file"`
	Path ConfigPathCmd `cmd:"" help:"Show the active configuration file path"`
	Show ConfigShowCmd `cmd:"" help:"Print the active configuration values"`
	Edit ConfigEditCmd `cmd:"" help:"Open the active configuration file in an editor"`
}

type ConfigInitCmd struct {
	Force bool `short:"f" help:"Overwrite existing configuration file"`
}

type ConfigPathCmd struct{}

type ConfigShowCmd struct{}

type ConfigEditCmd struct{}

func (cmd *ConfigInitCmd) Run(path ConfigPath) error {
	p := string(path)
	if _, err := os.Stat(p); err == nil && !cmd.Force {
		return fmt.Errorf("configuration file already exists at %s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
	}

	cfg := newDefaultConfig()
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

func (cmd *ConfigEditCmd) Run(path ConfigPath) error {
	p := string(path)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
	}

	if _, err := os.Stat(p); os.IsNotExist(err) {
		cfg := newDefaultConfig()
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err == nil {
			_ = os.WriteFile(p, data, 0o600)
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	c := exec.Command(editor, p)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func newDefaultConfig() *Config {
	cfg := &Config{}
	applyDefaults(cfg)
	return cfg
}

func applyDefaults(s any) {
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
			applyDefaults(fv.Addr().Interface())
			continue
		}
		if defaultVal, ok := ft.Tag.Lookup("default"); ok && fv.IsZero() {
			switch fv.Kind() {
			case reflect.String:
				fv.SetString(defaultVal)
			case reflect.Bool:
				fv.SetBool(defaultVal == "true" || defaultVal == "1")
			case reflect.Int, reflect.Int64:
				if n, err := strconv.ParseInt(defaultVal, 10, 64); err == nil {
					fv.SetInt(n)
				}
			}
		}
	}
}
