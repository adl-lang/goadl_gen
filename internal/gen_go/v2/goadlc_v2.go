package gen_go_v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/adl-lang/goadlc/internal/fn/slices"
	"github.com/golang/glog"
	"github.com/jpillora/opts"
	"golang.org/x/mod/modfile"
)

func NewGenGoV2() opts.Opts {
	wk, err := os.MkdirTemp("", "goadlc-")
	if err != nil {
		glog.Warningf(`os.MkdirTemp("", "goadlc-") %v`, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		glog.Warningf(`error getting current working directory %v`, err)
	}
	return opts.New(&goadlcV2Cmd{
		WorkingDir: wk,
		Outputdir:  cwd,
	})
}

type goadlcV2Cmd struct {
	WorkingDir  string
	Searchdir   []string `opts:"short=I" help:"Add the specifed directory to the ADL searchpath"`
	Outputdir   string   `opts:"short=O" help:"Set the directory where generated code is written "`
	MergeAdlext string   `help:"Add the specifed adl file extension to merged on loading"`
	Debug       bool     `help:"Print extra diagnostic information, especially about files being read/written"`
	NoGoFmt     bool     `help:"Don't run 'go fmt' on the generated files"`
	ModulePath  string   `help:"The path of the Go module for the generated code. Overrides the module-path from the '--go-mod-file' flag."`
	GoModFile   string   `help:"Path of a go.mod file. If the file exists, the module-path is used for generated imports."`
	// NoOverwrite    bool     `help:"Don't update files that haven't changed"`
	// Manifest       string   `help:"Write a manifest file recording generated files"`
	// CombinedOutput string   `help:"The json file to which all adl modules will be written"`
	Files []string `opts:"mode=arg"`
}

func (in *goadlcV2Cmd) Run() error {
	if len(in.Files) == 0 {
		return fmt.Errorf("no files specified")
	}

	jb := func(fd io.Reader) (moduleMap[goadl.Module], moduleMap[goadl.Decl], error) {
		combinedAst := make(moduleMap[goadl.Module])
		declMap := make(moduleMap[goadl.Decl])
		dec := json.NewDecoder(fd)
		err := dec.Decode(&combinedAst)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range combinedAst {
			for dk, dv := range v.Decls {
				declMap[k+"::"+dk] = dv
			}
		}
		return combinedAst, declMap, nil
	}
	modules := []moduleTuple[goadl.Module]{}
	combinedAst, declMap, err := loadAdl(in, &modules, jb)
	_ = combinedAst
	_ = declMap
	if err != nil {
		os.Exit(1)
	}

	modulePath := ""
	if in.ModulePath != "" {
		modulePath = in.ModulePath
	} else {
		if in.GoModFile == "" {
			in.GoModFile = filepath.Join(in.Outputdir, "go.mod")
			if in.Debug {
				fmt.Fprintf(os.Stderr, "looking for module-path in go.mod file. go.mod:%s\n", in.GoModFile)
			}
		}
		if gms, err := os.Stat(in.GoModFile); err == nil {
			if !gms.IsDir() {
				if modbufm, err := os.ReadFile(in.GoModFile); err == nil {
					modulePath = modfile.ModulePath(modbufm)
					if in.Debug {
						fmt.Fprintf(os.Stderr, "using module-path found in go.mod file. module-path:%s\n", in.GoModFile)
					}
				} else {
					return fmt.Errorf("module-path needed. Not specified in --module-path and couldn't be found in a go.mod file")
				}
			}
		} else {
			return fmt.Errorf("module-path required. Not specified in --module-path and no go.mod file found in output directory")
		}
	}

	// fmt.Printf("cli modules\n")
	for _, m := range modules {
		modCodeGen := ModuleCodeGen{}
		modCodeGen.Directory = strings.Split(m.name, ".")
		path := in.Outputdir + "/" + strings.Join(modCodeGen.Directory, "/")
		if _, err = os.Open(path); err != nil {
			err = os.MkdirAll(path, os.ModePerm)
			if err != nil {
				glog.Fatalf("mkdir -p %s, error: %v", path, err)
			}
		}
		// fmt.Printf("  annotation %v\n", m.module.Annotations)

		for name, decl := range m.module.Decls {
			fname := path + "/" + name + ".go"
			generalDeclV2(fname, modulePath, name, modCodeGen, decl, m.name)
			if !in.NoGoFmt {
				out, err := exec.Command("go", "fmt", fname).CombinedOutput()
				if err != nil {
					glog.Fatalf("go fmt error err : %v output '%s'", err, string(out))
				}
			}
		}
	}
	// fmt.Printf("all modules\n")
	// for k := range allModules {
	// 	fmt.Printf(" %s\n", k)
	// }
	return nil
}

type generator struct {
	modulePath string
	moduleName string
	name       string
	rr         templateRenderer
	imports    imports
}

func generalDeclV2(
	fname string,
	modulePath string,
	name string,
	modCodeGen ModuleCodeGen,
	decl goadl.Decl,
	moduleName string,
) {
	header := &generator{
		rr:         templateRenderer{t: templates},
		name:       name,
		moduleName: moduleName,
		modulePath: modulePath,
	}
	body := &generator{
		rr:         templateRenderer{t: templates},
		name:       name,
		moduleName: moduleName,
		imports:    newImports(),
		modulePath: modulePath,
	}
	goadl.HandleE_DeclType[any](
		decl.Type.Branch,
		body.generateStruct,
		body.generateUnion,
		body.generateTypeAlias,
		body.generateNewType,
	)

	body.rr.Render(texprmonoParams{
		G:          body,
		ModuleName: moduleName,
		Name:       name,
		Decl:       decl,
	})
	body.rr.Render(scopedDeclParams{
		G:          body,
		ModuleName: moduleName,
		Name:       name,
		Decl:       decl,
	})

	header.rr.Render(headerParams{
		Pkg: modCodeGen.Directory[len(modCodeGen.Directory)-1],
	})
	imports := []importSpec{
		{Path: "encoding/json"},
		{Path: "strings"},
	}
	for _, spec := range body.imports.specs {
		if body.imports.used[spec.Path] {
			imports = append(imports, spec)
		}
	}
	header.rr.Render(importsParams{
		Rt: "github.com/adl-lang/goadl_rt/v2",
		// RtAs: "goadl",
		Imports: imports,
	})

	var fd *os.File = nil
	var err error
	fd, err = os.OpenFile(fname, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	fd.Truncate(0)
	fd.Seek(0, 0)
	defer func() {
		fd.Sync()
		fd.Close()
	}()
	if err != nil {
		glog.Fatalf("open %s, error: %v", fname, err)
	}
	fd.Write(header.rr.Bytes())
	fd.Write(body.rr.Bytes())
}

func (*generator) JsonEncode(val any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.Encode(val)
	return string(bytes.Trim(buf.Bytes(), "\n"))
	// return  buf.String()
}

func (in *generator) generateStruct(s goadl.DeclTypeBranch_Struct_) (interface{}, error) {
	fmt.Fprintf(&in.rr.buf, "type %s struct {\n", in.name)
	for _, fld := range s.Fields {
		goType := in.GoType(fld.TypeExpr)
		goFldName := strings.ToUpper(fld.Name[:1]) + fld.Name[1:]
		fmt.Fprintf(&in.rr.buf, "    %[1]s %[2]s\n", goFldName, goType)
	}
	fmt.Fprintf(&in.rr.buf, "}\n")
	return nil, nil
}

func (in *generator) ToTitle(s string) string {
	return strings.ToTitle(s)
}

func (in *generator) generateUnion(u goadl.DeclTypeBranch_Union_) (interface{}, error) {
	branches := slices.Map[goadl.Field, unionBranchParams](u.Fields, func(f goadl.Field) unionBranchParams {
		return unionBranchParams{
			Name: f.Name,
			Type: in.GoType(f.TypeExpr),
		}
	})
	err := in.rr.Render(unionParams{
		G:        in,
		Name:     in.name,
		Branches: branches,
	})
	if err != nil {
		glog.Fatalf("%v", err)
	}
	// 	if len(u.TypeParams) > 0 {
	// 		fmt.Fprintf(&in.rr.buf, "	/* Union TypeParams not implemented %v*/\n", u.TypeParams)
	// 	}
	// 	typeName := strings.ToUpper(in.name[:1]) + in.name[1:]
	// 	fmt.Fprintf(&in.rr.buf, "type %s interface {\n", typeName)
	// 	fmt.Fprintf(&in.rr.buf, "   Branch() %sBranch\n", typeName)
	// 	fmt.Fprintf(&in.rr.buf, "}\n")
	// 	fmt.Fprintf(&in.rr.buf, "\n")

	// 	implName := GoEscape(strings.ToLower(in.name[:1]) + in.name[1:])
	// 	fmt.Fprintf(&in.rr.buf, "type %s map[string]interface{}\n", implName)
	// 	fmt.Fprintf(&in.rr.buf, "\n")

	// 	fmt.Fprintf(&in.rr.buf, "func Cast_%[1]s(v interface{}) %[1]s {\n", in.name)
	// 	fmt.Fprintf(&in.rr.buf, "    obj := %s(v.(map[string]interface{}))\n", implName)
	// 	fmt.Fprintf(&in.rr.buf, "    return &obj\n")
	// 	fmt.Fprintf(&in.rr.buf, "}\n")
	// 	fmt.Fprintf(&in.rr.buf, "\n")

	// 	fmt.Fprintf(&in.rr.buf, "type %sBranch interface {\n", typeName)
	// 	fmt.Fprintf(&in.rr.buf, "   is%sBranch()\n", typeName)
	// 	fmt.Fprintf(&in.rr.buf, "}\n")
	// 	fmt.Fprintf(&in.rr.buf, "\n")

	// 	for _, fld := range u.Fields {
	// 		fieldTypeName := strings.ToUpper(fld.Name[:1]) + fld.Name[1:]
	// 		fmt.Fprintf(&in.rr.buf, "type %[1]s_%[2]s struct {\n", typeName, fieldTypeName)
	// 		gotype := in.GoType(fld.TypeExpr)
	// 		if !gotype.TypeParam {
	// 			fmt.Fprintf(&in.rr.buf, "    %s\n", gotype)
	// 		} else {
	// 			fmt.Fprintf(&in.rr.buf, "    %s any\n", gotype)
	// 		}
	// 		fmt.Fprintf(&in.rr.buf, "}\n")
	// 		fmt.Fprintf(&in.rr.buf, "\n")
	// 	}
	// 	fmt.Fprintf(&in.rr.buf, "\n")

	// 	for _, fld := range u.Fields {
	// 		fieldTypeName := strings.ToUpper(fld.Name[:1]) + fld.Name[1:]
	// 		fmt.Fprintf(&in.rr.buf, "func (%[1]s_%[2]s) is%[1]sBranch() {}\n", typeName, fieldTypeName)
	// 	}
	// 	fmt.Fprintf(&in.rr.buf, "\n")

	// 	fmt.Fprintf(&in.rr.buf, "func (obj *%s) Branch() %sBranch {\n", implName, typeName)
	// 	fmt.Fprintf(&in.rr.buf, "for k, v := range *obj {\n")
	// 	fmt.Fprintf(&in.rr.buf, "    switch k {\n")
	// 	for _, fld := range u.Fields {
	// 		fmt.Fprintf(&in.rr.buf, "    case \"%s\":\n", fld.Name)
	// 		goadl.HandleTypeRef(
	// 			fld.TypeExpr.TypeRef.Branch,
	// 			func(primitive goadl.TypeRefBranch_Primitive) (interface{}, error) {
	// 				goFldName := strings.ToUpper(fld.Name[:1]) + fld.Name[1:]
	// 				_type := in.PrimitiveMap(string(primitive), fld.TypeExpr.Parameters)
	// 				fmt.Fprintf(&in.rr.buf, "        return %[1]s_%[2]s{v.(%[3]s)}\n", typeName, goFldName, _type)
	// 				return nil, nil
	// 			},
	// 			func(typeParam goadl.TypeRefBranch_TypeParam) (interface{}, error) {
	// 				fieldTypeName := strings.ToUpper(fld.Name[:1]) + fld.Name[1:]
	// 				fmt.Fprintf(&in.rr.buf, "        return %[1]s_%[2]s{v}\n", typeName, fieldTypeName)
	// 				return nil, nil
	// 			},
	// 			func(ref goadl.TypeRefBranch_Reference) (interface{}, error) {
	// 				fieldTypeName := strings.ToUpper(fld.Name[:1]) + fld.Name[1:]
	// 				fmt.Fprintf(&in.rr.buf, "        return %[1]s_%[2]s {\n", typeName, fieldTypeName)

	// 				if in.moduleName == ref.ModuleName {
	// 					fmt.Fprintf(&in.rr.buf, "            Cast_%s(v),\n", ref.Name)
	// 					fmt.Fprintf(&in.rr.buf, "        }\n")
	// 					return nil, nil
	// 				}
	// 				// in.imports = append(in.imports, ref.ModuleName)
	// 				fmt.Fprintf(&in.rr.buf, "            %sCast_%s(v),\n", strings.ReplaceAll(ref.ModuleName, ".", "_")+".", ref.Name)
	// 				fmt.Fprintf(&in.rr.buf, "        }\n")
	// 				return nil, nil
	// 			},
	// 		)
	// 	}
	// 	fmt.Fprintf(&in.rr.buf, `    default:
	//         panic("unknown branch '" + k + "'")
	// 	}
	// }
	// panic("empty map")
	// }

	// `)
	// 	fmt.Fprintf(&in.rr.buf, "	*/ \n")
	return nil, nil
}

func (in *generator) generateTypeAlias(td goadl.DeclTypeBranch_Type_) (interface{}, error) {
	fmt.Fprintf(&in.rr.buf, "	/* TypeDef not implemented */\n")
	return nil, nil
}

func (in *generator) generateNewType(nt goadl.DeclTypeBranch_Newtype_) (interface{}, error) {
	fmt.Fprintf(&in.rr.buf, "	/* NewType not implemented */\n")
	return nil, nil
}

func jsonPrimitiveDefaultToGo(primitive string, defVal interface{}) string {
	switch defVal.(type) {
	case string:
		return fmt.Sprintf(`%q`, defVal)
	}
	return fmt.Sprintf(`%v`, defVal)
}
