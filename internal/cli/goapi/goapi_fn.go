package goapi

import (
	"fmt"

	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
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
	body := &generator{
		baseGen: in.newBaseGen(in.GoMod.ModulePath, midPath),
		rr:      templateRenderer{t: templates},
	}
	body.rr.Render(serviceParams{
		Name: in.ApiStruct.Name,
	})
	for _, fi := range apiSt.Fields {
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok {
			if ref.Name == "HttpPost" {
				body.rr.Render(postParams{
					Name: fi.Name,
					Req:  fi.TypeExpr.Parameters[0],
					Resp: fi.TypeExpr.Parameters[1],
				})
			}
		}
	}
	fmt.Printf("%s\n", body.rr.buf.String())

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
	G    *generator
	Name string
}
type postParams struct {
	G    *generator
	Name string
	Req  adlast.TypeExpr
	Resp adlast.TypeExpr
}

func (in *GoApi) reservedImports() []goimports.ImportSpec {
	return []goimports.ImportSpec{
		{Path: "net/http"},
	}
}

type snResolver func(sn adlast.ScopedName) (*adlast.Decl, bool)

type baseGen struct {
	cli        *GoApi
	resolver   snResolver
	modulePath string
	midPath    string
	// moduleName string
	imports goimports.Imports
}

func (in *GoApi) newBaseGen(
	modulePath, midPath string,
	// moduleName string,
) *baseGen {
	imports := goimports.NewImports(
		in.reservedImports(),
		in.Loader.BundleMaps,
	)
	return &baseGen{
		cli:        in,
		resolver:   in.Loader.Resolver,
		modulePath: modulePath,
		midPath:    midPath,
		// moduleName: moduleName,
		imports: imports,
	}
}

func (bg *baseGen) GoImport(pkg string) (string, error) {
	if spec, ok := bg.imports.ByName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	} else {
		bg.imports.AddPath(spec.Path)
		return spec.Name + ".", nil
	}
}

type generator struct {
	*baseGen
	rr        templateRenderer
	genAdlAst bool
}

func ExpandTypeAliases(lr *loader.LoadResult, te adlast.TypeExpr) adlast.TypeExpr {
	return te
}
