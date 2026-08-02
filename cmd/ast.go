package cmd

import (
	"github.com/dat267/min/internal/astscan"
)

// AstCmdGroup encapsulates all AST-related subcommands.
type AstCmdGroup struct {
	Scan  AstScanCmd  `cmd:"" help:"Scan Go files, strip bodies, keep signatures"`
	Types AstTypesCmd `cmd:"" help:"Show types only (structs, interfaces, func signatures)"`
	Fn    AstFnCmd    `cmd:"" help:"Extract context for a single function"`
}

// AstScanCmd scans Go files and prints top-level declarations (stripping function bodies).
type AstScanCmd struct {
	Path string `help:"File or directory to scan" arg:"" default:"."`
}

func (c *AstScanCmd) Run() error {
	return astscan.Scan(c.Path)
}

// AstTypesCmd scans Go files and prints only type declarations.
type AstTypesCmd struct {
	Path string `help:"File or directory to scan" arg:"" default:"."`
}

func (c *AstTypesCmd) Run() error {
	return astscan.Types(c.Path)
}

// AstFnCmd extracts the context, references, and call sites for a specific function/method.
type AstFnCmd struct {
	Func string `arg:"" help:"Name of the function to search for (e.g. 'MyFunc' or 'MyStruct.MyMethod')."`
	Path string `arg:"" help:"Path to file or directory." default:"."`
	Type string `name:"type" help:"Optional receiver type name filter."`
}

func (c *AstFnCmd) Run() error {
	return astscan.Fn(c.Func, c.Path, c.Type)
}
