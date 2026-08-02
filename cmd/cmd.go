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

	if err := writeScaffoldProject(dir, name, module); err != nil {
		return fmt.Errorf("write project: %w", err)
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
