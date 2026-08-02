// Package astscan implements the `min ast` family of commands: scanning Go
// files, printing type declarations, and extracting context for a single
// function.
package astscan

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// testOut overrides the output writer in tests. When unset, output goes to
// os.Stdout, resolved on every call so tests that swap the global os.Stdout
// (see cmd/testutil_test.go's captureStdout) observe the new writer.
var testOut io.Writer

func out() io.Writer {
	if testOut != nil {
		return testOut
	}
	return os.Stdout
}

func wemitf(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
func wemitln(w io.Writer, args ...any)               { _, _ = fmt.Fprintln(w, args...) }

func emitf(format string, args ...any) { wemitf(out(), format, args...) }
func emitln(args ...any)               { wemitln(out(), args...) }
func emit(s string)                    { _, _ = fmt.Fprint(out(), s) }

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

// Scan prints top-level declarations of the Go file or directory at path,
// stripping function bodies.
func Scan(path string) error {
	return walkPath(path, printScanned)
}

// Types prints only type declarations of the Go file or directory at path.
func Types(path string) error {
	return walkPath(path, printTypes)
}

// Fn extracts the context, references, and call sites for a function or
// method matching target, optionally filtered by receiver typeFilter.
func Fn(target, path, typeFilter string) error {
	targetName, targetType := target, typeFilter
	if idx := strings.LastIndex(target, "."); idx != -1 {
		targetType = strings.Trim(target[:idx], "()")
		targetName = target[idx+1:]
	}

	// 1. Gather all target files
	var files []string
	if info, err := os.Stat(path); err != nil {
		return fmt.Errorf("stat: %w", err)
	} else if info.IsDir() {
		entries, _ := os.ReadDir(path)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				files = append(files, filepath.Join(path, e.Name()))
			}
		}
	} else {
		files = []string{path}
	}

	// 2. Parse each file once and find matching functions
	var parsed []*parsedFile
	var matches []targetMatch
	for _, p := range files {
		fset := token.NewFileSet()
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if f, err := parser.ParseFile(fset, p, content, parser.ParseComments); err == nil {
			pf := &parsedFile{path: p, fset: fset, file: f, content: content}
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

	match := matches[0]
	recvType := receiverTypeName(match.decl)
	displayName := targetName
	if recvType != "" {
		displayName = fmt.Sprintf("(%s).%s", recvType, targetName)
	}

	// 4. Output: Target Function
	emitf("// === Target Function: %s ===\n// %s\n\n", displayName, match.file.path)
	nodeSource(match.file.content, match.file.fset, match.decl)
	emitln()

	// 5. Output: Referenced Types
	emitf("// === Referenced Types ===\n\n")
	helpers := collectHelpers(match.decl, parsed)
	refNames := collectTypeRefs(match.decl)
	for _, h := range helpers {
		for k := range collectTypeRefs(h.decl) {
			refNames[k] = true
		}
	}

	for _, pf := range parsed {
		for _, d := range pf.file.Decls {
			if gd, ok := d.(*ast.GenDecl); ok {
				for _, s := range gd.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok && refNames[ts.Name.Name] {
						emitf("// %s\n", pf.path)
						nodeSource(pf.content, pf.fset, gd)
					}
				}
			}
		}
	}
	emitln()

	// 6. Output: Internal Helpers (signatures only)
	if len(helpers) > 0 {
		emitf("// === Internal Helpers (signatures only) ===\n\n")
		for _, h := range helpers {
			emitf("// %s\n", h.path)
			decl := *h.decl
			decl.Body = nil // Strip body for helpers
			nodeSource(h.content, h.fset, &decl)
		}
		emitln()
	}

	// 7. Output: Call Sites
	emitf("// === Call Sites ===\n\n")
	for _, pf := range parsed {
		if pf.path != match.file.path {
			ast.Inspect(pf.file, func(n ast.Node) bool {
				if ce, ok := n.(*ast.CallExpr); ok && extractCalledName(ce.Fun) == targetName {
					emitf("// %s:%d\n", pf.path, pf.fset.Position(ce.Pos()).Line)
					nodeSurround(pf.content, pf.fset, ce, 2)
					emitln()
				}
				return true
			})
		}
	}

	// 8. Output: Related Tests
	emitf("// === Related Tests ===\n\n")
	for _, pf := range parsed {
		if strings.HasSuffix(pf.path, "_test.go") {
			ast.Inspect(pf.file, func(n ast.Node) bool {
				if fd, ok := n.(*ast.FuncDecl); ok && strings.Contains(strings.ToLower(fd.Name.Name), strings.ToLower(targetName)) {
					emitf("// %s\n", pf.path)
					nodeSource(pf.content, pf.fset, fd)
				}
				return true
			})
		}
	}

	return nil
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
	emitln(string(content[start:end]))
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
	emitln(strings.Join(allLines[startLine-1:endLine], "\n"))
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

type fileFunc func(w io.Writer, f *ast.File, fset *token.FileSet) error

func walkPath(path string, fn fileFunc) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		var files []string
		if err := filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") {
				return err
			}
			files = append(files, p)
			return nil
		}); err != nil {
			return err
		}
		return processFilesParallel(files, fn)
	}
	return processFile(out(), path, fn)
}

// processFilesParallel parses and prints files concurrently, buffering each
// file's output so results are emitted in the original order.
func processFilesParallel(files []string, fn fileFunc) error {
	if len(files) == 0 {
		return nil
	}
	type result struct {
		out string
		err error
	}
	results := make([]result, len(files))

	workers := 8
	if len(files) < workers {
		workers = len(files)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				var buf bytes.Buffer
				err := processFile(&buf, files[idx], fn)
				results[idx] = result{out: buf.String(), err: err}
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			return r.err
		}
		emit(r.out)
	}
	return nil
}

func processFile(w io.Writer, path string, fn fileFunc) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	wemitf(w, "// %s\n", path)
	wemitf(w, "package %s\n\n", f.Name)
	return fn(w, f, fset)
}

// printNode safely prints any AST node as valid Go syntax.
func printNode(w io.Writer, fset *token.FileSet, node any) {
	var buf bytes.Buffer
	err := printer.Fprint(&buf, fset, node)
	if err == nil {
		wemitln(w, buf.String())
	}
}

// printScanned outputs structures and function signatures (strips body).
func printScanned(w io.Writer, f *ast.File, fset *token.FileSet) error {
	for _, d := range f.Decls {
		switch decl := d.(type) {
		case *ast.GenDecl:
			printNode(w, fset, decl)
			wemitln(w)
		case *ast.FuncDecl:
			// Clone the func decl and strip the body for signature-only printing
			copyDecl := *decl
			copyDecl.Body = nil
			printNode(w, fset, &copyDecl)
			wemitln(w)
		}
	}
	return nil
}

// printTypes outputs ONLY type declarations (structs, interfaces, typedefs).
func printTypes(w io.Writer, f *ast.File, fset *token.FileSet) error {
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
				printNode(w, fset, gen)
				wemitln(w)
			}
		}
	}
	return nil
}
