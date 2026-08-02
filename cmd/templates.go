package cmd

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed runtime.go config.go scaffold_templates/main.go.tmpl scaffold_templates/cmd.go.tmpl
var scaffoldTemplates embed.FS

type scaffoldData struct {
	AppName string
	Module  string
}

// writeScaffoldProject writes the files of a generated CLI project. The
// runtime and config files are byte-identical copies of the files this binary
// itself compiles, so the generated project cannot drift from min's runtime.
func writeScaffoldProject(dir, name, module string) error {
	data := scaffoldData{AppName: name, Module: module}

	render := func(tmplPath, dest string) error {
		raw, err := scaffoldTemplates.ReadFile(tmplPath)
		if err != nil {
			return err
		}
		t, err := template.New(filepath.Base(tmplPath)).Parse(string(raw))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			return err
		}
		return os.WriteFile(dest, buf.Bytes(), 0644)
	}

	cmdDir := filepath.Join(dir, "cmd")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		return err
	}

	if err := render("scaffold_templates/main.go.tmpl", filepath.Join(dir, "main.go")); err != nil {
		return err
	}
	if err := render("scaffold_templates/cmd.go.tmpl", filepath.Join(cmdDir, "cmd.go")); err != nil {
		return err
	}

	for _, f := range []string{"runtime.go", "config.go"} {
		raw, err := scaffoldTemplates.ReadFile(f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(cmdDir, f), raw, 0644); err != nil {
			return err
		}
	}
	return nil
}
