package gen_go_v2

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/template"

	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
)

func public(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func goEscape(n string) string {
	if g, h := goKeywords[n]; h {
		return g
	}
	return n
}

var (
	//go:embed templates/*
	templateFS embed.FS

	templates = template.Must(
		template.
			New("").
			Funcs(template.FuncMap{
				"public": public,
				"lower": func(s string) string {
					return strings.ToLower(s)
				},
				"goEscape": goEscape,
			}).
			ParseFS(templateFS, "templates/*"))
)

type templateRenderer struct {
	buf bytes.Buffer
	t   *template.Template
}

// Render calls ExecuteTemplate to render to its buffer.
func (tr *templateRenderer) Render(params any) {
	// Derive the template name from the name of the underlying type of
	// params:
	typeName := reflect.TypeOf(params).Name()
	name := typeName[:len(typeName)-len("Params")]
	err := tr.t.ExecuteTemplate(&tr.buf, name, params)
	if err != nil {
		data, _ := json.Marshal(params)
		fmt.Fprintf(os.Stderr, "error executing template -- template: %s\nerror: %v\n%s", name, err, data)
		panic(err)
	}
	// return nil
}

func (tr *templateRenderer) RenderTemplate(name string, params any) error {
	return tr.t.ExecuteTemplate(&tr.buf, name, params)
}

// Bytes returns the accumulated bytes.
func (tr *templateRenderer) Bytes() []byte {
	return tr.buf.Bytes()
}

type scopedDeclParams struct {
	G          *generator
	ModuleName string
	Name       string
	TypeParams typeParam
	Decl       adlast.Decl
	Fields     []fieldParams
}

type texprParams scopedDeclParams

type headerParams struct {
	Pkg string
}

type importsParams struct {
	// Rt      string
	Imports []importSpec
}

type structParams struct {
	G          *generator
	Name       string
	TypeParams typeParam
	Fields     []fieldParams
}

type unionParams struct {
	G          *generator
	Name       string
	TypeParams typeParam
	Branches   []fieldParams
}

type fieldParams struct {
	adlast.Field
	// Name           string
	// SerializedName string
	// Type           goTypeExpr
	// TypeParams typeParam
	HasDefault bool
	Just       any
}

type typeAliasParams struct {
	G          *generator
	Name       string
	TypeParams typeParam
	TypeExpr   adlast.TypeExpr
}
type newTypeParams typeAliasParams
