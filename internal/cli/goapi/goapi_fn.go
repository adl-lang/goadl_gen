package goapi

import (
	"fmt"

	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/cli/gogen"
	"github.com/adl-lang/goadlc/internal/cli/goimports"
	"github.com/adl-lang/goadlc/internal/cli/loader"
	"github.com/samber/lo"
)

func (in *GoApi) Run() error {
	mod, ok := in.Loader.CombinedAst[in.ApiStruct.ModuleName]
	if !ok {
		return fmt.Errorf("Module not found '%s", in.ApiStruct.ModuleName)
	}
	decl, ok := mod.Decls[in.ApiStruct.Name]
	if !ok {
		return fmt.Errorf("Decl not found '%s", in.ApiStruct.Name)
	}
	st, ok := decl.Type_.Cast_struct_()
	if !ok || len(st.TypeParams) != 0 {
		return fmt.Errorf("Unexpected - apiRequests is not a monomorphic struct")
	}
	apiSt := adlast.Make_Struct(
		[]string{},
		lo.Map[adlast.Field](st.Fields, func(f adlast.Field, _ int) adlast.Field {
			te := ExpandTypeAliases(in.Loader, f.TypeExpr)
			return adlast.MakeAll_Field(
				f.Name, f.SerializedName, te, f.Default, f.Annotations,
			)
		}),
	)
	midPath, err := goimports.MidPath(in.Outputdir, in.GoMod.RootDir)
	if err != nil {
		return err
	}
	base := gogen.NewBaseGen(
		in.GoMod.ModulePath,
		midPath,
		in.ApiStruct.ModuleName,
		in,
		*in.Loader,
	)
	body := &gogen.Generator{
		BaseGen: base,
		Rr:      gogen.TemplateRenderer{Tmpl: templates},
	}
	body.Rr.Render(serviceParams{
		G:    body,
		Name: in.ApiStruct.Name,
	})
	for _, fi := range apiSt.Fields {
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok {
			if ref.Name == "HttpPost" {
				body.Rr.Render(postParams{
					G:           body,
					Name:        fi.Name,
					Annotations: fi.Annotations,
					Req:         fi.TypeExpr.Parameters[0],
					Resp:        fi.TypeExpr.Parameters[1],
				})
			}
		}
	}
	fmt.Printf("%s\n", body.Rr.Buf.String())

	// adlast.HandleWithErr_DeclType[*adlast.Struct](
	// 	decl,
	// 	func(struct_ adlast.Struct) (*adlast.Struct, error) {
	// 		return &struct_, nil
	// 	},
	// 	func(union_ adlast.Union) (*adlast.Struct, error) {
	// 		return nil, fmt.Errorf("must be a struct")
	// 	},
	// 	func(type_ adlast.TypeDef) (*adlast.Struct, error) {

	// 	},
	// 	func(newtype_ adlast.NewType) (*adlast.Struct, error) {

	// 	},
	// 	func() (adlast.Struct, error) {
	// 		panic("shouldn't get here")
	// 	},
	// )
	return nil
}

type serviceParams struct {
	G    *gogen.Generator
	Name string
}
type postParams struct {
	G           *gogen.Generator
	Name        string
	Annotations adlast.Annotations
	Req         adlast.TypeExpr
	Resp        adlast.TypeExpr
}

func (in *GoApi) ReservedImports() []goimports.ImportSpec {
	return []goimports.ImportSpec{
		{Path: "net/http"},
	}
}

// GoImport implements gogen.SubTask.
func (bg *GoApi) IsStdLibGen() bool {
	return false
}

// GoImport implements gogen.SubTask.
func (bg *GoApi) GoAdlImportPath() string {
	return "github.com/adl-lang/goadl_rt/v3"
}

// GoImport implements gogen.SubTask.
func (in *GoApi) GoImport(pkg string, currModuleName string, imports goimports.Imports) (string, error) {
	if spec, ok := imports.ByName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	} else {
		imports.AddPath(spec.Path)
		return spec.Name + ".", nil
	}
}

func ExpandTypeAliases(lr *loader.LoadResult, te adlast.TypeExpr) adlast.TypeExpr {
	return te
}
