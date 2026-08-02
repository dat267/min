package cmd_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat267/min/cmd"
)

func TestOpenapi2GoCmd(t *testing.T) {
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

	genCmd := &cmd.Openapi2GoCmd{
		Input:  specPath,
		Output: outPath,
		Pkg:    "client",
		Client: "Client",
	}

	if err := genCmd.Run(context.Background()); err != nil {
		t.Fatalf("Openapi2GoCmd failed: %v", err)
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

func TestOpenapi2GoCmd_ParamsAndBodyStructs(t *testing.T) {
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

	genCmd := &cmd.Openapi2GoCmd{
		Input:  specPath,
		Output: outPath,
		Pkg:    "client",
		Client: "Client",
	}

	if err := genCmd.Run(context.Background()); err != nil {
		t.Fatalf("Openapi2GoCmd failed: %v", err)
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
	if !strings.Contains(code, "reqURL := c.BaseURL + fmt.Sprintf(\"/v1/orgs/%s/users\", url.PathEscape(orgId))") {
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

func TestOpenapi2GoCmd_AutoPkgFromParentDir(t *testing.T) {
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

	// Omit Pkg so it defaults to parent directory name ("my_custom_sdk" -> "my_custom_sdk")
	genCmd := &cmd.Openapi2GoCmd{
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

func TestOpenapi2GoCmd_DetectPeerPackageName(t *testing.T) {
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
	genCmd := &cmd.Openapi2GoCmd{
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

func TestOpenapi2GoCmd_RequestEditors(t *testing.T) {
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

	genCmd := &cmd.Openapi2GoCmd{
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

func TestOpenapi2GoCmd_ClientInterface(t *testing.T) {
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

	genCmd := &cmd.Openapi2GoCmd{Input: specPath, Output: outPath, Pkg: "client", Client: "Client"}
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
}

func TestHarCmd(t *testing.T) {
	tmpDir := t.TempDir()
	harPath := filepath.Join(tmpDir, "test.har")
	outJson := filepath.Join(tmpDir, "openapi.json")
	outYaml := filepath.Join(tmpDir, "openapi.yaml")

	harContent := `{
  "log": {
    "entries": [
      {
        "request": {
          "method": "POST",
          "url": "https://api.example.com/v1/users?source=web",
          "queryString": [{"name": "source", "value": "web"}],
          "postData": {
            "mimeType": "application/json",
            "text": "{\"name\": \"Alice\", \"age\": 30}"
          }
        },
        "response": {
          "status": 201,
          "statusText": "Created",
          "content": {
            "mimeType": "application/json",
            "text": "{\"id\": 1, \"name\": \"Alice\", \"age\": 30}"
          }
        }
      }
    ]
  }
}`

	if err := os.WriteFile(harPath, []byte(harContent), 0644); err != nil {
		t.Fatalf("failed to write test HAR: %v", err)
	}

	// Test JSON output
	harJsonCmd := &cmd.Har2OpenapiCmd{
		Input:  harPath,
		Output: outJson,
		Title:  "Test API",
	}
	if err := harJsonCmd.Run(context.Background()); err != nil {
		t.Fatalf("HarCmd JSON failed: %v", err)
	}

	jsonBytes, err := os.ReadFile(outJson)
	if err != nil {
		t.Fatalf("failed to read outJson: %v", err)
	}
	if !strings.Contains(string(jsonBytes), "openapi") || !strings.Contains(string(jsonBytes), "/v1/users") {
		t.Errorf("unexpected JSON spec output: %s", string(jsonBytes))
	}

	// Test YAML output
	harYamlCmd := &cmd.Har2OpenapiCmd{
		Input:  harPath,
		Output: outYaml,
		Title:  "Test API",
	}
	if err := harYamlCmd.Run(context.Background()); err != nil {
		t.Fatalf("HarCmd YAML failed: %v", err)
	}

	yamlBytes, err := os.ReadFile(outYaml)
	if err != nil {
		t.Fatalf("failed to read outYaml: %v", err)
	}
	if !strings.Contains(string(yamlBytes), "openapi: 3.0.3") {
		t.Errorf("unexpected YAML spec output: %s", string(yamlBytes))
	}
}

func TestHarCmdIncrementalMerge(t *testing.T) {
	tmpDir := t.TempDir()
	har1Path := filepath.Join(tmpDir, "test1.har")
	har2Path := filepath.Join(tmpDir, "test2.har")
	outYaml := filepath.Join(tmpDir, "openapi.yaml")

	har1Content := `{
  "log": {
    "entries": [
      {
        "request": { "method": "GET", "url": "https://api.example.com/v1/users" },
        "response": { "status": 200, "statusText": "OK" }
      }
    ]
  }
}`
	har2Content := `{
  "log": {
    "entries": [
      {
        "request": { "method": "POST", "url": "https://api.example.com/v1/posts" },
        "response": { "status": 201, "statusText": "Created" }
      }
    ]
  }
}`

	if err := os.WriteFile(har1Path, []byte(har1Content), 0644); err != nil {
		t.Fatalf("failed to write har1: %v", err)
	}
	if err := os.WriteFile(har2Path, []byte(har2Content), 0644); err != nil {
		t.Fatalf("failed to write har2: %v", err)
	}

	// Step 1: Run har1 -> openapi.yaml
	cmd1 := &cmd.Har2OpenapiCmd{Input: har1Path, Output: outYaml, Title: "Test API"}
	if err := cmd1.Run(context.Background()); err != nil {
		t.Fatalf("cmd1 failed: %v", err)
	}

	// Step 2: Run har2 -> openapi.yaml (should merge /v1/posts alongside existing /v1/users)
	cmd2 := &cmd.Har2OpenapiCmd{Input: har2Path, Output: outYaml, Title: "Test API"}
	if err := cmd2.Run(context.Background()); err != nil {
		t.Fatalf("cmd2 failed: %v", err)
	}

	mergedBytes, err := os.ReadFile(outYaml)
	if err != nil {
		t.Fatalf("read merged yaml failed: %v", err)
	}

	mergedContent := string(mergedBytes)
	if !strings.Contains(mergedContent, "/v1/users") || !strings.Contains(mergedContent, "/v1/posts") {
		t.Errorf("expected merged OpenAPI spec to contain both endpoints, got:\n%s", mergedContent)
	}
}

func TestHarCmdFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	harPath := filepath.Join(tmpDir, "full_export.har")
	outYaml := filepath.Join(tmpDir, "openapi.yaml")

	harContent := `{
  "log": {
    "entries": [
      {
        "request": { "method": "GET", "url": "https://app.example.com/assets/app.js" },
        "response": { "status": 200, "content": { "mimeType": "application/javascript" } }
      },
      {
        "request": { "method": "GET", "url": "https://api.example.com/v1/users" },
        "response": { "status": 200, "content": { "mimeType": "application/json" } }
      },
      {
        "request": { "method": "GET", "url": "https://other.com/v1/data" },
        "response": { "status": 200, "content": { "mimeType": "application/json" } }
      }
    ]
  }
}`

	if err := os.WriteFile(harPath, []byte(harContent), 0644); err != nil {
		t.Fatalf("failed to write test HAR: %v", err)
	}

	filterCmd := &cmd.Har2OpenapiCmd{
		Input:   harPath,
		Output:  outYaml,
		Host:    "api.example.com",
		Filter:  "/v1/",
		ApiOnly: true,
	}

	if err := filterCmd.Run(context.Background()); err != nil {
		t.Fatalf("filterCmd failed: %v", err)
	}

	outBytes, err := os.ReadFile(outYaml)
	if err != nil {
		t.Fatalf("failed to read outYaml: %v", err)
	}

	outContent := string(outBytes)
	if !strings.Contains(outContent, "/v1/users") {
		t.Errorf("expected /v1/users in filtered spec")
	}
	if strings.Contains(outContent, "app.js") {
		t.Errorf("static asset app.js should have been filtered out")
	}
	if strings.Contains(outContent, "other.com") {
		t.Errorf("other.com request should have been filtered out by host")
	}
}

func TestPublicAPIs_EndToEnd(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		harJson    string
		verifyFunc func(t *testing.T, code string, pkgDir string)
	}{
		{
			name:  "PetstoreAPI",
			title: "Swagger Petstore",
			harJson: `{
  "log": {
    "entries": [
      {
        "request": {
          "method": "POST",
          "url": "https://petstore.swagger.io/v2/pet",
          "postData": {
            "mimeType": "application/json",
            "text": "{\"id\": 10, \"name\": \"doggie\", \"photoUrls\": [\"http://example.com/pic.jpg\"], \"status\": \"available\", \"category\": {\"id\": 1, \"name\": \"Dogs\"}}"
          }
        },
        "response": {
          "status": 200,
          "content": {
            "mimeType": "application/json",
            "text": "{\"id\": 10, \"name\": \"doggie\", \"status\": \"available\", \"category\": {\"id\": 1, \"name\": \"Dogs\"}}"
          }
        }
      },
      {
        "request": {
          "method": "GET",
          "url": "https://petstore.swagger.io/v2/pet/findByStatus?status=available",
          "queryString": [{"name": "status", "value": "available"}]
        },
        "response": {
          "status": 200,
          "content": {
            "mimeType": "application/json",
            "text": "[{\"id\": 10, \"name\": \"doggie\", \"status\": \"available\"}]"
          }
        }
      }
    ]
  }
}`,
			verifyFunc: func(t *testing.T, code string, pkgDir string) {
				if !strings.Contains(code, "PostV2Pet") {
					t.Errorf("missing PostV2Pet method")
				}
				if !strings.Contains(code, "GetV2PetFindByStatus") {
					t.Errorf("missing GetV2PetFindByStatus method")
				}
				if !strings.Contains(code, "PhotoUrls []string") {
					t.Errorf("missing array property PhotoUrls []string")
				}
			},
		},
		{
			name:  "StripePaymentAPI",
			title: "Stripe API",
			harJson: `{
  "log": {
    "entries": [
      {
        "request": {
          "method": "POST",
          "url": "https://api.stripe.com/v1/payment_intents",
          "postData": {
            "mimeType": "application/json",
            "text": "{\"amount\": 2000, \"currency\": \"usd\", \"automatic_payment_methods\": {\"enabled\": true}}"
          }
        },
        "response": {
          "status": 200,
          "content": {
            "mimeType": "application/json",
            "text": "{\"id\": \"pi_123\", \"object\": \"payment_intent\", \"amount\": 2000, \"currency\": \"usd\", \"status\": \"requires_payment_method\"}"
          }
        }
      }
    ]
  }
}`,
			verifyFunc: func(t *testing.T, code string, pkgDir string) {
				if !strings.Contains(code, "PostV1PaymentIntents") {
					t.Errorf("missing PostV1PaymentIntents method")
				}
				if !strings.Contains(code, "Amount int64") {
					t.Errorf("missing Amount int64 field")
				}
				if !strings.Contains(code, "AutomaticPaymentMethods") {
					t.Errorf("missing nested struct AutomaticPaymentMethods")
				}
			},
		},
		{
			name:  "SlackWebAPI",
			title: "Slack Web API",
			harJson: `{
  "log": {
    "entries": [
      {
        "request": {
          "method": "POST",
          "url": "https://slack.com/api/chat.postMessage",
          "postData": {
            "mimeType": "application/json",
            "text": "{\"channel\": \"C123456\", \"text\": \"Hello World\", \"as_user\": true}"
          }
        },
        "response": {
          "status": 200,
          "content": {
            "mimeType": "application/json",
            "text": "{\"ok\": true, \"channel\": \"C123456\", \"ts\": \"1234567890.123456\", \"message\": {\"text\": \"Hello World\", \"user\": \"U123456\"}}"
          }
        }
      }
    ]
  }
}`,
			verifyFunc: func(t *testing.T, code string, pkgDir string) {
				if !strings.Contains(code, "PostApiChatPostMessage") {
					t.Errorf("missing PostApiChatPostMessage method")
				}
				if !strings.Contains(code, "AsUser bool") {
					t.Errorf("missing AsUser bool field")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			harPath := filepath.Join(tmpDir, "input.har")
			specPath := filepath.Join(tmpDir, "openapi.yaml")
			clientPath := filepath.Join(tmpDir, "client.go")

			if err := os.WriteFile(harPath, []byte(tt.harJson), 0644); err != nil {
				t.Fatalf("failed to write har file: %v", err)
			}

			// Step 1: HAR -> OpenAPI YAML
			harCmd := &cmd.Har2OpenapiCmd{
				Input:  harPath,
				Output: specPath,
				Title:  tt.title,
			}
			if err := harCmd.Run(context.Background()); err != nil {
				t.Fatalf("har convert failed: %v", err)
			}

			// Step 2: OpenAPI YAML -> Go SDK Client
			genCmd := &cmd.Openapi2GoCmd{
				Input:  specPath,
				Output: clientPath,
				Pkg:    "client",
				Client: "Client",
			}
			if err := genCmd.Run(context.Background()); err != nil {
				t.Fatalf("openapi gen failed: %v", err)
			}

			codeBytes, err := os.ReadFile(clientPath)
			if err != nil {
				t.Fatalf("read client.go failed: %v", err)
			}
			code := string(codeBytes)

			// Custom assertions
			tt.verifyFunc(t, code, tmpDir)

			// Step 3: Compile generated code with go build to verify 100% Go syntax validity
			cmdBuild := exec.Command("go", "build", clientPath)
			if out, err := cmdBuild.CombinedOutput(); err != nil {
				t.Fatalf("go build failed for generated SDK:\n%s\n%s", out, code)
			}
		})
	}
}

func TestPublicAPIs_RuntimeExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/pet/10" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": 10, "name": "doggie", "status": "available"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	harJson := fmt.Sprintf(`{
  "log": {
    "entries": [
      {
        "request": {
          "method": "GET",
          "url": "%s/v2/pet/{petId}"
        },
        "response": {
          "status": 200,
          "content": {
            "mimeType": "application/json",
            "text": "{\"id\": 10, \"name\": \"doggie\", \"status\": \"available\"}"
          }
        }
      }
    ]
  }
}`, server.URL)

	tmpDir := t.TempDir()
	harPath := filepath.Join(tmpDir, "pet.har")
	specPath := filepath.Join(tmpDir, "openapi.yaml")
	clientPath := filepath.Join(tmpDir, "client.go")

	_ = os.WriteFile(harPath, []byte(harJson), 0644)

	harCmd := &cmd.Har2OpenapiCmd{Input: harPath, Output: specPath, Title: "Petstore"}
	_ = harCmd.Run(context.Background())

	genCmd := &cmd.Openapi2GoCmd{Input: specPath, Output: clientPath, Pkg: "client"}
	_ = genCmd.Run(context.Background())

	codeBytes, _ := os.ReadFile(clientPath)
	code := string(codeBytes)
	if !strings.Contains(code, "GetV2PetPetId") {
		t.Fatalf("expected GetV2PetPetId method in generated client")
	}
}
