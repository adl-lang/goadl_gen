package gen_go_v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"os"
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
	// _ = declMap
	if err != nil {
		os.Exit(1)
	}

	modulePath := ""
	midPath := ""
	if in.ModulePath != "" {
		modulePath = in.ModulePath
	} else {
		if in.GoModFile == "" {
			dir := in.Outputdir
			goMod := filepath.Join(dir, "go.mod")
			last := false
			if in.Outputdir == "" {
				last = true
			}
			for {
				if in.Debug {
					fmt.Fprintf(os.Stderr, "searching for module-path in go.mod file. go.mod:%s\n", goMod)
				}

				if gms, err := os.Stat(goMod); err == nil && !gms.IsDir() {
					in.GoModFile = goMod
					break
				}
				dir0, file := filepath.Split(dir)
				fmt.Fprintf(os.Stderr, ">>> '%s' '%s'\n", dir0, file)
				dir = dir0
				if last {
					break
				}
				if dir == "" {
					last = true
				}
				goMod = filepath.Join(dir, "go.mod")
				midPath = filepath.Join(midPath, file)
			}
			in.GoModFile = goMod
			if in.Debug {
				fmt.Fprintf(os.Stderr, "looking for module-path in go.mod file. go.mod:%s\n", in.GoModFile)
			}
		}
		if gms, err := os.Stat(in.GoModFile); err == nil {
			if !gms.IsDir() {
				if modbufm, err := os.ReadFile(in.GoModFile); err == nil {
					modulePath = modfile.ModulePath(modbufm)
					if in.Debug {
						fmt.Fprintf(os.Stderr, "using module-path found in go.mod file. module-path:%s\n", modulePath)
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
			baseGen := &baseGen{
				declMap:    declMap,
				modulePath: modulePath,
				midPath:    midPath,
				moduleName: m.name,
				name:       name,
			}
			unformatted := baseGen.generalDeclV2(modCodeGen, decl)
			var formatted []byte
			if !in.NoGoFmt {
				formatted, err = format.Source(unformatted)
				if err != nil {
					formatted = unformatted
				}
			} else {
				formatted = unformatted
			}
			var fd *os.File = nil
			var err error
			fname := path + "/" + name + ".go"
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
			fd.Write(formatted)
		}
	}
	return nil
}

type baseGen struct {
	declMap    moduleMap[goadl.Decl]
	modulePath string
	midPath    string
	moduleName string
	name       string
}

type generator struct {
	*baseGen
	rr      templateRenderer
	imports imports
}

func (base *baseGen) generalDeclV2(
	modCodeGen ModuleCodeGen,
	decl goadl.Decl,
) []byte {
	header := &generator{
		baseGen: base,
		rr:      templateRenderer{t: templates},
	}
	body := &generator{
		baseGen: base,
		rr:      templateRenderer{t: templates},
		imports: newImports(),
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
		ModuleName: base.moduleName,
		Name:       goEscape(base.name),
		TypeParams: getTypeParams(decl),
		Decl:       decl,
	})
	body.rr.Render(scopedDeclParams{
		G:          body,
		ModuleName: base.moduleName,
		Name:       goEscape(base.name),
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
	header.rr.buf.Write(body.rr.Bytes())
	return header.rr.Bytes()
	// var fd *os.File = nil
	// var err error
	// fd, err = os.OpenFile(fname, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	// fd.Truncate(0)
	// fd.Seek(0, 0)
	// defer func() {
	// 	fd.Sync()
	// 	fd.Close()
	// }()
	// if err != nil {
	// 	glog.Fatalf("open %s, error: %v", fname, err)
	// }
	// fd.Write(header.rr.Bytes())
	// fd.Write(body.rr.Bytes())

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
				Name:       goEscape(f.Name),
				TypeParams: typeParam{s.TypeParams, false},
				Type:       in.GoType(f.TypeExpr),
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
		TypeParams: new_typeParams(u.TypeParams),
		Branches: slices.Map[goadl.Field, unionBranchParams](u.Fields, func(f goadl.Field) unionBranchParams {
			return unionBranchParams{
				Name:       goEscape(f.Name),
				TypeParams: new_typeParams(u.TypeParams),
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
		TypeParams: new_typeParams(td.TypeParams),
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
		TypeParams: new_typeParams(nt.TypeParams),
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
