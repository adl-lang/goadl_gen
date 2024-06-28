package goapi

import (
	"embed"
	"strings"
	"text/template"

	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/cli/gogen"
)

func public(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
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
				// "goEscape": goEscape,
			}).
			ParseFS(templateFS, "templates/*"))
)

type serviceParams struct {
	G     *gogen.Generator
	Name  string
	IsCap bool
}

type registerParams struct {
	G           *gogen.Generator
	Name        string
	IsCap       bool
	Annotations adlast.Annotations
	V           *adlast.TypeExpr
	CapApis     []tkid
}

type postParams struct {
	G           *gogen.Generator
	Name        string
	Annotations adlast.Annotations
	Req         adlast.TypeExpr
	Resp        adlast.TypeExpr
	IsCap       bool
}

type getParams struct {
	G           *gogen.Generator
	Name        string
	Annotations adlast.Annotations
	Resp        adlast.TypeExpr
	IsCap       bool
}

type regpostParams struct {
	G     *gogen.Generator
	Name  string
	IsCap bool
}
type reggetParams regpostParams

type regcapapiParams struct {
	G          *gogen.Generator
	StructName string
	Name       string
	Kids       []tkid
}

type tkid struct {
	Name  string
	Field *adlast.Field
}

type getcapapiParams struct {
	G           *gogen.Generator
	Name        string
	StructName  string
	Annotations adlast.Annotations
	C           adlast.TypeExpr
	S           adlast.TypeExpr
}
