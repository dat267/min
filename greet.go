package main

import "fmt"

type GreetCmd struct {
	Name string `arg:"" help:"Name of the person to greet" default:"World"`
}

func (cmd *GreetCmd) Run(cfg *Config) error {
	fmt.Printf("Hello, %s! (Current core timeout setting is %s)\n", cmd.Name, cfg.Core.Timeout)
	fmt.Printf("admin token %s", cfg.AdminToken)
	return nil
}
