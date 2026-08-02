package devgen

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type harContainer struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Entries []harEntry `json:"entries"`
}

type harEntry struct {
	Request  harRequest  `json:"request"`
	Response harResponse `json:"response"`
}

type harRequest struct {
	Method      string       `json:"method"`
	URL         string       `json:"url"`
	Headers     []harHeader  `json:"headers"`
	QueryString []harQuery   `json:"queryString"`
	PostData    *harPostData `json:"postData"`
}

type harResponse struct {
	Status     int         `json:"status"`
	StatusText string      `json:"statusText"`
	Headers    []harHeader `json:"headers"`
	Content    *harContent `json:"content"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harQuery struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type harContent struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// HarToOpenAPI reads the HAR file at input and writes (or prints, when output
// is empty) an OpenAPI 3.0 spec. Existing output specs are merged
// incrementally. host and filter narrow the captured requests; apiOnly drops
// static assets.
func HarToOpenAPI(input, output, title, host, filter string, apiOnly bool) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read HAR file: %w", err)
	}

	var har harContainer
	if err := json.Unmarshal(data, &har); err != nil {
		return fmt.Errorf("parse HAR json: %w", err)
	}

	spec := generateOpenAPI(har.Log.Entries, title, host, filter, apiOnly)

	// If output spec file already exists, merge new paths into existing spec
	if output != "" {
		if existingData, err := os.ReadFile(output); err == nil {
			var existingSpec map[string]any
			isYAML := strings.HasSuffix(output, ".yaml") || strings.HasSuffix(output, ".yml")
			var unmarshalErr error
			if isYAML {
				unmarshalErr = yaml.Unmarshal(existingData, &existingSpec)
			} else {
				unmarshalErr = json.Unmarshal(existingData, &existingSpec)
			}

			if unmarshalErr == nil && existingSpec != nil {
				existingPaths, ok := existingSpec["paths"].(map[string]any)
				if !ok {
					existingPaths = make(map[string]any)
					existingSpec["paths"] = existingPaths
				}

				if newPaths, ok := spec["paths"].(map[string]map[string]any); ok {
					for p, methods := range newPaths {
						if _, exists := existingPaths[p]; !exists {
							existingPaths[p] = methods
						} else if existingMethods, ok := existingPaths[p].(map[string]any); ok {
							for m, op := range methods {
								existingMethods[m] = op
							}
						}
					}
					spec["paths"] = existingPaths
				}
			}
		}
	}

	var outData []byte
	isYAML := strings.HasSuffix(output, ".yaml") || strings.HasSuffix(output, ".yml")

	if isYAML {
		outData, err = yaml.Marshal(spec)
		if err != nil {
			return fmt.Errorf("marshal YAML: %w", err)
		}
	} else {
		outData, err = json.MarshalIndent(spec, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}
	}

	if output != "" {
		dir := filepath.Dir(output)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create dir: %w", err)
			}
		}
		if err := os.WriteFile(output, outData, 0644); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
		fmt.Printf("Successfully generated OpenAPI spec at %s\n", output)
	} else {
		fmt.Println(string(outData))
	}

	return nil
}

func generateOpenAPI(entries []harEntry, title, host, filter string, apiOnly bool) map[string]any {
	paths := make(map[string]map[string]any)

	for _, entry := range entries {
		req := entry.Request
		resp := entry.Response

		parsedURL, err := url.Parse(req.URL)
		if err != nil {
			continue
		}

		if host != "" && !strings.EqualFold(parsedURL.Host, host) {
			continue
		}

		pathStr := parsedURL.Path
		if pathStr == "" {
			pathStr = "/"
		}

		if filter != "" && !strings.HasPrefix(pathStr, filter) {
			continue
		}

		if apiOnly && isStaticAsset(pathStr, resp) {
			continue
		}

		method := strings.ToLower(req.Method)
		if method == "" {
			continue
		}

		if paths[pathStr] == nil {
			paths[pathStr] = make(map[string]any)
		}

		op := make(map[string]any)
		op["summary"] = fmt.Sprintf("%s %s", strings.ToUpper(method), pathStr)

		var params []map[string]any
		for _, q := range req.QueryString {
			params = append(params, map[string]any{
				"name":     q.Name,
				"in":       "query",
				"required": false,
				"schema": map[string]any{
					"type": "string",
				},
				"example": q.Value,
			})
		}
		if len(params) > 0 {
			op["parameters"] = params
		}

		if req.PostData != nil && req.PostData.Text != "" {
			mime := req.PostData.MimeType
			if mime == "" {
				mime = "application/json"
			}
			mime = strings.Split(mime, ";")[0]

			bodySchema := inferSchemaFromText(req.PostData.Text)
			op["requestBody"] = map[string]any{
				"content": map[string]any{
					mime: map[string]any{
						"schema": bodySchema,
					},
				},
			}
		}

		statusStr := fmt.Sprintf("%d", resp.Status)
		if resp.Status == 0 {
			statusStr = "200"
		}

		respObj := map[string]any{
			"description": resp.StatusText,
		}
		if resp.StatusText == "" {
			respObj["description"] = "Successful response"
		}

		if resp.Content != nil && resp.Content.Text != "" {
			mime := resp.Content.MimeType
			if mime == "" {
				mime = "application/json"
			}
			mime = strings.Split(mime, ";")[0]

			respSchema := inferSchemaFromText(resp.Content.Text)
			respObj["content"] = map[string]any{
				mime: map[string]any{
					"schema": respSchema,
				},
			}
		}

		op["responses"] = map[string]any{
			statusStr: respObj,
		}

		paths[pathStr][method] = op
	}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   title,
			"version": "1.0.0",
		},
		"paths": paths,
	}
}

func inferSchemaFromText(text string) map[string]any {
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err == nil {
		return inferSchema(raw)
	}
	return map[string]any{
		"type":    "string",
		"example": text,
	}
}

func inferSchema(val any) map[string]any {
	if val == nil {
		return map[string]any{"nullable": true}
	}

	switch v := val.(type) {
	case map[string]any:
		props := make(map[string]any)
		for k, item := range v {
			props[k] = inferSchema(item)
		}
		return map[string]any{
			"type":       "object",
			"properties": props,
		}
	case []any:
		if len(v) > 0 {
			return map[string]any{
				"type":  "array",
				"items": inferSchema(v[0]),
			}
		}
		return map[string]any{
			"type":  "array",
			"items": map[string]any{},
		}
	case string:
		return map[string]any{
			"type":    "string",
			"example": v,
		}
	case float64:
		if float64(int64(v)) == v {
			return map[string]any{
				"type":    "integer",
				"example": int64(v),
			}
		}
		return map[string]any{
			"type":    "number",
			"example": v,
		}
	case bool:
		return map[string]any{
			"type":    "boolean",
			"example": v,
		}
	default:
		return map[string]any{
			"type": "string",
		}
	}
}

func isStaticAsset(path string, resp harResponse) bool {
	ext := strings.ToLower(filepath.Ext(path))
	staticExts := map[string]bool{
		".js": true, ".css": true, ".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".svg": true, ".ico": true, ".woff": true, ".woff2": true,
		".ttf": true, ".eot": true, ".html": true, ".htm": true, ".map": true,
	}
	if staticExts[ext] {
		return true
	}

	if resp.Content != nil && resp.Content.MimeType != "" {
		mime := strings.ToLower(resp.Content.MimeType)
		if strings.HasPrefix(mime, "text/html") ||
			strings.HasPrefix(mime, "text/css") ||
			strings.HasPrefix(mime, "image/") ||
			strings.HasPrefix(mime, "font/") ||
			strings.HasPrefix(mime, "application/javascript") ||
			strings.HasPrefix(mime, "text/javascript") {
			return true
		}
	}

	return false
}
