package cmd

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// AstCmdGroup encapsulates all AST-related subcommands.
type AstCmdGroup struct {
	Scan  AstScanCmd  `cmd:"" help:"Scan Go files, strip bodies, keep signatures"`
	Types AstTypesCmd `cmd:"" help:"Show types only (structs, interfaces, func signatures)"`
	Fn    AstFnCmd    `cmd:"" help:"Extract context for a single function"`
}

// AstScanCmd scans Go files and prints top-level declarations (stripping function bodies).
type AstScanCmd struct {
	Path string `help:"File or directory to scan" arg:"" default:"."`
}

func (c *AstScanCmd) Run() error {
	return walkPath(c.Path, printScanned)
}

// AstTypesCmd scans Go files and prints only type declarations.
type AstTypesCmd struct {
	Path string `help:"File or directory to scan" arg:"" default:"."`
}

func (c *AstTypesCmd) Run() error {
	return walkPath(c.Path, printTypes)
}

// AstFnCmd extracts the context, references, and call sites for a specific function/method.
type AstFnCmd struct {
	Func string `arg:"" help:"Name of the function to search for (e.g. 'MyFunc' or 'MyStruct.MyMethod')."`
	Path string `arg:"" help:"Path to file or directory." default:"."`
	Type string `name:"type" help:"Optional receiver type name filter."`
}

type parsedFile struct {
	path    string
	fset    *token.FileSet
	file    *ast.File
	content []byte
}

type targetMatch struct {
	decl *ast.FuncDecl
	file *parsedFile
}

type helperInfo struct {
	decl    *ast.FuncDecl
	fset    *token.FileSet
	path    string
	content []byte
}

func (c *AstFnCmd) Run() error {
	targetName, targetType := c.Func, c.Type
	if idx := strings.LastIndex(c.Func, "."); idx != -1 {
		targetType = strings.Trim(c.Func[:idx], "()")
		targetName = c.Func[idx+1:]
	}

	// 1. Gather all target files efficiently
	var files []string
	if info, err := os.Stat(c.Path); err != nil {
		return fmt.Errorf("stat: %w", err)
	} else if info.IsDir() {
		entries, _ := os.ReadDir(c.Path)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				files = append(files, filepath.Join(c.Path, e.Name()))
			}
		}
	} else {
		files = []string{c.Path}
	}

	// 2. Parse files and find matching functions matching the criteria
	var parsed []*parsedFile
	var matches []targetMatch
	for _, p := range files {
		fset := token.NewFileSet()
		if f, err := parser.ParseFile(fset, p, nil, parser.ParseComments); err == nil {
			pf := &parsedFile{path: p, fset: fset, file: f, content: readFile(p)}
			parsed = append(parsed, pf)
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == targetName {
					if targetType == "" || receiverTypeName(fd) == targetType {
						matches = append(matches, targetMatch{decl: fd, file: pf})
					}
				}
			}
		}
	}

	// 3. Resolve ambiguity and strict filtering
	if len(matches) == 0 {
		return fmt.Errorf("function %q (type: %q) not found", targetName, targetType)
	}
	if len(matches) > 1 {
		var available []string
		for _, m := range matches {
			if recv := receiverTypeName(m.decl); recv != "" {
				available = append(available, fmt.Sprintf("%s.%s", recv, targetName))
			} else {
				available = append(available, targetName)
			}
		}
		return fmt.Errorf("ambiguous matches for %q: %s", targetName, strings.Join(available, ", "))
	}

	target := matches[0]
	recvType := receiverTypeName(target.decl)
	displayName := targetName
	if recvType != "" {
		displayName = fmt.Sprintf("(%s).%s", recvType, targetName)
	}

	// 4. Output: Target Function
	fmt.Printf("// === Target Function: %s ===\n// %s\n\n", displayName, target.file.path)
	nodeSource(target.file.content, target.file.fset, target.decl)
	fmt.Println()

	// 5. Output: Referenced Types
	fmt.Printf("// === Referenced Types ===\n\n")
	refNames := collectTypeRefs(target.decl)
	for _, h := range collectHelpers(target.decl, parsed) {
		for k := range collectTypeRefs(h.decl) {
			refNames[k] = true
		}
	}

	for _, pf := range parsed {
		for _, d := range pf.file.Decls {
			if gd, ok := d.(*ast.GenDecl); ok {
				for _, s := range gd.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok && refNames[ts.Name.Name] {
						fmt.Printf("// %s\n", pf.path)
						nodeSource(pf.content, pf.fset, gd)
					}
				}
			}
		}
	}
	fmt.Println()

	// 6. Output: Internal Helpers
	if helpers := collectHelpers(target.decl, parsed); len(helpers) > 0 {
		fmt.Printf("// === Internal Helpers (signatures only) ===\n\n")
		for _, h := range helpers {
			fmt.Printf("// %s\n", h.path)
			decl := *h.decl
			decl.Body = nil // Strip body for helpers
			nodeSource(h.content, h.fset, &decl)
		}
		fmt.Println()
	}

	// 7. Output: Call Sites
	fmt.Printf("// === Call Sites ===\n\n")
	for _, pf := range parsed {
		if pf.path != target.file.path {
			ast.Inspect(pf.file, func(n ast.Node) bool {
				if ce, ok := n.(*ast.CallExpr); ok && extractCalledName(ce.Fun) == targetName {
					fmt.Printf("// %s:%d\n", pf.path, pf.fset.Position(ce.Pos()).Line)
					nodeSurround(pf.content, pf.fset, ce, 2)
					fmt.Println()
				}
				return true
			})
		}
	}

	// 8. Output: Related Tests
	fmt.Printf("// === Related Tests ===\n\n")
	for _, pf := range parsed {
		if strings.HasSuffix(pf.path, "_test.go") {
			ast.Inspect(pf.file, func(n ast.Node) bool {
				if fd, ok := n.(*ast.FuncDecl); ok && strings.Contains(strings.ToLower(fd.Name.Name), strings.ToLower(targetName)) {
					fmt.Printf("// %s\n", pf.path)
					nodeSource(pf.content, pf.fset, fd)
				}
				return true
			})
		}
	}

	return nil
}

func readFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func receiverTypeName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	typeExpr := fd.Recv.List[0].Type
	if star, ok := typeExpr.(*ast.StarExpr); ok {
		typeExpr = star.X
	}
	if id, ok := typeExpr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// nodeSource prints the exact source code for an AST node directly from the file bytes.
func nodeSource(content []byte, fset *token.FileSet, n ast.Node) {
	start := fset.Position(n.Pos()).Offset
	end := fset.Position(n.End()).Offset
	if start < 0 || end > len(content) || start >= end {
		return
	}
	fmt.Println(string(content[start:end]))
}

// nodeSurround prints the node alongside N lines of surrounding context.
func nodeSurround(content []byte, fset *token.FileSet, n ast.Node, lines int) {
	startLine := fset.Position(n.Pos()).Line - lines
	if startLine < 1 {
		startLine = 1
	}
	endLine := fset.Position(n.End()).Line + lines
	allLines := strings.Split(string(content), "\n")
	if startLine > len(allLines) {
		return
	}
	if endLine > len(allLines) {
		endLine = len(allLines)
	}
	fmt.Println(strings.Join(allLines[startLine-1:endLine], "\n"))
}

func collectHelpers(fd *ast.FuncDecl, parsed []*parsedFile) []helperInfo {
	calls := map[string]bool{}
	ast.Inspect(fd, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := ce.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		calls[id.Name] = true
		return true
	})

	var helpers []helperInfo
	for _, pf := range parsed {
		for _, d := range pf.file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !calls[fn.Name.Name] {
				continue
			}
			helpers = append(helpers, helperInfo{decl: fn, fset: pf.fset, path: pf.path, content: pf.content})
		}
	}
	return helpers
}

func extractCalledName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.CallExpr:
		return extractCalledName(t.Fun)
	}
	return ""
}

func collectTypeRefs(fd *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}

	if recvName := receiverTypeName(fd); recvName != "" {
		names[recvName] = true
	}

	collect := func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Obj != nil {
			return true
		}
		if len(id.Name) == 0 || (id.Name[0] >= 'a' && id.Name[0] <= 'z') {
			return true
		}
		// Ignore standard Go built-in types
		switch id.Name {
		case "bool", "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"float32", "float64", "string", "byte", "rune",
			"error", "any", "true", "false", "nil":
			return true
		}
		names[id.Name] = true
		return true
	}

	if fd.Recv != nil {
		ast.Inspect(fd.Recv, collect)
	}
	if fd.Type.Params != nil {
		ast.Inspect(fd.Type.Params, collect)
	}
	if fd.Type.Results != nil {
		ast.Inspect(fd.Type.Results, collect)
	}
	return names
}

func walkPath(path string, fn func(*ast.File, *token.FileSet) error) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") {
				return err
			}
			return processFile(p, fn)
		})
	}
	return processFile(path, fn)
}

func processFile(path string, fn func(*ast.File, *token.FileSet) error) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	fmt.Printf("// %s\n", path)
	fmt.Printf("package %s\n\n", f.Name)
	return fn(f, fset)
}

// printNode safely prints any AST node as valid Go syntax.
func printNode(fset *token.FileSet, node any) {
	var buf bytes.Buffer
	err := printer.Fprint(&buf, fset, node)
	if err == nil {
		fmt.Println(buf.String())
	}
}

// printScanned outputs structures and function signatures (strips body).
func printScanned(f *ast.File, fset *token.FileSet) error {
	for _, d := range f.Decls {
		switch decl := d.(type) {
		case *ast.GenDecl:
			printNode(fset, decl)
			fmt.Println()
		case *ast.FuncDecl:
			// Clone the func decl and strip the body for signature-only printing
			copyDecl := *decl
			copyDecl.Body = nil
			printNode(fset, &copyDecl)
			fmt.Println()
		}
	}
	return nil
}

// printTypes outputs ONLY type declarations (structs, interfaces, typedefs).
func printTypes(f *ast.File, fset *token.FileSet) error {
	for _, d := range f.Decls {
		if gen, ok := d.(*ast.GenDecl); ok {
			hasType := false
			for _, spec := range gen.Specs {
				if _, isType := spec.(*ast.TypeSpec); isType {
					hasType = true
					break
				}
			}
			if hasType {
				printNode(fset, gen)
				fmt.Println()
			}
		}
	}
	return nil
}
