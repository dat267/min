// Package scaffold implements the `min cmd` family of commands: initializing
// projects and managing command structs via Go AST analysis.
package scaffold

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

	"github.com/dat267/min/internal/naming"
)

// AddCommand implements `min cmd add`: adds a command (dot-separated segments
// create nested command groups) to the cmd/ package of the current project.
// When group is true, the final segment is created as a bare command group
// (no Run method) instead of a leaf command.
func AddCommand(name, desc string, group bool) error {
	segments := strings.Split(name, ".")
	if len(segments) == 0 || segments[0] == "" {
		return fmt.Errorf("command name is required")
	}
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("invalid command name %q: empty segment", name)
		}
	}
	if _, err := os.Stat("cmd"); os.IsNotExist(err) {
		return fmt.Errorf("no cmd/ directory found (run 'min init' first)")
	}

	cliFile := DetectCLIFile()
	if cliFile == "" {
		return fmt.Errorf("could not find CLI struct in cmd/ (run 'min init' first)")
	}

	helpText := desc
	leaf := segments[len(segments)-1]
	if helpText == "" {
		if group {
			helpText = leaf + " command group"
		} else {
			helpText = leaf + " command"
		}
	}

	currentFile := cliFile
	currentStruct := "CLI"
	primaryGroupFile := filepath.Join("cmd", strings.ToLower(segments[0])+".go")

	for i, seg := range segments {
		isFinal := i == len(segments)-1
		isLeaf := isFinal && !group
		fieldName := naming.SanitizeFieldName(seg)
		structName := fieldName + "Cmd"

		targetFile := primaryGroupFile

		// Check if struct already exists in the target scopes
		exists := false

		// 1. Check current parent file (e.g. cmd.go)
		if found := FindStructInFile(currentFile, seg); found != "" {
			structName = found
			exists = true
			targetFile = currentFile
		} else if _, err := os.Stat(targetFile); err == nil {
			// 2. Check the primary group file (e.g. admin.go)
			if StructExistsInFile(targetFile, structName) {
				exists = true
			} else if found := FindStructInFile(targetFile, seg); found != "" {
				structName = found
				exists = true
			}
		}

		if exists {
			if isLeaf {
				return fmt.Errorf("command %q already exists in %s", name, targetFile)
			}
			if StructHasMethod(targetFile, structName, "Run") {
				if group {
					return fmt.Errorf("cannot add group %q: %s is already a leaf command", name, structName)
				}
				return fmt.Errorf("cannot add subcommands under %q: %s is already a leaf command", seg, structName)
			}
		} else {
			// We need to generate the struct
			if _, err := os.Stat(targetFile); os.IsNotExist(err) {
				if err := os.WriteFile(targetFile, []byte("package cmd\n"), 0644); err != nil {
					return fmt.Errorf("create file %s: %w", targetFile, err)
				}
			}

			if err := AppendStructToFile(targetFile, structName, isLeaf); err != nil {
				return fmt.Errorf("append struct to %s: %w", targetFile, err)
			}

			segHelp := helpText
			if !isFinal {
				segHelp = seg + " command group"
			}
			if err := RegisterField(currentFile, currentStruct, fieldName, structName, segHelp); err != nil {
				return fmt.Errorf("register field in %s: %w", currentFile, err)
			}
		}

		currentFile = targetFile
		currentStruct = structName
	}

	fmt.Printf("Successfully added command %q\n", name)
	return nil
}

// ShowCommands implements `min cmd show`: lists the command files in cmd/.
// Runtime files (runtime.go, config.go, root.go) and the CLI stub (cmd.go) are
// not commands.
func ShowCommands() error {
	entries, err := os.ReadDir("cmd")
	if err != nil {
		return fmt.Errorf("read cmd/: %w", err)
	}
	fmt.Println("Commands:")
	count := 0
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".go") && name != "root.go" && name != "cmd.go" &&
			name != "runtime.go" && name != "config.go" && !strings.HasSuffix(name, "_test.go") {
			fmt.Printf("  %s\n", strings.TrimSuffix(name, ".go"))
			count++
		}
	}
	if count == 0 {
		fmt.Println("  (no commands yet)")
	}
	return nil
}

// EditCommand implements `min cmd edit`: opens the file defining the named
// command in the default editor.
func EditCommand(name string) error {
	path := filepath.Join("cmd", strings.ToLower(name)+".go")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		segments := strings.Split(name, ".")
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
			return fmt.Errorf("command %q not found in cmd/", name)
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

// DetectCLIFile returns the path (relative to cwd) of the file that defines
// the CLI struct in the cmd/ directory, or "" when none is found.
func DetectCLIFile() string {
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

// FindStructInFile returns the name of a struct in filePath matching the
// command segment, or "" when none matches.
func FindStructInFile(filePath string, seg string) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return ""
	}

	prefix := naming.SanitizeFieldName(seg)
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

// StructHasMethod reports whether structName has a method named methodName
// defined in filePath.
func StructHasMethod(filePath, structName, methodName string) bool {
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

// StructExistsInFile reports whether a type named structName is declared in
// filePath.
func StructExistsInFile(filePath, structName string) bool {
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

// AppendStructToFile appends a new command struct (and a Run method for leaf
// commands) to the Go file at filePath.
func AppendStructToFile(filePath, structName string, isLeaf bool) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	content := string(data)

	if StructExistsInFile(filePath, structName) {
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
		content = EnsureImports(content, "fmt", "strings")
	} else {
		fmt.Fprintf(&sb, `type %[1]s struct {
}
`, structName)
	}

	content = strings.TrimRight(content, "\n\r\t ") + "\n" + sb.String()
	return os.WriteFile(filePath, []byte(content), 0644)
}

// RegisterField inserts a struct field registering a subcommand into the
// given parent struct.
func RegisterField(file, structName, fieldName, typeName, helpText string) error {
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

// EnsureImports adds the given stdlib import paths to content if they are not
// already present.
func EnsureImports(content string, imports ...string) string {
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
