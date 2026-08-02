package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type CmdGroup struct {
	Init CmdInitCmd `cmd:"" help:"Initialize a new project"`
	Add  CmdAddCmd  `cmd:"" help:"Add a new command"`
	Show CmdShowCmd `cmd:"" help:"List all commands"`
	Edit CmdEditCmd `cmd:"" help:"Edit a command struct"`
}

type CmdInitCmd struct {
	Name   string `help:"Project name (or '.' for current directory)" arg:""`
	Module string `help:"Go module path" default:""`
}

func (c *CmdInitCmd) Run() error {
	name := c.Name
	if name == "" {
		return fmt.Errorf("project name is required")
	}

	var dir string
	if name == "." {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
		name = filepath.Base(wd)
		dir = "."
	} else {
		dir = name
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("directory %q already exists", dir)
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}

	module := c.Module
	if module == "" {
		module = name
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainTmpl(module)), 0644); err != nil {
		return fmt.Errorf("write main.go: %w", err)
	}

	_ = os.MkdirAll(filepath.Join(dir, "cmd"), 0755)
	if err := os.WriteFile(filepath.Join(dir, "cmd", "cmd.go"), []byte(cmdTmpl(name)), 0644); err != nil {
		return fmt.Errorf("write cmd/cmd.go: %w", err)
	}

	modCmd := exec.Command("go", "mod", "init", module)
	modCmd.Dir = dir
	if out, err := modCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod init: %w\n%s", err, out)
	}

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = dir
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("resolve dependencies: %w\n%s", err, out)
	}

	fmt.Printf("Created project %q in %s/\n", name, dir)
	fmt.Printf("  cd %s && go run .\n", dir)
	return nil
}

func mainTmpl(module string) string {
	return fmt.Sprintf(`package main

import (
	"context"

	"%s/cmd"
)

func main() {
	cmd.Execute(context.Background())
}
`, module)
}

