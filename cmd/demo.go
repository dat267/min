package cmd

import "fmt"

type DemoCmd struct {
	Name  string `help:"Your name" required:""`
	Token string `help:"API token" required:""`
}

func (d *DemoCmd) Run() error {
	fmt.Printf("Hello %s! Your token is %s\n", d.Name, d.Token)
	return nil
}
