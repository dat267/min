package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ConfigCmdGroup struct {
	Init  ConfigInitCmd  `cmd:"" help:"Generate a default configuration file"`
	Path  ConfigPathCmd  `cmd:"" help:"Show configuration file path"`
	Show  ConfigShowCmd  `cmd:"" help:"Print current configuration values"`
	Set   ConfigSetCmd   `cmd:"" help:"Set a config value"`
	Unset ConfigUnsetCmd `cmd:"" help:"Unset a config value"`
}

type ConfigInitCmd struct {
	Overwrite bool `help:"Overwrite existing configuration file"`
}

func (cmd *ConfigInitCmd) Run() error {
	p := CfgPath()
	if _, err := os.Stat(p); err == nil && !cmd.Overwrite {
		return fmt.Errorf("configuration file already exists at %s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
	}
	data, err := json.MarshalIndent(map[string]any{}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}
	fmt.Printf("Configuration file created at %s\n", p)
	return nil
}

type ConfigPathCmd struct{}

func (cmd *ConfigPathCmd) Run() error {
	p := CfgPath()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		fmt.Printf("%s (does not exist)\n", p)
		return nil
	}
	fmt.Println(p)
	return nil
}

type ConfigShowCmd struct{}

func (cmd *ConfigShowCmd) Run() error {
	p := CfgPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("%s (does not exist)\n", p)
			return nil
		}
		return fmt.Errorf("failed to read configuration file: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

type ConfigSetCmd struct {
	Key   string `arg:"" help:"Configuration key (dot-notation for nested, e.g. core.timeout)"`
	Value string `arg:"" help:"Value to set"`
}

func (cmd *ConfigSetCmd) Run() error {
	p := CfgPath()
	cfgMap, err := loadConfigMap(p)
	if err != nil {
		return err
	}

	var val any = cmd.Value
	if cmd.Value == "true" {
		val = true
	} else if cmd.Value == "false" {
		val = false
	} else if n, err := strconv.Atoi(cmd.Value); err == nil {
		val = n
	}

	keys := strings.Split(cmd.Key, ".")
	setNestedMap(cfgMap, keys, val)

	if err := saveConfigMap(p, cfgMap); err != nil {
		return err
	}
	fmt.Printf("Set %q = %v\n", cmd.Key, val)
	return nil
}

type ConfigUnsetCmd struct {
	Key string `arg:"" help:"Configuration key to unset"`
}

func (cmd *ConfigUnsetCmd) Run() error {
	p := CfgPath()
	cfgMap, err := loadConfigMap(p)
	if err != nil {
		return err
	}

	keys := strings.Split(cmd.Key, ".")
	if unsetNestedMap(cfgMap, keys) {
		if err := saveConfigMap(p, cfgMap); err != nil {
			return err
		}
		fmt.Printf("Unset %q\n", cmd.Key)
	} else {
		fmt.Printf("Key %q not found\n", cmd.Key)
	}
	return nil
}

func loadConfigMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}
	if m == nil {
		m = make(map[string]any)
	}
	return m, nil
}

func saveConfigMap(path string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func setNestedMap(m map[string]any, keys []string, val any) {
	if len(keys) == 0 {
		return
	}
	if len(keys) == 1 {
		m[keys[0]] = val
		return
	}
	sub, ok := m[keys[0]].(map[string]any)
	if !ok {
		sub = make(map[string]any)
		m[keys[0]] = sub
	}
	setNestedMap(sub, keys[1:], val)
}

func unsetNestedMap(m map[string]any, keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	if len(keys) == 1 {
		if _, ok := m[keys[0]]; ok {
			delete(m, keys[0])
			return true
		}
		return false
	}
	sub, ok := m[keys[0]].(map[string]any)
	if !ok {
		return false
	}
	return unsetNestedMap(sub, keys[1:])
}