func cmdTmpl(name string) string {
	return fmt.Sprintf(`package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
)

var cfgPath string

func init() {
	cfgPath = resolveConfigPath()
}

func resolveConfigPath() string {
	appName := "%s"
	envKey := strings.ToUpper(appName) + "_CONFIG_FILE"
	if cf := os.Getenv(envKey); cf != "" {
		return cf
	}
	localFile := appName + ".json"
	if _, err := os.Stat(localFile); err == nil {
		return localFile
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, appName, appName+".json")
	}
	return localFile
}

type CLI struct {
	ConfigFile string    `+"`"+`help:"Config file path" json:"-"`+"`"+`
	Greet      GreetCmd  `+"`"+`cmd:"" help:"Print a greeting"`+"`"+`
	Config     ConfigCmd `+"`"+`cmd:"" help:"Manage configuration"`+"`"+`
}

func SetConfigPath(p string) {
	if p == "" {
		cfgPath = resolveConfigPath()
	} else {
		cfgPath = p
	}
}

func CfgPath() string { return cfgPath }

func Execute(ctx context.Context) {
	app := &CLI{}

	if cf := resolveConfigFileFlag(); cf != "" {
		SetConfigPath(cf)
	}
	activeConfig := CfgPath()

	options := []kong.Option{
		kong.Name("%[1]s"),
		kong.Description("A CLI application"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.BindTo(ctx, (*context.Context)(nil)),
	}

	if f, err := os.Open(activeConfig); err == nil {
		if resolver, err := JSONResolver(f); err == nil {
			options = append(options, kong.Resolvers(resolver))
		}
		_ = f.Close()
	}

	k, err := kong.New(app, options...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %%v\n", err)
		os.Exit(1)
	}

	kongCtx, err := k.Parse(os.Args[1:])
	k.FatalIfErrorf(err)

	SetConfigPath(app.ConfigFile)
	k.FatalIfErrorf(kongCtx.Run())
}

func resolveConfigFileFlag() string {
	for i, arg := range os.Args {
		if arg == "--config-file" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "--config-file=") {
			parts := strings.SplitN(arg, "=", 2)
			return parts[1]
		}
	}
	return ""
}

// JSONResolver builds a Kong resolver capable of loading both flat and nested JSON configuration.
func JSONResolver(r io.Reader) (kong.Resolver, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	flat := make(map[string]any)

	var flattenNested func(prefix string, m map[string]any)
	flattenNested = func(prefix string, m map[string]any) {
		for k, v := range m {
			key := k
			if prefix != "" {
				key = prefix + "-" + k
			}
			if sub, ok := v.(map[string]any); ok {
				flattenNested(key, sub)
			} else if prefix != "" {
				flat[key] = v
			}
		}
	}
	flattenNested("", raw)

	for k, v := range raw {
		if _, isMap := v.(map[string]any); !isMap {
			flat[k] = v
		}
	}

	return kong.ResolverFunc(func(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		if val, ok := flat[flag.Name]; ok {
			return val, nil
		}
		return nil, nil
	}), nil
}


type GreetCmd struct {
	Name  string `+"`"+`help:"Name to greet" arg:"" default:"World"`+"`"+`
	Count int    `+"`"+`help:"Repeat count" default:"1"`+"`"+`
	Shout bool   `+"`"+`help:"Shout" short:"s"`+"`"+`
}

func (c *GreetCmd) Run() error {
	for i := 0; i < c.Count; i++ {
		msg := fmt.Sprintf("Hello, %%s!", c.Name)
		if c.Shout {
			msg = strings.ToUpper(msg)
		}
		fmt.Println(msg)
	}
	return nil
}

type ConfigCmd struct {
	Init  ConfigInitCmd  `+"`"+`cmd:"" help:"Generate a default configuration file"`+"`"+`
	Path  ConfigPathCmd  `+"`"+`cmd:"" help:"Show configuration file path"`+"`"+`
	Show  ConfigShowCmd  `+"`"+`cmd:"" help:"Print current configuration values"`+"`"+`
	Set   ConfigSetCmd   `+"`"+`cmd:"" help:"Set configuration key"`+"`"+`
	Unset ConfigUnsetCmd `+"`"+`cmd:"" help:"Unset configuration key"`+"`"+`
	Edit  ConfigEditCmd  `+"`"+`cmd:"" help:"Edit configuration file"`+"`"+`
}

type ConfigInitCmd struct {
	Overwrite bool `+"`"+`help:"Overwrite existing configuration file"`+"`"+`
}

func (c *ConfigInitCmd) Run() error {
	p := CfgPath()
	if _, err := os.Stat(p); err == nil && !c.Overwrite {
		return fmt.Errorf("configuration file already exists at %%s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("failed to create configuration directory: %%w", err)
	}
	data, err := json.MarshalIndent(map[string]any{}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %%w", err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %%w", err)
	}
	fmt.Printf("Configuration file created at %%s\n", p)
	return nil
}

type ConfigPathCmd struct{}

func (c *ConfigPathCmd) Run() error {
	p := CfgPath()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		fmt.Printf("%%s (does not exist)\n", p)
		return nil
	}
	fmt.Println(p)
	return nil
}

type ConfigShowCmd struct{}

func (c *ConfigShowCmd) Run() error {
	p := CfgPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("%%s (does not exist)\n", p)
			return nil
		}
		return fmt.Errorf("failed to read configuration file: %%w", err)
	}
	fmt.Println(string(data))
	return nil
}

type ConfigSetCmd struct {
	Key   string `+"`"+`help:"Configuration key" arg:""`+"`"+`
	Value string `+"`"+`help:"Configuration value" arg:""`+"`"+`
}

func (c *ConfigSetCmd) Run() error {
	p := CfgPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %%w", err)
	}
	var raw map[string]any
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &raw)
	}
	if raw == nil {
		raw = make(map[string]any)
	}

	parts := strings.Split(c.Key, ".")
	curr := raw
	for i, part := range parts {
		if i == len(parts)-1 {
			curr[part] = c.Value
			break
		}
		next, ok := curr[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			curr[part] = next
		}
		curr = next
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %%w", err)
	}
	return os.WriteFile(p, data, 0644)
}

type ConfigUnsetCmd struct {
	Key string `+"`"+`help:"Configuration key" arg:""`+"`"+`
}

func (c *ConfigUnsetCmd) Run() error {
	p := CfgPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read configuration file: %%w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %%w", err)
	}

	parts := strings.Split(c.Key, ".")
	var unsetFunc func(m map[string]any, keys []string) bool
	unsetFunc = func(m map[string]any, keys []string) bool {
		if len(keys) == 0 {
			return false
		}
		if len(keys) == 1 {
			delete(m, keys[0])
			return len(m) == 0
		}
		if sub, ok := m[keys[0]].(map[string]any); ok {
			if unsetFunc(sub, keys[1:]) {
				delete(m, keys[0])
			}
		}
		return len(m) == 0
	}
	unsetFunc(raw, parts)

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %%w", err)
	}
	return os.WriteFile(p, out, 0644)
}

type ConfigEditCmd struct{}

func (c *ConfigEditCmd) Run() error {
	p := CfgPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %%w", err)
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		if err := os.WriteFile(p, []byte("{}\n"), 0644); err != nil {
			return fmt.Errorf("failed to write default config: %%w", err)
		}
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	ecmd := exec.Command(editor, p)
	ecmd.Stdin = os.Stdin
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr
	return ecmd.Run()
}
`, name)
}

