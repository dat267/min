package cmd_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat267/min/cmd"
)

func TestOpenAPIGenCmd(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.yaml")
	outPath := filepath.Join(tmpDir, "client.go")

	specContent := `openapi: 3.0.3
info:
  title: Example API
  version: 1.0.0
paths:
  /v1/users:
    get:
      summary: Get users
      parameters:
        - name: limit
          in: query
    post:
      summary: Create user
      requestBody:
        content:
          application/json:
            schema:
              type: object
`

	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write openapi spec: %v", err)
	}

	genCmd := &cmd.OpenAPIGenCmd{
		Input:  specPath,
		Output: outPath,
		Pkg:    "client",
		Client: "Client",
	}

	if err := genCmd.Run(context.Background()); err != nil {
		t.Fatalf("OpenAPIGenCmd failed: %v", err)
	}

	codeBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read generated code: %v", err)
	}

	code := string(codeBytes)
	if !strings.Contains(code, "package client") {
		t.Errorf("missing package declaration in generated code")
	}
	if !strings.Contains(code, "type Client struct") {
		t.Errorf("missing Client struct in generated code")
	}
	if !strings.Contains(code, "GetV1Users") {
		t.Errorf("missing GetV1Users method in generated code")
	}
	if !strings.Contains(code, "PostV1Users") {
		t.Errorf("missing PostV1Users method in generated code")
	}
}

func TestOpenAPIGenCmd_ParamsAndBodyStructs(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.yaml")
	outPath := filepath.Join(tmpDir, "client.go")

	specContent := `openapi: 3.0.3
info:
  title: Full API Test
  version: 1.0.0
paths:
  /v1/orgs/{org_id}/users:
    get:
      summary: Get users with query filters
      parameters:
        - name: org_id
          in: path
          required: true
        - name: role
          in: query
        - name: limit
          in: query
    post:
      summary: Create user in org
      parameters:
        - name: org_id
          in: path
          required: true
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                username:
                  type: string
                age:
                  type: integer
`

	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write openapi spec: %v", err)
	}

	genCmd := &cmd.OpenAPIGenCmd{
		Input:  specPath,
		Output: outPath,
		Pkg:    "client",
		Client: "Client",
	}

	if err := genCmd.Run(context.Background()); err != nil {
		t.Fatalf("OpenAPIGenCmd failed: %v", err)
	}

	codeBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read generated code: %v", err)
	}

	code := string(codeBytes)
	
	// Check query parameter struct generation
	if !strings.Contains(code, "type GetV1OrgsOrgIdUsersParams struct") {
		t.Errorf("missing GetV1OrgsOrgIdUsersParams struct in generated code:\n%s", code)
	}
	if !strings.Contains(code, "Role string `json:\"role,omitempty\"` ") {
		t.Errorf("missing Role field in query params struct:\n%s", code)
	}

	// Check path parameter formatting
	if !strings.Contains(code, "func (c *Client) GetV1OrgsOrgIdUsers(ctx context.Context, orgId string, queryParams *GetV1OrgsOrgIdUsersParams)") {
		t.Errorf("missing GetV1OrgsOrgIdUsers method signature with path and query parameters:\n%s", code)
	}
	if !strings.Contains(code, "reqURL := c.BaseURL + fmt.Sprintf(\"/v1/orgs/%s/users\", orgId)") {
		t.Errorf("missing fmt.Sprintf path template formatting:\n%s", code)
	}

	// Check request body payload struct generation
	if !strings.Contains(code, "type PostV1OrgsOrgIdUsersRequest struct") {
		t.Errorf("missing PostV1OrgsOrgIdUsersRequest struct in generated code:\n%s", code)
	}
	if !strings.Contains(code, "Username string `json:\"username,omitempty\"` ") {
		t.Errorf("missing Username field in request struct:\n%s", code)
	}
}

