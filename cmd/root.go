package cmd

import (
	"fmt"
	"runtime/debug"
)

// CLI is the root CLI struct containing all subcommand groups.
type CLI struct {
	Dev        DevCmdGroup     `cmd:"" help:"Developer utilities (cURL, HAR, OpenAPI generator)"`
	ConfigFile string          `help:"Config file path" json:"-"`
	Version    VersionCmd      `cmd:"" help:"Show version"`
	Ast        AstCmdGroup     `cmd:"" help:"Parse Go AST for AI context"`
	Cmd        CmdGroup        `cmd:"" help:"Manage commands"`
	Config     ConfigCmdGroup  `cmd:"" help:"Manage application configuration"`
}

var Version = "dev"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
}

type VersionCmd struct{}

func (c *VersionCmd) Run() error {
	fmt.Println(Version)
	return nil
}