type CmdAddCmd struct {
	Name string `help:"Command name (use dots for nesting, e.g. admin.users.create)" arg:""`
	Desc string `help:"Description for the command"`
}

func (c *CmdAddCmd) Run() error {
	segments := strings.Split(c.Name, ".")
	if len(segments) == 0 || segments[0] == "" {
		return fmt.Errorf("command name is required")
	}
	if _, err := os.Stat("cmd"); os.IsNotExist(err) {
		return fmt.Errorf("no cmd/ directory found (run 'min init' first)")
	}

	cliFile := detectCLIFile()
	if cliFile == "" {
		return fmt.Errorf("could not find CLI struct in cmd/ (run 'min init' first)")
	}

	helpText := c.Desc
	leaf := segments[len(segments)-1]
	if helpText == "" {
		helpText = leaf + " command"
	}

	currentFile := cliFile
	currentStruct := "CLI"
	primaryGroupFile := filepath.Join("cmd", strings.ToLower(segments[0])+".go")

	for i, seg := range segments {
		isLeaf := i == len(segments)-1
		fieldName := title(seg)
		structName := fieldName + "Cmd"

		targetFile := primaryGroupFile

		// Check if struct already exists in the target scopes
		exists := false

		// 1. Check current parent file (e.g. cmd.go)
		if found := findStructInFile(currentFile, seg); found != "" {
			structName = found
			exists = true
			targetFile = currentFile
		} else if _, err := os.Stat(targetFile); err == nil {
			// 2. Check the primary group file (e.g. admin.go)
			if structExistsInFile(targetFile, structName) {
				exists = true
			} else if found := findStructInFile(targetFile, seg); found != "" {
				structName = found
				exists = true
			}
		}

		if exists {
			if isLeaf {
				return fmt.Errorf("command %q already exists in %s", c.Name, targetFile)
			}
			if structHasMethod(targetFile, structName, "Run") {
				return fmt.Errorf("cannot add subcommands under %q: %s is already a leaf command", seg, structName)
			}
		} else {
			// We need to generate the struct
			if _, err := os.Stat(targetFile); os.IsNotExist(err) {
				if err := os.WriteFile(targetFile, []byte("package cmd\n"), 0644); err != nil {
					return fmt.Errorf("create file %s: %w", targetFile, err)
				}
			}

			if err := appendStructToFile(targetFile, structName, isLeaf); err != nil {
				return fmt.Errorf("append struct to %s: %w", targetFile, err)
			}

			segHelp := helpText
			if !isLeaf {
				segHelp = seg + " command group"
			}
			if err := registerField(currentFile, currentStruct, fieldName, structName, segHelp); err != nil {
				return fmt.Errorf("register field in %s: %w", currentFile, err)
			}
		}

		currentFile = targetFile
		currentStruct = structName
	}

	fmt.Printf("Successfully added command %q\n", c.Name)
	return nil
}

type CmdShowCmd struct{}

func (c *CmdShowCmd) Run() error {
	entries, err := os.ReadDir("cmd")
	if err != nil {
		return fmt.Errorf("read cmd/: %w", err)
	}
	fmt.Println("Commands:")
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") && e.Name() != "root.go" && e.Name() != "cmd.go" && !strings.HasSuffix(e.Name(), "_test.go") {
			name := strings.TrimSuffix(e.Name(), ".go")
			fmt.Printf("  %s\n", name)
			count++
		}
	}
	if count == 0 {
		fmt.Println("  (no commands yet)")
	}
	return nil
}

type CmdEditCmd struct {
	Name string `help:"Command name" arg:""`
}

func (c *CmdEditCmd) Run() error {
	path := filepath.Join("cmd", strings.ToLower(c.Name)+".go")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		segments := strings.Split(c.Name, ".")
		found := false
		for i := len(segments) - 1; i >= 0; i-- {
			p := filepath.Join("cmd", strings.ToLower(segments[i])+".go")
			if _, err := os.Stat(p); err == nil {
				path = p
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("command %q not found in cmd/", c.Name)
		}
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	ecmd := exec.Command(editor, path)
	ecmd.Stdin = os.Stdin
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr
	return ecmd.Run()
}

func detectCLIFile() string {
	entries, err := os.ReadDir("cmd")
	if err != nil {
		return ""
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join("cmd", e.Name())
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if ts.Name.Name == "CLI" {
					if _, ok := ts.Type.(*ast.StructType); ok {
						return path
					}
				}
			}
		}
	}
	return ""
}

