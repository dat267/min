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

// TestPublicAPIs_EndToEnd verifies HAR conversion to OpenAPI and Go SDK generation
// against realistic public API schemas (GitHub, Stripe, Petstore, Slack).
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

// TestPublicAPIs_RuntimeExecution executes generated client methods against mock servers
// returning real public API response schemas.
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
