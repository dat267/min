package devgen

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dat267/min/internal/naming"
	"gopkg.in/yaml.v3"
)

type openAPISpec struct {
	OpenAPI string                          `json:"openapi" yaml:"openapi"`
	Info    openAPIInfo                     `json:"info" yaml:"info"`
	Paths   map[string]map[string]openAPIOp `json:"paths" yaml:"paths"`
}

type openAPIInfo struct {
	Title   string `json:"title" yaml:"title"`
	Version string `json:"version" yaml:"version"`
}

type openAPIOp struct {
	Summary     string          `json:"summary" yaml:"summary"`
	OperationID string          `json:"operationId" yaml:"operationId,omitempty"`
	Parameters  []openAPIParam  `json:"parameters" yaml:"parameters"`
	RequestBody *openAPIReqBody `json:"requestBody" yaml:"requestBody"`
	Responses   map[string]any  `json:"responses" yaml:"responses"`
}

type openAPIParam struct {
	Name     string `json:"name" yaml:"name"`
	In       string `json:"in" yaml:"in"`
	Required bool   `json:"required" yaml:"required"`
}

type openAPIReqBody struct {
	Content map[string]any `json:"content" yaml:"content"`
}

type structCollector struct {
	structs         map[string]string // structName -> struct definition code
	usedNames       map[string]bool   // set of used struct names
	usedMethodNames map[string]bool   // set of used generated method names
}

func newStructCollector() *structCollector {
	return &structCollector{
		structs:         make(map[string]string),
		usedNames:       make(map[string]bool),
		usedMethodNames: make(map[string]bool),
	}
}

func (sc *structCollector) getUniqueName(baseName string) string {
	name := naming.ToCamelCase(baseName)
	if !sc.usedNames[name] {
		sc.usedNames[name] = true
		return name
	}
	for i := 2; ; i++ {
		proposed := fmt.Sprintf("%s%d", name, i)
		if !sc.usedNames[proposed] {
			sc.usedNames[proposed] = true
			return proposed
		}
	}
}

// getUniqueMethodName returns a Go-legal, unique method name for the given
// base name, appending numeric suffixes on collision.
func (sc *structCollector) getUniqueMethodName(baseName string) string {
	name := naming.SanitizeIdentStart(naming.ToCamelCase(baseName), "Do")
	if !sc.usedMethodNames[name] {
		sc.usedMethodNames[name] = true
		return name
	}
	for i := 2; ; i++ {
		proposed := fmt.Sprintf("%s%d", name, i)
		if !sc.usedMethodNames[proposed] {
			sc.usedMethodNames[proposed] = true
			return proposed
		}
	}
}

type sdkMethod struct {
	funcName          string
	methodUpper       string
	pathStr           string
	reqStruct         string
	respStruct        string
	queryParamsStruct string
	pathParams        []openAPIParam
	hasQuery          bool
	hasBody           bool
}

