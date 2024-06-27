package goapi

import (
	"fmt"
	fp "path/filepath"
	"strings"

	"github.com/adl-lang/goadl_rt/v3/customtypes"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/cli/gogen"
	"github.com/adl-lang/goadlc/internal/cli/goimports"
	"github.com/adl-lang/goadlc/internal/cli/loader"
	"github.com/samber/lo"
)

func (in *GoApi) Run() error {
	mod, ok := in.Loader.CombinedAst[in.ApiStruct.ModuleName]
	if !ok {
		return fmt.Errorf("module not found '%s", in.ApiStruct.ModuleName)
	}
	decl0, ok := mod.Decls[in.ApiStruct.Name]
	if !ok {
		return fmt.Errorf("decl not found '%s", in.ApiStruct.Name)
	}
	st, ok := decl0.Type_.Cast_struct_()
	if !ok || len(st.TypeParams) != 0 {
		return fmt.Errorf("unexpected - apiRequests is not a monomorphic struct")
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
	genedSrvs := map[string]*apiInstance{}
	apis := []*apiInstance{{
		ParentName: "",
		Struct:     apiSt,
		ScopedName: in.ApiStruct,
		Field:      nil,
		Kids:       []*apiInstance{},
	}}
	// NOTE: ADL can't have instance of self recursive type, so no need to deal with this case (i.e. remove yup flag)
	for i := 0; i < len(apis); i++ {
		// fmt.Fprintf(os.Stderr, "capapis %v\n", lo.Map(capapis, func(fi nameStruct, _ int) string {
		// 	return fi.Name.Name
		// }))
		inst0 := apis[i]
		if prevSt, ok := genedSrvs[inst0.ScopedName.Name]; ok {
			if prevSt.ScopedName == in.ApiStruct {
				return fmt.Errorf("unexpected - starting struct can't also be used as a capabilty api, use a newtype")
			}
			if prevSt.ScopedName.ModuleName != inst0.ScopedName.ModuleName {
				return fmt.Errorf("unexpected - (alias error) can't used two struct with the same name from different modules, use a newtype")
			}
			continue
		}
		newapis, err := in.genInterface(body, inst0)
		inst0.Kids = newapis
		if err != nil {
			return err
		}
		genedSrvs[inst0.ScopedName.Name] = inst0
		// fmt.Fprintf(os.Stderr, "newapis %v\n", lo.Map(newapis, func(fi nameStruct, _ int) string {
		// 	return fi.Name.Name
		// }))
		apis = append(apis, newapis...)
	}

	uapis := lo.UniqBy(apis, func(it *apiInstance) string {
		return it.ScopedName.Name
	})
	for _, inst := range uapis {
		err = in.genRegister(body, inst)
		if err != nil {
			return err
		}
	}

	modCodeGenDir := strings.Split(in.ApiStruct.ModuleName, ".")
	modCodeGenPkg := modCodeGenDir[len(modCodeGenDir)-1]
	path := fp.Join(fp.Join(in.Outputdir, fp.Join(modCodeGenDir...)), modCodeGenPkg+"_srv.go")
	err = body.WriteFile(in.Root, modCodeGenPkg, path, false, []goimports.ImportSpec{})
	if err != nil {
		return err
	}

	// header := &gogen.Generator{
	// 	BaseGen: body.BaseGen,
	// 	Rr:      gogen.TemplateRenderer{Tmpl: templates},
	// }

	// header.Rr.Render(headerParams{
	// 	Pkg: modCodeGenPkg,
	// })
	// useImports := []goimports.ImportSpec{}
	// for _, spec := range body.Imports.Specs {
	// 	if body.Imports.Used[spec.Path] {
	// 		useImports = append(useImports, spec)
	// 	}
	// }
	// header.Rr.Render(importsParams{
	// 	Imports: useImports,
	// })
	// header.Rr.Buf.Write(body.Rr.Bytes())

	// fmt.Printf("%s\n", header.Rr.Buf.String())

	return nil
}

type apiInstance struct {
	// empty str if root (cli specified struct)
	ParentName string
	Struct     adlast.Struct
	ScopedName adlast.ScopedName
	// nil if root
	Field *adlast.Field
	Kids  []*apiInstance
}

func (in *GoApi) genInterface(
	body *gogen.Generator,
	inst *apiInstance,
	// name string,
	// apiSt adlast.Struct,
	// isCap bool,
) ([]*apiInstance, error) {
	body.Rr.Render(serviceParams{
		G:     body,
		Name:  inst.ScopedName.Name,
		IsCap: inst.Field != nil,
	})
	capapis := []*apiInstance{}
	for _, fi := range inst.Struct.Fields {
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok {
			switch ref.Name {
			case "HttpPost":
				body.Rr.Render(postParams{
					G:           body,
					Name:        fi.Name,
					Annotations: fi.Annotations,
					Req:         fi.TypeExpr.Parameters[0],
					Resp:        fi.TypeExpr.Parameters[1],
					IsCap:       inst.Field != nil,
				})
			case "HttpGet":
				body.Rr.Render(getParams{
					G:           body,
					Name:        fi.Name,
					Annotations: fi.Annotations,
					Resp:        fi.TypeExpr.Parameters[0],
					IsCap:       inst.Field != nil,
				})
			case "CapabilityApi":
				apiTe := fi.TypeExpr.Parameters[2]
				apiRef, ok := apiTe.TypeRef.Cast_reference()
				if !ok {
					return nil, fmt.Errorf("unexpected - cap api is not a ref. TypeExpre : %v", apiTe)
				}
				decl, exist := in.Loader.Resolver(apiRef)
				if !exist {
					return nil, fmt.Errorf("can't resolve decl. Ref : %v", apiRef)
				}
				capSt, ok := decl.Type_.Cast_struct_()
				if !ok {
					return nil, fmt.Errorf("unexpected - cap api is not a struct. Ref : %v", apiRef)
				}
				capapis = append(capapis, &apiInstance{inst.ScopedName.Name, capSt, apiRef, &fi, []*apiInstance{}})
				body.Rr.Render(getcapapiParams{
					G:           body,
					Name:        fi.Name,
					StructName:  apiRef.Name,
					Annotations: fi.Annotations,
					C:           fi.TypeExpr.Parameters[0],
					S:           fi.TypeExpr.Parameters[1],
				})
			}
		}
	}
	body.Rr.Buf.WriteString("}\n")
	return capapis, nil
}

func (in *GoApi) genRegister(
	body *gogen.Generator,
	inst *apiInstance,
) error {
	ann := customtypes.MapMap[adlast.ScopedName, any]{}
	var vte *adlast.TypeExpr
	if inst.Field != nil {
		ann = inst.Field.Annotations
		vte = &inst.Field.TypeExpr.Parameters[2]
	}
	body.Rr.Render(registerParams{
		G:           body,
		Name:        inst.ScopedName.Name,
		IsCap:       inst.Field != nil,
		V:           vte,
		Annotations: ann,
		CapApis: lo.Map(inst.Kids, func(api *apiInstance, _ int) *adlast.Field {
			return api.Field
		}),
	})
	for _, fi := range inst.Struct.Fields {
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok {
			switch ref.Name {
			case "HttpPost":
				body.Rr.Render(regpostParams{
					G:     body,
					Name:  fi.Name,
					IsCap: inst.Field != nil,
				})
			case "HttpGet":
				body.Rr.Render(reggetParams{
					G:     body,
					Name:  fi.Name,
					IsCap: inst.Field != nil,
				})
			case "CapabilityApi":
				apiTe := fi.TypeExpr.Parameters[2]
				apiRef, _ := apiTe.TypeRef.Cast_reference()
				// decl, _ := in.Loader.Resolver(apiRef)
				// capSt, _ := decl.Type_.Cast_struct_()
				body.Rr.Render(regcapapiParams{
					G:          body,
					StructName: apiRef.Name,
					FieldName:  fi.Name,
				})
			}
		}
	}
	body.Rr.Buf.WriteString("}\n")
	return nil
}

func (in *GoApi) ReservedImports() []goimports.ImportSpec {
	return []goimports.ImportSpec{
		{Path: "net/http"},
		{Path: "context"},
		{Path: "github.com/helix-collective/go_protoapp/common/capability"},
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
