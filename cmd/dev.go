package cmd

type DevCmdGroup struct {
	Har2Openapi Har2OpenapiCmd `cmd:"" help:"Convert HAR capture file to OpenAPI 3.0 specification"`
	Openapi2Go  Openapi2GoCmd  `cmd:"" help:"Generate Go SDK client from OpenAPI specification"`
}
