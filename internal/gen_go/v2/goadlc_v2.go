package gen_go_v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	// _ = declMap
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
		for name, decl := range m.module.Decls {
			fname := path + "/" + name + ".go"
			generalDeclV2(declMap, fname, modulePath, name, modCodeGen, decl, m.name)
			if !in.NoGoFmt {
				out, err := exec.Command("go", "fmt", fname).CombinedOutput()
				if err != nil {
					glog.Fatalf("go fmt error err : %v output '%s'", err, string(out))
				}
			}
		}
	}
	return nil
}

type generator struct {
	declMap    moduleMap[goadl.Decl]
	modulePath string
	moduleName string
	name       string
	rr         templateRenderer
	imports    imports
}

func generalDeclV2(
	declMap moduleMap[goadl.Decl],
	fname string,
	modulePath string,
	name string,
	modCodeGen ModuleCodeGen,
	decl goadl.Decl,
	moduleName string,
) {
	header := &generator{
		declMap:    declMap,
		rr:         templateRenderer{t: templates},
		name:       goEscape(name),
		moduleName: moduleName,
		modulePath: modulePath,
	}
	body := &generator{
		declMap:    declMap,
		rr:         templateRenderer{t: templates},
		name:       goEscape(name),
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
		Name:       goEscape(name),
		TypeParams: getTypeParams(decl),
		Decl:       decl,
	})
	body.rr.Render(scopedDeclParams{
		G:          body,
		ModuleName: moduleName,
		Name:       goEscape(name),
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

func getTypeParams(decl goadl.Decl) typeParam {
	return goadl.HandleP_DeclType[typeParam](
		decl.Type.Branch,
		func(struct_ goadl.Struct) typeParam { return typeParam{struct_.TypeParams, false} },
		func(union_ goadl.Union) typeParam { return typeParam{union_.TypeParams, false} },
		func(type_ goadl.TypeDef) typeParam { return typeParam{type_.TypeParams, false} },
		func(newtype_ goadl.NewType) typeParam { return typeParam{newtype_.TypeParams, false} },
	)
}

type typeParam struct {
	ps    []string
	added bool
}

func (tp typeParam) AddParam(newp string) typeParam {
	psMap := make(map[string]bool)
	tp0 := make([]string, len(tp.ps)+1)
	for i, p := range tp.ps {
		tp0[i] = p
		psMap[p] = true
	}

	tp0[len(tp.ps)] = newp
	if psMap[tp0[len(tp.ps)]] {
		n := uint64(1)
		for {
			n++
			tp0[len(tp.ps)] = newp + strconv.FormatUint(n, 10)
			if !psMap[tp0[len(tp.ps)]] {
				break
			}
		}
	}
	return typeParam{tp0, true}
}
func (tp typeParam) Has() bool {
	return (!tp.added && len(tp.ps) != 0) || len(tp.ps) != 1
}
func (tp typeParam) Last() string {
	if len(tp.ps) == 0 {
		return ""
	}
	return tp.ps[len(tp.ps)-1]
}
func (tp typeParam) LSide() string {
	if len(tp.ps) == 0 {
		return ""
	}
	return "[" + strings.Join(slices.Map(tp.ps, func(e string) string { return e + " any" }), ", ") + "]"
}
func (tp typeParam) RSide() string {
	if len(tp.ps) == 0 {
		return ""
	}
	return "[" + strings.Join(tp.ps, ",") + "]"
}
func (tp typeParam) TexprArgs() string {
	if len(tp.ps) == 0 {
		return ""
	}
	return strings.Join(slices.Map(tp.ps, func(e string) string { return fmt.Sprintf("%s goadl.ATypeExpr[%s]", strings.ToLower(e), e) }), ", ")
}
func (tp typeParam) TexprValues() string {
	if len(tp.ps) == 0 {
		return ""
	}
	return strings.Join(slices.Map(tp.ps, func(e string) string { return fmt.Sprintf("%s.Value", strings.ToLower(e)) }), ", ")
}

func (*generator) JsonEncode(val any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.Encode(val)
	return string(bytes.Trim(buf.Bytes(), "\n"))
	// return  buf.String()
}

func (in *generator) generateStruct(s goadl.DeclTypeBranch_Struct_) (interface{}, error) {
	in.rr.Render(structParams{
		G:          in,
		Name:       goEscape(in.name),
		TypeParams: typeParam{s.TypeParams, false},
		Fields: slices.Map[goadl.Field, unionBranchParams](s.Fields, func(f goadl.Field) unionBranchParams {
			return unionBranchParams{
				Name: goEscape(f.Name),
				Type: in.GoType(f.TypeExpr),
			}
		}),
	})
	return nil, nil
}

func (in *generator) ToTitle(s string) string {
	return strings.ToTitle(s)
}

func (in *generator) generateUnion(u goadl.DeclTypeBranch_Union_) (interface{}, error) {
	in.rr.Render(unionParams{
		G:          in,
		Name:       goEscape(in.name),
		TypeParams: typeParam{u.TypeParams, false},
		Branches: slices.Map[goadl.Field, unionBranchParams](u.Fields, func(f goadl.Field) unionBranchParams {
			return unionBranchParams{
				Name:       goEscape(f.Name),
				TypeParams: typeParam{u.TypeParams, false},
				Type:       in.GoType(f.TypeExpr),
			}
		}),
	})
	return nil, nil
}

func (in *generator) generateTypeAlias(td goadl.DeclTypeBranch_Type_) (interface{}, error) {
	in.rr.Render(typeAliasParams{
		G:          in,
		Name:       goEscape(in.name),
		TypeParams: typeParam{td.TypeParams, false},
		RType:      in.GoType(td.TypeExpr),
	})
	return nil, nil
}

type typeAliasParams struct {
	G          *generator
	Name       string
	TypeParams typeParam
	RType      goTypeExpr
}
type newTypeParams typeAliasParams

func (in *generator) generateNewType(nt goadl.DeclTypeBranch_Newtype_) (interface{}, error) {
	in.rr.Render(newTypeParams{
		G:          in,
		Name:       goEscape(in.name),
		TypeParams: typeParam{nt.TypeParams, false},
		RType:      in.GoType(nt.TypeExpr),
	})
	return nil, nil
}

func jsonPrimitiveDefaultToGo(primitive string, defVal interface{}) string {
	switch defVal.(type) {
	case string:
		return fmt.Sprintf(`%q`, defVal)
	}
	return fmt.Sprintf(`%v`, defVal)
}
