package cmd

import "fmt"

type EdgeCmd struct {
	Name  string        `help:"Your name" short:"n" default:"World"`
	Count int           `help:"Repeat count" short:"c" default:"1"`
	Yes   bool          `help:"Boolean flag" short:"b"`
	Wait  string        `help:"Timeout value" default:"10s"`
	Tags  []string      `help:"Tags (repeatable)" short:"t"`
	First string        `help:"First positional" arg:""`
	Rest  []string      `help:"Remaining files" arg:""`
	Token string        `help:"API token" required:""`
}

func (e *EdgeCmd) Run(cmd *Cmd) error {
	fmt.Printf("Name:     %q\n", e.Name)
	fmt.Printf("Count:    %d\n", e.Count)
	fmt.Printf("Yes:      %v\n", e.Yes)
	fmt.Printf("Wait:     %q\n", e.Wait)
	fmt.Printf("Tags:     %v\n", e.Tags)
	fmt.Printf("First:    %q\n", e.First)
	fmt.Printf("Rest:     %v\n", e.Rest)
	fmt.Printf("Token:    %q\n", e.Token)
	fmt.Printf("---\n")
	fmt.Printf("AdminToken: %q\n", cmd.AdminToken)
	fmt.Printf("CoreTimeout: %q\n", cmd.CoreTimeout)
	fmt.Printf("CoreRetries: %d\n", cmd.CoreRetries)
	fmt.Printf("Debug:      %v\n", cmd.Debug)
	fmt.Printf("DryRun:     %v\n", cmd.DryRun)
	return nil
}