func TestOpenAPIGenCmd_AutoPkgFromParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	customPkgDir := filepath.Join(tmpDir, "my_custom_sdk")
	outPath := filepath.Join(customPkgDir, "client.go")
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	specContent := `openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /v1/health:
    get:
      summary: Health check
`
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	// Omit Pkg so it defaults to parent directory name ("my_custom_sdk" -> "mycustomsdk")
	genCmd := &cmd.OpenAPIGenCmd{
		Input:  specPath,
		Output: outPath,
	}

	if err := genCmd.Run(context.Background()); err != nil {
		t.Fatalf("genCmd failed: %v", err)
	}

	codeBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read outPath failed: %v", err)
	}

	code := string(codeBytes)
	if !strings.Contains(code, "package my_custom_sdk") {
		t.Errorf("expected package my_custom_sdk derived from parent dir, got:\n%s", code)
	}
}

func TestOpenAPIGenCmd_DetectPeerPackageName(t *testing.T) {
	tmpDir := t.TempDir()
	peerFile := filepath.Join(tmpDir, "helper.go")
	outPath := filepath.Join(tmpDir, "client.go")
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	peerContent := "package existingpkg\n\nfunc Helper() {}\n"
	if err := os.WriteFile(peerFile, []byte(peerContent), 0644); err != nil {
		t.Fatalf("write peer file failed: %v", err)
	}

	specContent := `openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /v1/ping:
    get:
      summary: Ping
`
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	// Omit Pkg so it auto-detects existingpkg from peer helper.go
	genCmd := &cmd.OpenAPIGenCmd{
		Input:  specPath,
		Output: outPath,
	}

	if err := genCmd.Run(context.Background()); err != nil {
		t.Fatalf("genCmd failed: %v", err)
	}

	codeBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read outPath failed: %v", err)
	}

	code := string(codeBytes)
	if !strings.Contains(code, "package existingpkg") {
		t.Errorf("expected package existingpkg detected from peer file, got:\n%s", code)
	}
}

func TestOpenAPIGenCmd_RequestEditors(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "client.go")
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	specContent := `openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /v1/auth:
    get:
      summary: Auth check
`
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	genCmd := &cmd.OpenAPIGenCmd{
		Input:  specPath,
		Output: outPath,
		Pkg:    "client",
	}

	if err := genCmd.Run(context.Background()); err != nil {
		t.Fatalf("genCmd failed: %v", err)
	}

	codeBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read outPath failed: %v", err)
	}

	code := string(codeBytes)
	if !strings.Contains(code, "type RequestEditor func(ctx context.Context, req *http.Request) error") {
		t.Errorf("missing RequestEditor type definition in generated code:\n%s", code)
	}
	if !strings.Contains(code, "for _, editor := range c.RequestEditors") {
		t.Errorf("missing RequestEditors execution loop in generated client method:\n%s", code)
	}
}

func TestOpenAPIGenCmd_ClientInterface(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "client.go")
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	specContent := `openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /v1/users:
    get:
      summary: List users
      parameters:
        - name: limit
          in: query
    post:
      summary: Create user
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
`
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	genCmd := &cmd.OpenAPIGenCmd{Input: specPath, Output: outPath, Pkg: "client", Client: "Client"}
	if err := genCmd.Run(context.Background()); err != nil {
		t.Fatalf("genCmd failed: %v", err)
	}

	codeBytes, _ := os.ReadFile(outPath)
	code := string(codeBytes)

	// Interface type must be generated
	if !strings.Contains(code, "type ClientInterface interface {") {
		t.Errorf("missing ClientInterface type:\n%s", code)
	}
	// Interface must contain all method signatures
	if !strings.Contains(code, "GetV1Users(ctx context.Context, queryParams *GetV1UsersParams) (*Response[[]byte], error)") {
		t.Errorf("missing GetV1Users in ClientInterface:\n%s", code)
	}
	if !strings.Contains(code, "PostV1Users(ctx context.Context, reqBody *PostV1UsersRequest) (*Response[[]byte], error)") {
		t.Errorf("missing PostV1Users in ClientInterface:\n%s", code)
	}
	// Concrete *Client must satisfy the interface (verified by go build in public API tests)
}





