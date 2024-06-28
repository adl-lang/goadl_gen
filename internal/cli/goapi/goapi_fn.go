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
	apis := &apiInstance{
		Struct:     apiSt,
		ScopedName: in.ApiStruct,
		Field:      nil,
	}
	result0 := []*apiInstance{}
	visited0 := map[string]bool{}
	err = in.dfs(apis, apis, visited0, &result0)
	if err != nil {
		return err
	}
	in.genInterface(body, apis)
	in.genRegister(body, apis)
	for _, apis0 := range result0[1:] {
		result1 := []*apiInstance{}
		visited1 := map[string]bool{}
		err = in.dfs(apis, apis, visited1, &result1)
		if err != nil {
			return err
		}
		in.genInterface(body, apis0)
		in.genRegister(body, apis0)
	}
	modCodeGenDir := strings.Split(in.ApiStruct.ModuleName, ".")
	modCodeGenPkg := modCodeGenDir[len(modCodeGenDir)-1]
	path := fp.Join(fp.Join(in.Outputdir, fp.Join(modCodeGenDir...)), modCodeGenPkg+"_srv.go")
	err = body.WriteFile(in.Root, modCodeGenPkg, path, false, []goimports.ImportSpec{})
	if err != nil {
		return err
	}
	return nil
}

var reservedNames = []string{"C", "c", "S", "s", "V", "v"}

func (in *GoApi) dfs(root *apiInstance, apiSt *apiInstance, visited map[string]bool, result *[]*apiInstance) error {
	if lo.Contains(reservedNames, apiSt.ScopedName.Name) {
		return fmt.Errorf("error srtuct of type CapabilityApi name clash with type params (can't be named [C,S,V]). %s.%s", apiSt.ScopedName.ModuleName, apiSt.ScopedName.Name)
	}
	if visited[apiSt.ScopedName.Name] {
		curr, _ := lo.Find(*result, func(it *apiInstance) bool {
			return it.ScopedName.Name == apiSt.ScopedName.Name
		})
		if curr.ScopedName.ModuleName != apiSt.ScopedName.ModuleName {
			return fmt.Errorf("unexpected - (alias error) can't used two struct with the same name from different modules, use a newtype")
		}
		if root != nil {
			if root == curr {
				return fmt.Errorf("unexpected - starting struct can't also be used as a capabilty api, use a newtype")
			}
		}
		return nil
	}
	visited[apiSt.ScopedName.Name] = true
	*result = append(*result, apiSt)
	for _, fi := range apiSt.Struct.Fields {
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok && ref.Name == "CapabilityApi" {
			if lo.Contains(reservedNames, fi.Name) {
				return fmt.Errorf("error fields of type CapabilityApi name clash with type params (can't be named [C,S,V]). %s.%s::%s", apiSt.ScopedName.ModuleName, apiSt.ScopedName.Name, fi.Name)
			}
			apiTe := fi.TypeExpr.Parameters[2]
			apiRef, ok := apiTe.TypeRef.Cast_reference()
			if !ok {
				return fmt.Errorf("unexpected - cap api is not a ref. TypeExpre : %v", apiTe)
			}
			decl, exist := in.Loader.Resolver(apiRef)
			if !exist {
				return fmt.Errorf("can't resolve decl. Ref : %v", apiRef)
			}
			capSt, ok := decl.Type_.Cast_struct_()
			if !ok {
				return fmt.Errorf("unexpected - cap api is not a struct. Ref : %v", apiRef)
			}
			inst0 := &apiInstance{
				Struct:     capSt,
				ScopedName: apiRef,
				Field:      &fi,
			}
			err := in.dfs(nil, inst0, visited, result)
			if err != nil {
				return nil
			}
		}
	}
	return nil
}

func (in *GoApi) transKids(path string, apiSt *adlast.Struct) []tkid {
	return lo.FlatMap(apiSt.Fields, func(fi adlast.Field, _ int) []tkid {
		ret := []tkid{}
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok && ref.Name == "CapabilityApi" {
			apiTe := fi.TypeExpr.Parameters[2]
			apiRef, _ := apiTe.TypeRef.Cast_reference()
			decl, _ := in.Loader.Resolver(apiRef)
			capSt, _ := decl.Type_.Cast_struct_()
			kids := in.transKids(path+fi.Name+"_", &capSt)

			name := path + fi.Name
			ret = append(ret, tkid{name, &fi})
			ret = append(ret, kids...)
		}
		return ret
	})
}

type apiInstance struct {
	Struct     adlast.Struct
	ScopedName adlast.ScopedName
	Field      *adlast.Field
}

func (in *GoApi) genInterface(
	body *gogen.Generator,
	inst *apiInstance,
) {
	body.Rr.Render(serviceParams{
		G:     body,
		Name:  inst.ScopedName.Name,
		IsCap: inst.Field != nil,
	})
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
				apiRef, _ := apiTe.TypeRef.Cast_reference()
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
	tkids := in.transKids("", &inst.Struct)
	body.Rr.Render(registerParams{
		G:           body,
		Name:        inst.ScopedName.Name,
		IsCap:       inst.Field != nil,
		V:           vte,
		Annotations: ann,
		CapApis:     tkids,
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
				decl, _ := in.Loader.Resolver(apiRef)
				capSt, _ := decl.Type_.Cast_struct_()
				tkid := []tkid{{fi.Name, &fi}}
				tkid = append(tkid, in.transKids(fi.Name+"_", &capSt)...)
				body.Rr.Render(regcapapiParams{
					G:          body,
					StructName: apiRef.Name,
					Name:       fi.Name,
					Kids:       tkid,
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
