package cmd

import (
	"fmt"
	"strings"
)

type GreetCmd struct {
	Name  string `help:"Name of the person to greet" default:"World" arg:""`
	Shout bool   `help:"Convert the greeting to uppercase" short:"s"`
	Times int    `help:"Number of times to repeat" default:"1" short:"t"`
}

func (g *GreetCmd) Run(cmd *Cmd) error {
	msg := fmt.Sprintf("Hello, %s! (Current core timeout setting is %s)", g.Name, cmd.CoreTimeout)
	if g.Shout {
		msg = strings.ToUpper(msg)
	}
	for i := 0; i < g.Times; i++ {
		fmt.Println(msg)
	}
	return nil
}