func findStructInFile(filePath string, seg string) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return ""
	}

	prefix := title(seg)
	candidates := []string{
		prefix + "Cmd",
		prefix + "CmdGroup",
		prefix + "Group",
		prefix,
	}

	structs := make(map[string]bool)

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, isStruct := ts.Type.(*ast.StructType); isStruct {
				name := ts.Name.Name
				structs[name] = true
			}
		}
	}

	for _, c := range candidates {
		if structs[c] {
			return c
		}
	}

	// Fall back to structs whose name continues from the segment at a
	// camelCase word boundary (e.g. "Admin" matches "AdminCommands" but not
	// "Administrator"). Sort for deterministic results.
	var matches []string
	for name := range structs {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := name[len(prefix):]
		if rest == "" {
			continue
		}
		if r, _ := utf8.DecodeRuneInString(rest); unicode.IsUpper(r) {
			matches = append(matches, name)
		}
	}
	if len(matches) > 0 {
		sort.Strings(matches)
		return matches[0]
	}

	return ""
}

func structHasMethod(filePath, structName, methodName string) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return false
	}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != methodName || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		recv := fd.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			recv = star.X
		}
		if id, ok := recv.(*ast.Ident); ok && id.Name == structName {
			return true
		}
	}
	return false
}

func structExistsInFile(filePath, structName string) bool {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, data, 0)
	if err != nil {
		return strings.Contains(string(data), "type "+structName+" struct")
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == structName {
				return true
			}
		}
	}
	return false
}

func appendStructToFile(filePath, structName string, isLeaf bool) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	content := string(data)

	if structExistsInFile(filePath, structName) {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("\n")
	if isLeaf {
		fmt.Fprintf(&sb, `type %[1]s struct {
	Name  string `+"`"+`help:"Name" arg:""`+"`"+`
	Count int    `+"`"+`help:"Repeat count" default:"1"`+"`"+`
	Shout bool   `+"`"+`help:"Shout" short:"s"`+"`"+`
}

func (c *%[1]s) Run() error {
	for i := 0; i < c.Count; i++ {
		msg := fmt.Sprintf("Hello, %%s!", c.Name)
		if c.Shout {
			msg = strings.ToUpper(msg)
		}
		fmt.Println(msg)
	}
	return nil
}
`, structName)
		content = ensureImports(content, "fmt", "strings")
	} else {
		fmt.Fprintf(&sb, `type %[1]s struct {
}
`, structName)
	}

	content = strings.TrimRight(content, "\n\r\t ") + "\n" + sb.String()
	return os.WriteFile(filePath, []byte(content), 0644)
}

func registerField(file, structName, fieldName, typeName, helpText string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	content := string(data)

	if strings.Contains(content, fieldName+" "+typeName) {
		return nil
	}

	target := "type " + structName + " struct"
	idx := strings.Index(content, target)
	if idx < 0 {
		return fmt.Errorf("struct %q not found in %s", structName, file)
	}

	braceIdx := strings.Index(content[idx:], "{")
	if braceIdx < 0 {
		return fmt.Errorf("invalid struct %q in %s", structName, file)
	}

	insertPos := idx + braceIdx + 1
	if insertPos < len(content) && content[insertPos] == '\n' {
		insertPos++
	}

	tag := `cmd:""`
	helpText = strings.ReplaceAll(helpText, "`", "'")
	helpText = strings.ReplaceAll(helpText, "\r", " ")
	helpText = strings.ReplaceAll(helpText, "\n", " ")
	helpText = strings.ReplaceAll(helpText, `"`, `\"`)
	line := fmt.Sprintf("\t%s %s `%s help:\"%s\"`\n", fieldName, typeName, tag, helpText)
	out := content[:insertPos] + line + content[insertPos:]
	return os.WriteFile(file, []byte(out), 0644)
}

func ensureImports(content string, imports ...string) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", content, parser.ImportsOnly)
	if err != nil {
		return content
	}

	existing := make(map[string]bool)
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		existing[path] = true
	}

	var missing []string
	for _, imp := range imports {
		if !existing[imp] {
			missing = append(missing, imp)
		}
	}

	if len(missing) == 0 {
		return content
	}

	if importIdx := strings.Index(content, "import ("); importIdx >= 0 {
		insertPos := importIdx + len("import (\n")
		var sb strings.Builder
		for _, m := range missing {
			fmt.Fprintf(&sb, "\t%q\n", m)
		}
		return content[:insertPos] + sb.String() + content[insertPos:]
	}

	if packageIdx := strings.Index(content, "package "); packageIdx >= 0 {
		if nl := strings.IndexByte(content[packageIdx:], '\n'); nl >= 0 {
			insertPos := packageIdx + nl + 1
			var sb strings.Builder
			sb.WriteString("\nimport (\n")
			for _, m := range missing {
				fmt.Fprintf(&sb, "\t%q\n", m)
			}
			sb.WriteString(")\n")
			return content[:insertPos] + sb.String() + content[insertPos:]
		}
	}

	return content
}

func title(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
