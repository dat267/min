package cmd

import (
	"context"

	"github.com/dat267/min/internal/devgen"
)

type DevCmdGroup struct {
	Har2Openapi Har2OpenapiCmd `cmd:"" help:"Convert HAR capture file to OpenAPI 3.0 specification"`
	Openapi2Go  Openapi2GoCmd  `cmd:"" help:"Generate Go SDK client from OpenAPI specification"`
}

type Har2OpenapiCmd struct {
	Input   string `help:"Path to HAR file" arg:""`
	Output  string `help:"Output OpenAPI file path (json or yaml)" short:"o" default:""`
	Title   string `help:"API title" default:"API Specification"`
	Host    string `help:"Filter requests by host (e.g. api.example.com)" default:""`
	Filter  string `help:"Path prefix filter (e.g. /v1/ or /api/)" default:""`
	ApiOnly bool   `help:"Ignore static assets (js, css, images, fonts, html)" default:"true"`
}

func (c *Har2OpenapiCmd) Run(ctx context.Context) error {
	return devgen.HarToOpenAPI(c.Input, c.Output, c.Title, c.Host, c.Filter, c.ApiOnly)
}

type Openapi2GoCmd struct {
	Input  string `help:"Path to OpenAPI spec file (JSON or YAML)" arg:""`
	Output string `help:"Output Go SDK package directory or file path" short:"o" default:"client.go"`
	Pkg    string `help:"Go package name (defaults to parent directory name of output file)" default:""`
	Client string `help:"Go SDK Client struct name" default:"Client"`
}

func (c *Openapi2GoCmd) Run(ctx context.Context) error {
	return devgen.OpenAPIToGo(c.Input, c.Output, c.Pkg, c.Client)
}
