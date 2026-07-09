package dev

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func ProcessCurl(curlCmd string) (string, []byte, error) {
	req, bodyStr, err := parseCurl(curlCmd)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse curl command: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var jsonRaw any
	structs := ""
	if err := json.Unmarshal(body, &jsonRaw); err == nil {
		structs = generateStructs("Response", jsonRaw)
	}

	sourceCode := generateSourceCode(req, bodyStr, structs)
	return sourceCode, body, nil
}

func parseCurl(curl string) (*http.Request, string, error) {
	tokens := tokenizeCurl(curl)
	method := "GET"
	var rawURL string
	headers := make(http.Header)
	var body string

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch token {
		case "-X", "--request":
			if i+1 < len(tokens) {
				method = tokens[i+1]
				i++
			}
		case "-H", "--header":
			if i+1 < len(tokens) {
				headerKv := tokens[i+1]
				parts := strings.SplitN(headerKv, ":", 2)
				if len(parts) == 2 {
					headers.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				}
				i++
			}
		case "-d", "--data", "--data-raw", "--data-binary":
			if i+1 < len(tokens) {
				body = strings.ReplaceAll(tokens[i+1], "\n", "")
				body = strings.ReplaceAll(body, "\r", "")
				if method == "GET" {
					method = "POST"
				}
				i++
			}
		default:
			if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
				rawURL = token
			}
		}
	}

	if rawURL == "" {
		return nil, "", fmt.Errorf("no URL found in cURL command")
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return nil, "", err
	}
	req.Header = headers
	return req, body, nil
}

func tokenizeCurl(cmd string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune
	escaped := false

	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			if i+1 < len(runes) && runes[i+1] == '\n' {
				i++
				continue
			}
			escaped = true
			continue
		}

		switch {
		case (r == '\'' || r == '"') && !inQuote:
			inQuote = true
			quoteChar = r
		case inQuote && r == quoteChar:
			inQuote = false
			quoteChar = 0
		case unicode.IsSpace(r) && !inQuote:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func generateSourceCode(req *http.Request, reqBody string, structs string) string {
	var sb strings.Builder
	sb.WriteString("package main\n\nimport (\n")
	if structs != "" {
		sb.WriteString("\t\"encoding/json\"\n")
	}
	sb.WriteString("\t\"fmt\"\n\t\"io\"\n\t\"net/http\"\n")
	if reqBody != "" {
		sb.WriteString("\t\"strings\"\n")
	}
	sb.WriteString(")\n\n")

	if structs != "" {
		sb.WriteString(structs)
		sb.WriteString("\n\n")
	}

	sb.WriteString("func main() {\n")
	if reqBody != "" {
		fmt.Fprintf(&sb, "\tbodyReader := strings.NewReader(%s)\n", strconv.Quote(reqBody))
		fmt.Fprintf(&sb, "\treq, err := http.NewRequest(%q, %q, bodyReader)\n", req.Method, req.URL.String())
	} else {
		fmt.Fprintf(&sb, "\treq, err := http.NewRequest(%q, %q, nil)\n", req.Method, req.URL.String())
	}

	sb.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n")

	for key, values := range req.Header {
		for _, val := range values {
			fmt.Fprintf(&sb, "\treq.Header.Set(%q, %q)\n", key, val)
		}
	}

	sb.WriteString("\tclient := &http.Client{}\n")
	sb.WriteString("\tresp, err := client.Do(req)\n")
	sb.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n")
	sb.WriteString("\tdefer resp.Body.Close()\n\n")
	sb.WriteString("\tbody, err := io.ReadAll(resp.Body)\n")
	sb.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n\n")

	if structs != "" {
		sb.WriteString("\tvar result Response\n")
		sb.WriteString("\tif err := json.Unmarshal(body, &result); err != nil {\n\t\tpanic(err)\n\t}\n")
		sb.WriteString("\tfmt.Printf(\"%+v\\n\", result)\n")
	} else {
		sb.WriteString("\tfmt.Println(string(body))\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}

type structCollector struct {
	structs map[string]string
}

func generateStructs(rootName string, data any) string {
	collector := &structCollector{structs: make(map[string]string)}
	collector.walk(rootName, data)

	names := make([]string, 0, len(collector.structs))
	for k := range collector.structs {
		names = append(names, k)
	}
	sort.Strings(names)

	var sb strings.Builder
	for i, k := range names {
		sb.WriteString(collector.structs[k])
		if i < len(names)-1 {
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

func (c *structCollector) walk(name string, data any) string {
	structName := exportedName(name)

	switch v := data.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		fields := make([]string, 0, len(keys))
		for _, k := range keys {
			fieldType := c.walk(k, v[k])
			fields = append(fields, fmt.Sprintf("\t%s %s `json:%q`", exportedName(k), fieldType, k))
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "type %s struct {\n", structName)
		sb.WriteString(strings.Join(fields, "\n"))
		sb.WriteString("\n}")

		c.structs[structName] = sb.String()
		return structName

	case []any:
		if len(v) == 0 {
			return "[]any"
		}
		return "[]" + c.walk(name, v[0])

	case bool:
		return "bool"
	case float64:
		if v == float64(int64(v)) {
			return "int"
		}
		return "float64"
	case string:
		return "string"
	case nil:
		return "any"
	default:
		return reflect.TypeOf(data).String()
	}
}

func exportedName(s string) string {
	if s == "" {
		return "Field"
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || unicode.IsSpace(r)
	})
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	res := strings.Join(parts, "")
	if len(res) > 0 && unicode.IsDigit(rune(res[0])) {
		res = "Num" + res
	}
	return res
}