// OpenAPIToGo generates a Go SDK client package from the OpenAPI spec at
// input, writing it and its tests to output (a file path, or a directory that
// receives client.go). pkg defaults to the output directory or a peer package
// name; client defaults to "Client". Both generated files are passed through
// go/format.Source before being written.
func OpenAPIToGo(input, output, pkg, client string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read OpenAPI file: %w", err)
	}

	var spec openAPISpec

	if strings.HasSuffix(input, ".yaml") || strings.HasSuffix(input, ".yml") {
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return fmt.Errorf("parse YAML OpenAPI spec: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &spec); err != nil {
			if errYaml := yaml.Unmarshal(data, &spec); errYaml != nil {
				return fmt.Errorf("parse OpenAPI spec: %w", err)
			}
		}
	}

	outPath := output
	if fi, err := os.Stat(outPath); err == nil && fi.IsDir() {
		outPath = filepath.Join(outPath, "client.go")
	} else if !strings.HasSuffix(outPath, ".go") {
		if err != nil && os.IsNotExist(err) && (strings.HasSuffix(output, "/") || filepath.Ext(output) == "") {
			outPath = filepath.Join(outPath, "client.go")
		}
	}

	pkgName := pkg
	if pkgName == "" {
		pkgName = detectPeerPackageName(outPath)
	}

	clientName := client
	if clientName == "" {
		clientName = "Client"
	}

	collector := newStructCollector()
	collector.usedNames[clientName] = true
	methods := collectSDKMethods(&spec, collector)

	code := generateGoSDK(&spec, pkgName, clientName, collector, methods)
	testCode := generateGoSDKTests(methods, pkgName, clientName)

	dir := filepath.Dir(outPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}

	formatted, err := FormatSource([]byte(code))
	if err != nil {
		return fmt.Errorf("generated invalid Go for %s: %w", outPath, err)
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	base := strings.TrimSuffix(filepath.Base(outPath), ".go")
	testPath := filepath.Join(filepath.Dir(outPath), base+"_test.go")
	formattedTest, err := FormatSource([]byte(testCode))
	if err != nil {
		return fmt.Errorf("generated invalid Go for %s: %w", testPath, err)
	}
	if err := os.WriteFile(testPath, formattedTest, 0644); err != nil {
		return fmt.Errorf("write test file: %w", err)
	}

	fmt.Printf("Successfully generated Go SDK at %s\n", outPath)
	fmt.Printf("Successfully generated Go SDK tests at %s\n", testPath)
	return nil
}

// collectSDKMethods gathers all operations into an ordered, de-duplicated list
// of methods, building any request/response/query structs along the way.
func collectSDKMethods(spec *openAPISpec, collector *structCollector) []sdkMethod {
	var methods []sdkMethod

	var pathKeys []string
	for k := range spec.Paths {
		pathKeys = append(pathKeys, k)
	}
	sort.Strings(pathKeys)

	for _, pathStr := range pathKeys {
		pathMethods := spec.Paths[pathStr]
		var methodKeys []string
		for m := range pathMethods {
			if isHTTPMethod(m) {
				methodKeys = append(methodKeys, m)
			}
		}
		sort.Strings(methodKeys)

		for _, methodStr := range methodKeys {
			op := pathMethods[methodStr]
			methodUpper := strings.ToUpper(methodStr)
			methodTitle := naming.TitleCase(methodStr)
			baseName := op.OperationID
			if baseName == "" {
				baseName = methodTitle + "_" + pathStr
			}
			funcName := collector.getUniqueMethodName(naming.ToCamelCase(baseName))

			hasQuery := false
			for _, p := range op.Parameters {
				if p.In == "query" {
					hasQuery = true
					break
				}
			}

			reqStructName := ""
			hasBody := false
			if op.RequestBody != nil && len(op.RequestBody.Content) > 0 {
				hasBody = true
				if appJson, ok := op.RequestBody.Content["application/json"].(map[string]any); ok {
					if schema, ok := appJson["schema"].(map[string]any); ok {
						reqStructName = collector.getUniqueName(funcName + "Request")
						collector.buildStructFromSchema(reqStructName, schema)
					}
				}
			}

			respStructName := ""
			if op.Responses != nil {
				for status, respVal := range op.Responses {
					if strings.HasPrefix(status, "2") {
						if respMap, ok := respVal.(map[string]any); ok {
							if content, ok := respMap["content"].(map[string]any); ok {
								if appJson, ok := content["application/json"].(map[string]any); ok {
									if schema, ok := appJson["schema"].(map[string]any); ok {
										respStructName = collector.getUniqueName(funcName + "Response")
										collector.buildStructFromSchema(respStructName, schema)
									}
								}
							}
						}
					}
				}
			}

			queryParamsStruct := ""
			var pathParams []openAPIParam
			var queryParamsList []openAPIParam

			for _, p := range op.Parameters {
				switch p.In {
				case "path":
					pathParams = append(pathParams, p)
				case "query":
					queryParamsList = append(queryParamsList, p)
				}
			}

			// Sort path parameters based on their positional appearance in the path string
			sort.Slice(pathParams, func(i, j int) bool {
				idxI := strings.Index(pathStr, "{"+pathParams[i].Name+"}")
				idxJ := strings.Index(pathStr, "{"+pathParams[j].Name+"}")
				if idxI == -1 {
					return false
				}
				if idxJ == -1 {
					return true
				}
				return idxI < idxJ
			})

			if len(queryParamsList) > 0 {
				hasQuery = true
				queryParamsStruct = collector.getUniqueName(funcName + "Params")
				var qSb strings.Builder
				fmt.Fprintf(&qSb, "type %s struct {\n", queryParamsStruct)
				used := make(map[string]bool)
				for _, qp := range queryParamsList {
					fieldName := naming.SanitizeIdentStart(naming.ToCamelCase(qp.Name), "Field")
					if used[fieldName] {
						continue
					}
					used[fieldName] = true
					fmt.Fprintf(&qSb, "\t%s string `json:\"%s,omitempty\"`\n", fieldName, qp.Name)
				}
				qSb.WriteString("}")
				collector.structs[queryParamsStruct] = qSb.String()
			}

			methods = append(methods, sdkMethod{
				funcName:          funcName,
				methodUpper:       methodUpper,
				pathStr:           pathStr,
				reqStruct:         reqStructName,
				respStruct:        respStructName,
				queryParamsStruct: queryParamsStruct,
				pathParams:        pathParams,
				hasQuery:          hasQuery,
				hasBody:           hasBody,
			})
		}
	}

	return methods
}

