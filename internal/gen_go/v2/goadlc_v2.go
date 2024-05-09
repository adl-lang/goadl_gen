package gen_go_v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/golang/glog"
	"github.com/jpillora/opts"
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
	// NoOverwrite    bool     `help:"Don't update files that haven't changed"`
	// Manifest       string   `help:"Write a manifest file recording generated files"`
	// CombinedOutput string   `help:"The json file to which all adl modules will be written"`
	Files []string `opts:"mode=arg"`
}

// func (in *goadlcV2Cmd) workingDir() string  { return in.V1.WorkingDir }
// func (in *goadlcV2Cmd) searchdir() []string { return in.V1.Searchdir }
// func (in *goadlcV2Cmd) mergeAdlext() string { return in.V1.MergeAdlext }
// func (in *goadlcV2Cmd) debug() bool         { return in.V1.Debug }
// func (in *goadlcV2Cmd) files() []string     { return in.V1.Files }

func (in *goadlcV2Cmd) Run() error {

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
			generalDeclV2(fname, path, name, modCodeGen, decl, m.name)
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
	// fd         *os.File
	moduleName string
	name       string
	rr         templateRenderer
	imports    imports
}

func generalDeclV2(fname string, path string, name string, modCodeGen ModuleCodeGen, decl goadl.Decl, moduleName string) {
	header := &generator{
		rr:         templateRenderer{t: templates},
		name:       name,
		moduleName: moduleName,
	}
	body := &generator{
		rr:         templateRenderer{t: templates},
		name:       name,
		moduleName: moduleName,
	}
	goadl.HandleDeclType(
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
	header.rr.Render(importsParams{
		Rt: "github.com/adl-lang/goadl_rt/v2",
		// RtAs: "goadl",
		Imports: []string{
			"encoding/json",
			"strings",
		},
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
		glog.Fatalf("open %s, error: %v", path, err)
	}
	fd.Write(header.rr.Bytes())
	fd.Write(body.rr.Bytes())
}

type scopedDeclParams struct {
	G          *generator
	ModuleName string
	Name       string
	Decl       goadl.Decl
}

type texprmonoParams scopedDeclParams

func (*generator) JsonEncode(val any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.Encode(val)
	return string(bytes.Trim(buf.Bytes(), "\n"))
	// return  buf.String()
}

func (in *generator) generateStruct(s goadl.DeclTypeBranch_Struct_) (interface{}, error) {
	//	s.TypeParams
	fmt.Fprintf(&in.rr.buf, "type %s struct {\n", in.name)
	for _, fld := range s.Fields {
		goType := in.GoType(fld.TypeExpr)
		goFldName := strings.ToUpper(fld.Name[:1]) + fld.Name[1:]
		fmt.Fprintf(&in.rr.buf, "    %[1]s %[2]s\n", goFldName, goType)

		// goadl.HandleTypeRef(
		// 	fld.TypeExpr.TypeRef.Branch,
		// 	func(primitive goadl.TypeRefBranch_Primitive) (interface{}, error) {
		// 		in.generateStructPrimitiveField(primitive, fld)
		// 		return nil, nil
		// 	},
		// 	func(typeParam goadl.TypeRefBranch_TypeParam) (interface{}, error) {
		// 		fmt.Fprintf(&in.rr.buf, "	/* typeParam not implemented %s */\n", typeParam)
		// 		return nil, nil
		// 	},
		// 	func(reference goadl.TypeRefBranch_Reference) (interface{}, error) {
		// 		fmt.Fprintf(&in.rr.buf, "	/* reference not implemented %s.%s */\n", reference.ModuleName, reference.Name)
		// 		return nil, nil
		// 	},
		// )
	}

	fmt.Fprintf(&in.rr.buf, "}\n")
	return nil, nil
}

func (in *generator) generateStructPrimitiveField(primitive goadl.TypeRefBranch_Primitive, fld goadl.Field) (interface{}, error) {
	if _type, ex := goadl.PrimitiveMap[primitive]; ex {
		in.rr.Render(structPrimParams{
			G:              in,
			Name:           fld.Name,
			Type:           _type,
			SerializedName: fld.SerializedName,
		})
		return nil, nil
	}
	// te := fld.TypeExpr.Parameters[0]
	switch primitive {
	case "Vector":
		in.rr.Render(structFieldVectorParams{
			G:              in,
			Name:           fld.Name,
			Type:           "x",
			SerializedName: fld.SerializedName,
		})
	case "StringMap":
		in.rr.Render(structFieldStringMapParams{
			G:              in,
			Name:           fld.Name,
			Type:           "y",
			SerializedName: fld.SerializedName,
		})
	case "Nullable":
		in.rr.Render(structFieldNullableParams{
			G:              in,
			Name:           fld.Name,
			Type:           "z",
			SerializedName: fld.SerializedName,
		})
	}
	return nil, nil
}

type structPrimParams struct {
	G              *generator
	Name           string
	SerializedName string
	Type           string
}

type structFieldVectorParams structPrimParams
type structFieldStringMapParams structPrimParams
type structFieldNullableParams structPrimParams

func (in *generator) ToTitle(s string) string {
	return strings.ToTitle(s)
}

func (in *generator) generateUnion(u goadl.DeclTypeBranch_Union_) (interface{}, error) {
	fmt.Fprintf(&in.rr.buf, "	/* Union not implemented */\n")
	if len(u.TypeParams) > 0 {
		fmt.Fprintf(&in.rr.buf, "	/* Union TypeParams not implemented %v*/\n", u.TypeParams)
	}
	fmt.Fprintf(&in.rr.buf, "type %s struct {\n", in.name)
	for _, fld := range u.Fields {
		goadl.HandleTypeRef(
			fld.TypeExpr.TypeRef.Branch,
			func(primitive goadl.TypeRefBranch_Primitive) (interface{}, error) {
				goFldName := strings.ToTitle(fld.Name)
				if fld.Default.Just != nil {
					fmt.Fprintf(&in.rr.buf, "	/* default todo %v*/\n", fld.Default.Just)
				}
				_type := goadl.PrimitiveMap[primitive]
				fmt.Fprintf(&in.rr.buf, "	%[2]s *%[3]s `json:\"%[1]s\"`\n", fld.Name, goFldName, _type)
				return nil, nil
			},
			func(typeParam goadl.TypeRefBranch_TypeParam) (interface{}, error) {
				fmt.Fprintf(&in.rr.buf, "	/* typeParam not implemented %s */\n", typeParam)
				return nil, nil
			},
			func(reference goadl.TypeRefBranch_Reference) (interface{}, error) {
				fmt.Fprintf(&in.rr.buf, "	/* reference not implemented %s.%s */\n", reference.ModuleName, reference.Name)
				return nil, nil
			},
		)
	}
	fmt.Fprintf(&in.rr.buf, "}\n")
	fmt.Fprintf(&in.rr.buf, `func init() {
goadl.RESOLVER.Register(
goadl.ScopedName{
	ModuleName: "%[1]s",
	Name:       "%[2]s",
},
func() interface{} {
	return &%[2]s{}
},
)
}`, in.moduleName, in.name)
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
