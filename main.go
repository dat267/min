package main

import (
	"time"
)

const (
	DefaultAppName = "min"
	AppDescription = "Internal workflows and troubleshooting utility"
)

type ConfigPath string

type Config struct {
	AdminToken string     `json:"admin-token"`
	Core       CoreConfig `json:"core"`
	Debug      bool       `json:"debug"`
	DryRun     bool       `json:"dry-run"`
}

type CoreConfig struct {
	Timeout time.Duration `json:"timeout" default:"2m"`
	Retries int           `json:"retries" default:"3"`
}

type CLI struct {
	ConfigFile string `help:"Path to config file." placeholder:"PATH"`

	Config ConfigCmdGroup `cmd:"" help:"Manage application configuration"`
	Greet  GreetCmd       `cmd:"" help:"Print a personalized greeting message"`
}

func main() {
	RunCLI(&CLI{}, &Config{}, DefaultAppName, AppDescription)
}
