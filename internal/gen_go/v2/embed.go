package gen_go_v2

import (
	"bytes"
	"embed"
	"reflect"
	"text/template"
)

var (
	//go:embed templates/*
	templateFS embed.FS

	templates = template.Must(template.New("").Funcs(template.FuncMap{}).ParseFS(templateFS, "templates/*"))
)

type templateRenderer struct {
	buf bytes.Buffer
	t   *template.Template
}

// Render calls ExecuteTemplate to render to its buffer.
func (tr *templateRenderer) Render(params any) error {
	// Derive the template name from the name of the underlying type of
	// params:
	typeName := reflect.TypeOf(params).Name()
	name := typeName[:len(typeName)-len("Params")]
	return tr.t.ExecuteTemplate(&tr.buf, name, params)
}

func (tr *templateRenderer) RenderTemplate(name string, params any) error {
	return tr.t.ExecuteTemplate(&tr.buf, name, params)
}

// Bytes returns the accumulated bytes.
func (tr *templateRenderer) Bytes() []byte {
	return tr.buf.Bytes()
}

type headerParams struct {
	Pkg string
}

type importsParams struct {
	Rt      string
	Imports []importSpec
}