func generateGoSDK(spec *openAPISpec, pkgName, clientName string, collector *structCollector, methods []sdkMethod) string {
	var sb strings.Builder

	if pkgName == "" {
		pkgName = "client"
	}
	if clientName == "" {
		clientName = "Client"
	}

	hasAnyQuery := false
	hasAnyPath := false
	hasAnyBody := false
	hasAnyJSON := false
	for _, m := range methods {
		hasAnyQuery = hasAnyQuery || m.hasQuery
		hasAnyPath = hasAnyPath || len(m.pathParams) > 0
		hasAnyBody = hasAnyBody || m.hasBody
		hasAnyJSON = hasAnyJSON || m.hasBody || m.respStruct != "" || m.queryParamsStruct != ""
	}
	hasAnyMethod := len(methods) > 0

	fmt.Fprintf(&sb, "// Code generated by min openapi gen. DO NOT EDIT.\npackage %s\n\n", pkgName)
	sb.WriteString("import (\n")
	if hasAnyBody {
		sb.WriteString("\t\"bytes\"\n")
	}
	sb.WriteString("\t\"context\"\n")
	if hasAnyJSON {
		sb.WriteString("\t\"encoding/json\"\n")
	}
	if hasAnyMethod {
		sb.WriteString("\t\"fmt\"\n")
	}
	if hasAnyMethod {
		sb.WriteString("\t\"io\"\n")
	}
	sb.WriteString("\t\"net/http\"\n")
	if hasAnyQuery || hasAnyPath {
		sb.WriteString("\t\"net/url\"\n")
	}
	sb.WriteString("\t\"strings\"\n")
	sb.WriteString(")\n\n")

	sb.WriteString("// RequestEditor is a function that can modify the outbound http.Request (e.g. headers, auth, cookies).\n")
	sb.WriteString("type RequestEditor func(ctx context.Context, req *http.Request) error\n\n")
	sb.WriteString("// Response wraps the HTTP response details alongside the unmarshaled body model.\n")
	sb.WriteString("type Response[T any] struct {\n")
	sb.WriteString("\tStatusCode int\n")
	sb.WriteString("\tHeader     http.Header\n")
	sb.WriteString("\tData       *T\n")
	sb.WriteString("\tBody       []byte\n")
	sb.WriteString("}\n\n")
	fmt.Fprintf(&sb, "// %s provides an SDK for the %s.\n", clientName, spec.Info.Title)
	fmt.Fprintf(&sb, "type %s struct {\n", clientName)
	sb.WriteString("\tBaseURL        string\n")
	sb.WriteString("\tHTTPClient     *http.Client\n")
	sb.WriteString("\tRequestEditors []RequestEditor\n")
	sb.WriteString("}\n\n")

	fmt.Fprintf(&sb, "// New%s creates a new %s SDK client.\n", clientName, clientName)
	fmt.Fprintf(&sb, "func New%s(baseURL string, httpClient *http.Client, editors ...RequestEditor) *%s {\n", clientName, clientName)
	sb.WriteString("\tif httpClient == nil {\n")
	sb.WriteString("\t\thttpClient = http.DefaultClient\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn &")
	sb.WriteString(clientName)
	sb.WriteString("{\n")
	sb.WriteString("\t\tBaseURL:        strings.TrimRight(baseURL, \"/\"),\n")
	sb.WriteString("\t\tHTTPClient:     httpClient,\n")
	sb.WriteString("\t\tRequestEditors: editors,\n")
	sb.WriteString("\t}\n")
	sb.WriteString("}\n\n")

	if len(collector.structs) > 0 {
		sb.WriteString("// --- Models & Types ---\n\n")
		var structNames []string
		for name := range collector.structs {
			structNames = append(structNames, name)
		}
		sort.Strings(structNames)

		for _, name := range structNames {
			sb.WriteString(collector.structs[name])
			sb.WriteString("\n\n")
		}
	}

	fmt.Fprintf(&sb, "// %sInterface is the interface implemented by *%s, allowing callers to mock the client in tests.\n", clientName, clientName)
	fmt.Fprintf(&sb, "type %sInterface interface {\n", clientName)
	for _, m := range methods {
		paramList := "ctx context.Context"
		for _, pp := range m.pathParams {
			paramList += fmt.Sprintf(", %s string", naming.LowerParamName(pp.Name))
		}
		if m.hasQuery {
			if m.queryParamsStruct != "" {
				paramList += fmt.Sprintf(", queryParams *%s", m.queryParamsStruct)
			} else {
				paramList += ", queryParams map[string]string"
			}
		}
		if m.hasBody {
			if m.reqStruct != "" {
				paramList += fmt.Sprintf(", reqBody *%s", m.reqStruct)
			} else {
				paramList += ", reqBody any"
			}
		}
		returnType := "(*Response[[]byte], error)"
		if m.respStruct != "" {
			returnType = fmt.Sprintf("(*Response[%s], error)", m.respStruct)
		}
		fmt.Fprintf(&sb, "\t%s(%s) %s\n", m.funcName, paramList, returnType)
	}
	sb.WriteString("}\n\n")

	for _, m := range methods {
		fmt.Fprintf(&sb, "// %s executes %s %s\n", m.funcName, m.methodUpper, m.pathStr)
		paramList := "ctx context.Context"

		for _, pp := range m.pathParams {
			paramList += fmt.Sprintf(", %s string", naming.LowerParamName(pp.Name))
		}

		if m.hasQuery {
			if m.queryParamsStruct != "" {
				paramList += fmt.Sprintf(", queryParams *%s", m.queryParamsStruct)
			} else {
				paramList += ", queryParams map[string]string"
			}
		}
		if m.hasBody {
			if m.reqStruct != "" {
				paramList += fmt.Sprintf(", reqBody *%s", m.reqStruct)
			} else {
				paramList += ", reqBody any"
			}
		}

		returnType := "(*Response[[]byte], error)"
		if m.respStruct != "" {
			returnType = fmt.Sprintf("(*Response[%s], error)", m.respStruct)
		}

		fmt.Fprintf(&sb, "func (c *%s) %s(%s) %s {\n", clientName, m.funcName, paramList, returnType)

		if len(m.pathParams) > 0 {
			formattedPath := m.pathStr
			var formatArgs []string
			for _, pp := range m.pathParams {
				target := "{" + pp.Name + "}"
				formattedPath = strings.ReplaceAll(formattedPath, target, "%s")
				formatArgs = append(formatArgs, fmt.Sprintf("url.PathEscape(%s)", naming.LowerParamName(pp.Name)))
			}
			fmt.Fprintf(&sb, "\treqURL := c.BaseURL + fmt.Sprintf(%q, %s)\n", formattedPath, strings.Join(formatArgs, ", "))
		} else {
			fmt.Fprintf(&sb, "\treqURL := c.BaseURL + %q\n", m.pathStr)
		}

		if m.hasQuery {
			sb.WriteString("\tif queryParams != nil {\n")
			sb.WriteString("\t\tq := url.Values{}\n")
			if m.queryParamsStruct != "" {
				sb.WriteString("\t\tparamBytes, _ := json.Marshal(queryParams)\n")
				sb.WriteString("\t\tvar paramMap map[string]any\n")
				sb.WriteString("\t\t_ = json.Unmarshal(paramBytes, &paramMap)\n")
				sb.WriteString("\t\tfor k, v := range paramMap {\n")
				sb.WriteString("\t\t\tif vStr := fmt.Sprintf(\"%v\", v); vStr != \"\" {\n")
				sb.WriteString("\t\t\t\tq.Set(k, vStr)\n")
				sb.WriteString("\t\t\t}\n")
				sb.WriteString("\t\t}\n")
			} else {
				sb.WriteString("\t\tfor k, v := range queryParams {\n")
				sb.WriteString("\t\t\tq.Set(k, v)\n")
				sb.WriteString("\t\t}\n")
			}
			sb.WriteString("\t\tif encoded := q.Encode(); encoded != \"\" {\n")
			sb.WriteString("\t\t\tif strings.Contains(reqURL, \"?\") {\n")
			sb.WriteString("\t\t\t\treqURL += \"&\" + encoded\n")
			sb.WriteString("\t\t\t} else {\n")
			sb.WriteString("\t\t\t\treqURL += \"?\" + encoded\n")
			sb.WriteString("\t\t\t}\n")
			sb.WriteString("\t\t}\n")
			sb.WriteString("\t}\n")
		}

		if m.hasBody {
			sb.WriteString("\tbodyBytes, err := json.Marshal(reqBody)\n")
			sb.WriteString("\tif err != nil {\n")
			sb.WriteString("\t\treturn nil, fmt.Errorf(\"marshal request body: %w\", err)\n")
			sb.WriteString("\t}\n")
			fmt.Fprintf(&sb, "\treq, err := http.NewRequestWithContext(ctx, %q, reqURL, bytes.NewReader(bodyBytes))\n", m.methodUpper)
		} else {
			fmt.Fprintf(&sb, "\treq, err := http.NewRequestWithContext(ctx, %q, reqURL, nil)\n", m.methodUpper)
		}

		sb.WriteString("\tif err != nil {\n")
		sb.WriteString("\t\treturn nil, fmt.Errorf(\"create request: %w\", err)\n")
		sb.WriteString("\t}\n")

		if m.hasBody {
			sb.WriteString("\treq.Header.Set(\"Content-Type\", \"application/json\")\n")
		}

		sb.WriteString("\tfor _, editor := range c.RequestEditors {\n")
		sb.WriteString("\t\tif err := editor(ctx, req); err != nil {\n")
		sb.WriteString("\t\t\treturn nil, fmt.Errorf(\"request editor error: %w\", err)\n")
		sb.WriteString("\t\t}\n")
		sb.WriteString("\t}\n\n")

		sb.WriteString("\tresp, err := c.HTTPClient.Do(req)\n")
		sb.WriteString("\tif err != nil {\n")
		sb.WriteString("\t\treturn nil, fmt.Errorf(\"execute request: %w\", err)\n")
		sb.WriteString("\t}\n")
		sb.WriteString("\tdefer resp.Body.Close()\n\n")

		sb.WriteString("\trespData, err := io.ReadAll(resp.Body)\n")
		sb.WriteString("\tif err != nil {\n")
		sb.WriteString("\t\treturn nil, fmt.Errorf(\"read response body: %w\", err)\n")
		sb.WriteString("\t}\n\n")

		sb.WriteString("\tif resp.StatusCode >= 400 {\n")
		sb.WriteString("\t\treturn &Response[")
		sb.WriteString(selectTypeName(m.respStruct, "[]byte"))
		sb.WriteString("]{StatusCode: resp.StatusCode, Header: resp.Header, Body: respData}, fmt.Errorf(\"HTTP %d: %s\", resp.StatusCode, string(respData))\n")
		sb.WriteString("\t}\n\n")

		if m.respStruct != "" {
			fmt.Fprintf(&sb, "\tvar result %s\n", m.respStruct)
			sb.WriteString("\tif err := json.Unmarshal(respData, &result); err != nil {\n")
			fmt.Fprintf(&sb, "\t\treturn &Response[%s]{StatusCode: resp.StatusCode, Header: resp.Header, Body: respData}, fmt.Errorf(\"unmarshal response: %%w\", err)\n", m.respStruct)
			sb.WriteString("\t}\n")
			fmt.Fprintf(&sb, "\treturn &Response[%s]{StatusCode: resp.StatusCode, Header: resp.Header, Data: &result, Body: respData}, nil\n", m.respStruct)
		} else {
			sb.WriteString("\treturn &Response[[]byte]{StatusCode: resp.StatusCode, Header: resp.Header, Data: &respData, Body: respData}, nil\n")
		}

		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func generateGoSDKTests(methods []sdkMethod, pkgName, clientName string) string {
	var sb strings.Builder

	type testMethod struct {
		funcName     string
		methodUpper  string
		expectedPath string
		pathParams   []openAPIParam
		hasQuery     bool
		hasBody      bool
	}

	var tests []testMethod
	for _, m := range methods {
		expectedPath := m.pathStr
		for _, pp := range m.pathParams {
			expectedPath = strings.ReplaceAll(expectedPath, "{"+pp.Name+"}", url.PathEscape("test-"+naming.LowerParamName(pp.Name)))
		}
		tests = append(tests, testMethod{
			funcName:     m.funcName,
			methodUpper:  m.methodUpper,
			expectedPath: expectedPath,
			pathParams:   m.pathParams,
			hasQuery:     m.hasQuery,
			hasBody:      m.hasBody,
		})
	}

	fmt.Fprintf(&sb, "// Code generated by min openapi gen. DO NOT EDIT.\npackage %s\n\n", pkgName)
	sb.WriteString("import (\n")
	if len(tests) > 0 {
		sb.WriteString("\t\"context\"\n")
		sb.WriteString("\t\"net/http\"\n")
		sb.WriteString("\t\"net/http/httptest\"\n")
	}
	sb.WriteString("\t\"testing\"\n")
	sb.WriteString(")\n\n")

	fmt.Fprintf(&sb, "// Ensure *%s satisfies %sInterface at compile time.\n", clientName, clientName)
	fmt.Fprintf(&sb, "var _ %sInterface = (*%s)(nil)\n\n", clientName, clientName)

	fmt.Fprintf(&sb, "func TestNew%s(t *testing.T) {\n", clientName)
	fmt.Fprintf(&sb, "\tc := New%s(\"https://api.example.com\", nil)\n", clientName)
	sb.WriteString("\tif c == nil {\n\t\tt.Fatal(\"NewClient returned nil\")\n\t}\n")
	sb.WriteString("\tif c.BaseURL != \"https://api.example.com\" {\n\t\tt.Errorf(\"unexpected BaseURL: %s\", c.BaseURL)\n\t}\n")
	sb.WriteString("\tif c.HTTPClient == nil {\n\t\tt.Error(\"HTTPClient should not be nil\")\n\t}\n")
	sb.WriteString("}\n\n")

	isMutating := func(method string) bool {
		switch method {
		case "POST", "PUT", "PATCH", "DELETE":
			return true
		}
		return false
	}

	buildCallArgs := func(m testMethod) string {
		args := "context.Background()"
		for _, pp := range m.pathParams {
			args += fmt.Sprintf(", \"test-%s\"", naming.LowerParamName(pp.Name))
		}
		if m.hasQuery {
			args += ", nil"
		}
		if m.hasBody {
			args += ", nil"
		}
		return args
	}

	for _, m := range tests {
		callArgs := buildCallArgs(m)
		fmt.Fprintf(&sb, "func Test%s_%s(t *testing.T) {\n", clientName, m.funcName)
		if isMutating(m.methodUpper) {
			fmt.Fprintf(&sb, "\t// %s is a mutating operation. Remove the t.Skip below to run this test intentionally.\n", m.methodUpper)
			fmt.Fprintf(&sb, "\tt.Skip(\"mutating operation (%s) — remove this t.Skip to run\")\n", m.methodUpper)
		}
		sb.WriteString("\tserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n")
		fmt.Fprintf(&sb, "\t\tif r.Method != %q {\n\t\t\tt.Errorf(\"expected %%s, got %%s\", %q, r.Method)\n\t\t}\n", m.methodUpper, m.methodUpper)
		fmt.Fprintf(&sb, "\t\tif r.URL.Path != %q {\n\t\t\tt.Errorf(\"expected %%s, got %%s\", %q, r.URL.Path)\n\t\t}\n", m.expectedPath, m.expectedPath)
		if m.hasBody {
			sb.WriteString("\t\tif ct := r.Header.Get(\"Content-Type\"); ct != \"application/json\" {\n\t\t\tt.Errorf(\"expected application/json, got %s\", ct)\n\t\t}\n")
		}
		sb.WriteString("\t\tw.Header().Set(\"Content-Type\", \"application/json\")\n")
		sb.WriteString("\t\tw.WriteHeader(http.StatusOK)\n")
		sb.WriteString("\t\t_, _ = w.Write([]byte(`{}`))\n")
		sb.WriteString("\t}))\n\tdefer server.Close()\n\n")
		fmt.Fprintf(&sb, "\tclient := New%s(server.URL, nil)\n", clientName)
		fmt.Fprintf(&sb, "\tresp, err := client.%s(%s)\n", m.funcName, callArgs)
		sb.WriteString("\tif err != nil {\n")
		fmt.Fprintf(&sb, "\t\tt.Fatalf(%q+\": unexpected error: %%v\", err)\n", m.funcName)
		sb.WriteString("\t}\n")
		sb.WriteString("\tif resp.StatusCode != http.StatusOK {\n")
		fmt.Fprintf(&sb, "\t\tt.Errorf(%q+\": expected 200, got %%d\", resp.StatusCode)\n", m.funcName)
		sb.WriteString("\t}\n}\n\n")
	}

	if len(tests) > 0 {
		first := tests[0]
		for _, m := range tests {
			if !isMutating(m.methodUpper) {
				first = m
				break
			}
		}
		callArgs := buildCallArgs(first)

		fmt.Fprintf(&sb, "func Test%s_RequestEditorIsCalled(t *testing.T) {\n", clientName)
		sb.WriteString("\tcalled := false\n")
		sb.WriteString("\tserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n")
		sb.WriteString("\t\tif r.Header.Get(\"X-Test-Editor\") != \"ok\" {\n\t\t\tt.Errorf(\"RequestEditor not invoked: missing X-Test-Editor header\")\n\t\t}\n")
		sb.WriteString("\t\tw.WriteHeader(http.StatusOK)\n\t\t_, _ = w.Write([]byte(`{}`))\n")
		sb.WriteString("\t}))\n\tdefer server.Close()\n\n")
		sb.WriteString("\teditor := func(ctx context.Context, req *http.Request) error {\n\t\tcalled = true\n\t\treq.Header.Set(\"X-Test-Editor\", \"ok\")\n\t\treturn nil\n\t}\n")
		fmt.Fprintf(&sb, "\tclient := New%s(server.URL, nil, editor)\n", clientName)
		fmt.Fprintf(&sb, "\t_, _ = client.%s(%s)\n", first.funcName, callArgs)
		sb.WriteString("\tif !called {\n\t\tt.Error(\"RequestEditor was not called\")\n\t}\n}\n\n")

		fmt.Fprintf(&sb, "func Test%s_HTTPErrorPopulatesResponse(t *testing.T) {\n", clientName)
		sb.WriteString("\tserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n")
		sb.WriteString("\t\tw.WriteHeader(http.StatusNotFound)\n\t\t_, _ = w.Write([]byte(`{\"error\":\"not found\"}`))\n")
		sb.WriteString("\t}))\n\tdefer server.Close()\n\n")
		fmt.Fprintf(&sb, "\tclient := New%s(server.URL, nil)\n", clientName)
		fmt.Fprintf(&sb, "\tresp, err := client.%s(%s)\n", first.funcName, callArgs)
		sb.WriteString("\tif err == nil {\n\t\tt.Error(\"expected error for HTTP 404, got nil\")\n\t}\n")
		sb.WriteString("\tif resp == nil {\n\t\tt.Fatal(\"Response must be non-nil even on error\")\n\t}\n")
		sb.WriteString("\tif resp.StatusCode != http.StatusNotFound {\n\t\tt.Errorf(\"expected 404, got %d\", resp.StatusCode)\n\t}\n}\n")
	}

	return sb.String()
}

func (sc *structCollector) buildStructFromSchema(structName string, schema map[string]any) string {
	props, ok := schema["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		code := fmt.Sprintf("type %s map[string]any", structName)
		sc.structs[structName] = code
		return structName
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "type %s struct {\n", structName)

	var propKeys []string
	for k := range props {
		propKeys = append(propKeys, k)
	}
	sort.Strings(propKeys)

	for _, k := range propKeys {
		pVal := props[k]
		pMap, ok := pVal.(map[string]any)
		if !ok {
			continue
		}

		fieldName := naming.ToCamelCase(k)
		goType := sc.mapSchemaToGoType(structName+fieldName, pMap)
		fmt.Fprintf(&sb, "\t%s %s `json:\"%s,omitempty\"`\n", fieldName, goType, k)
	}
	sb.WriteString("}")

	code := sb.String()
	sc.structs[structName] = code
	return structName
}

func (sc *structCollector) mapSchemaToGoType(parentField string, schema map[string]any) string {
	sType, _ := schema["type"].(string)

	switch sType {
	case "integer":
		return "int64"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "string":
		return "string"
	case "array":
		if items, ok := schema["items"].(map[string]any); ok {
			elemType := sc.mapSchemaToGoType(parentField+"Item", items)
			return "[]" + elemType
		}
		return "[]any"
	case "object":
		if props, ok := schema["properties"].(map[string]any); ok && len(props) > 0 {
			nestedName := sc.getUniqueName(parentField)
			sc.buildStructFromSchema(nestedName, schema)
			return nestedName
		}
		return "map[string]any"
	default:
		return "any"
	}
}

func isHTTPMethod(m string) bool {
	switch strings.ToUpper(m) {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "TRACE":
		return true
	default:
		return false
	}
}

func selectTypeName(val, fallback string) string {
	if val != "" {
		return val
	}
	return fallback
}

func detectPeerPackageName(outPath string) string {
	dir := filepath.Dir(outPath)
	if dir == "" {
		dir = "."
	}

	// 1. Peer .go package name
	entries, err := os.ReadDir(dir)
	if err == nil {
		outBase := filepath.Base(outPath)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || e.Name() == outBase || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			filePath := filepath.Join(dir, e.Name())
			if data, err := os.ReadFile(filePath); err == nil {
				content := string(data)
				for _, line := range strings.Split(content, "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "package ") {
						pkg := strings.TrimSpace(strings.TrimPrefix(line, "package "))
						if pkg != "" {
							return pkg
						}
					}
				}
			}
		}
	}

	// 2. Parent directory name
	if abs, err := filepath.Abs(outPath); err == nil {
		parentDir := filepath.Base(filepath.Dir(abs))
		parentDir = naming.SanitizePkgName(parentDir)
		if parentDir != "" && parentDir != "." && parentDir != "/" {
			return parentDir
		}
	}

	// 3. Root/CWD fallback ("main")
	absDir, err := filepath.Abs(dir)
	cwd, errCwd := os.Getwd()
	if err == nil && errCwd == nil && absDir == cwd {
		return "main"
	}

	// 4. Default fallback ("client")
	return "client"
}
