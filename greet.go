package main

import (
	"fmt"
	"strings"
)

type GreetCmd struct {
	Name        string   `arg:"" help:"Name of the person to greet." default:"World"`
	Shout       bool     `short:"s" help:"Convert the greeting to uppercase."`
	Times       int      `short:"t" help:"Number of times to repeat the greeting." default:"1"`
	CoreTimeout Duration `help:"Core timeout override"`
}

func (cmd *GreetCmd) Run() error {
	msg := fmt.Sprintf("Hello, %s! (Current core timeout setting is %s)", cmd.Name, cmd.CoreTimeout)
	if cmd.Shout {
		msg = strings.ToUpper(msg)
	}

	for i := 0; i < cmd.Times; i++ {
		fmt.Println(msg)
	}

	return nil
}
