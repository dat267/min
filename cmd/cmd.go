package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CmdGroup struct {
	Add  CmdAddCmd  `cmd:"" help:"Add a new command"`
	Show CmdShowCmd `cmd:"" help:"List all commands"`
	Edit CmdEditCmd `cmd:"" help:"Edit a command struct"`
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

	helpText := c.Desc
	leaf := segments[len(segments)-1]
	if helpText == "" {
		helpText = leaf + " command"
	}

	parentStruct := "CLI"
	parentFile := detectCLIFile()
	if parentFile == "" {
		return fmt.Errorf("could not find CLI struct in cmd/ (run 'min init' first)")
	}

	// outputPath is the single file where all new structs will be written.
	// It is set to the first segment whose file doesn't exist yet.
	var outputPath string

	// Collect structs that need to be written to outputPath (new intermediate
	// parents whose own file doesn't exist yet, plus the leaf itself).
	type pendingStruct struct {
		name   string // struct name, e.g. "DevCmd"
		isLeaf bool
	}
	var pending []pendingStruct

	// First pass: validate and figure out which segments need new files.
	currentParentStruct := parentStruct
	currentParentFile := parentFile
	type registration struct {
		file, parentStruct, field, typeName, help string
	}
	var registrations []registration

	for i, seg := range segments {
		isLeaf := i == len(segments)-1
		path := filepath.Join("cmd", strings.ToLower(seg)+".go")
		fieldName := title(seg)
		structName := fieldName + "Cmd"

		if _, err := os.Stat(path); os.IsNotExist(err) {
			// First new segment determines the output filename.
			if outputPath == "" {
				outputPath = path
			}
			pending = append(pending, pendingStruct{name: structName, isLeaf: isLeaf})
			registrations = append(registrations, registration{
				file:         currentParentFile,
				parentStruct: currentParentStruct,
				field:        fieldName,
				typeName:     structName,
				help:         helpText,
			})
			currentParentStruct = structName
			currentParentFile = outputPath
		} else {
			if isLeaf {
				return fmt.Errorf("command %q already exists", seg)
			}
			existing := detectStructName(path)
			if existing == "" {
				return fmt.Errorf("could not find struct in %s", path)
			}
			currentParentStruct = existing
			currentParentFile = path
		}
	}

	// Build the single file content for all pending structs.
	var sb strings.Builder
	sb.WriteString("package cmd\n")

	// Only the leaf gets the full import + Run boilerplate; intermediate
	// group structs are plain empty structs.
	hasLeaf := false
	for _, p := range pending {
		if p.isLeaf {
			hasLeaf = true
		}
	}
	if hasLeaf {
		sb.WriteString(`
import (
	"fmt"
	"strings"
)
`)
	}

	for _, p := range pending {
		if p.isLeaf {
			sb.WriteString(fmt.Sprintf(`
type %[1]s struct {
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
`, p.name))
		} else {
			sb.WriteString(fmt.Sprintf(`
type %[1]s struct {
}
`, p.name))
		}
	}

	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}
	fmt.Printf("Created %s\n", outputPath)

	// Register fields in parent structs (existing files get updated in place;
	// new structs that live in leafPath don't need separate file writes).
	for _, reg := range registrations {
		if err := registerField(reg.file, reg.parentStruct, reg.field, reg.typeName, reg.help); err != nil {
			return fmt.Errorf("register in %s: %w", reg.file, err)
		}
	}

	return nil
}

func detectStructName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)
	idx := strings.Index(content, "type ")
	if idx < 0 {
		return ""
	}
	rest := content[idx+5:]
	end := strings.Index(rest, " struct")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func registerField(file, structName, fieldName, typeName, helpText string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	content := string(data)

	target := "type " + structName + " struct {\n"
	idx := strings.Index(content, target)
	if idx < 0 {
		return fmt.Errorf("struct %q not found in %s", structName, file)
	}
	idx += len(target)

	tag := `cmd:""`
	line := fmt.Sprintf("\t%s %s `%s help:\"%s\"`\n", fieldName, typeName, tag, helpText)
	out := content[:idx] + line + content[idx:]
	return os.WriteFile(file, []byte(out), 0644)
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
		return fmt.Errorf("command %q not found in cmd/", c.Name)
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
					_, ok := ts.Type.(*ast.StructType)
					if ok {
						return path
					}
				}
			}
		}
	}
	return ""
}

func title(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
