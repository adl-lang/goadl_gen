package gen_go_v2

import (
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	goslices "slices"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadl_rt/v3/sys/types"
	"github.com/adl-lang/goadlc/internal/fn/slices"
	"github.com/adl-lang/goadlc/internal/root"
	"github.com/golang/glog"
)

func NewGenGoV3(rt *root.RootObj) any {
	wk, err := os.MkdirTemp("", "goadlc-")
	if err != nil {
		glog.Warningf(`os.MkdirTemp("", "goadlc-") %v`, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		glog.Warningf(`error getting current working directory %v`, err)
	}
	return &goadlcCmd{
		rt:         rt,
		WorkingDir: wk,
		Outputdir:  cwd,
		ModuleMap:  []ImportMap{},
		GenAstInfo: []GenAstInfo{},
		GoAdlPath:  "github.com/adl-lang/goadl_rt/v2",
	}
}

type goadlcCmd struct {
	rt          *root.RootObj
	WorkingDir  string
	Searchdir   []string    `opts:"short=I" help:"Add the specifed directory to the ADL searchpath"`
	Outputdir   string      `opts:"short=O" help:"Set the directory where generated code is written "`
	MergeAdlext string      `help:"Add the specifed adl file extension to merged on loading"`
	Debug       bool        `help:"Print extra diagnostic information, especially about files being read/written"`
	NoGoFmt     bool        `help:"Don't run 'go fmt' on the generated files"`
	GoAdlPath   string      `help:"The path to the Go ADL runtime import"`
	ModulePath  string      `help:"The path of the Go module for the generated code. Overrides the module-path from the '--go-mod-file' flag."`
	GoModFile   string      `help:"Path of a go.mod file. If the file exists, the module-path is used for generated imports."`
	ExcludeAst  bool        `opts:"short=t" help:"Don't generate type expr, scoped decl and init registration functions"`
	ModuleMap   ImportMaps  `opts:"short=M" help:"Mapping from ADL module name to Go import specifiction"`
	GenAstInfo  GenAstInfos `help:"Mapping from ADL module name to details on where to generate ast info. Of the form [modulename:goPkg:outputFile]"`
	StdLibGen   bool        `help:"Used for bootstrapping, only use when generating the sys.aldast & sys.types modules"`

	// NoOverwrite    bool     `help:"Don't update files that haven't changed"`
	// Manifest       string   `help:"Write a manifest file recording generated files"`
	// CombinedOutput string   `help:"The json file to which all adl modules will be written"`

	Files []string `opts:"mode=arg" help:"File or pattern"`
	files []string
}

type GenAstInfos []GenAstInfo

type GenAstInfo struct {
	ModuleName    string
	Pkg           string
	RelOutputFile string
}

func (ims *GenAstInfos) Set(text string) error {
	panic("method only here to make opts happy")
}

func (im *GenAstInfo) Set(text string) error {
	parts := strings.Split(text, `:`)
	lp := len(parts)
	if lp != 3 {
		return fmt.Errorf("expecting module to go map of the form [modulename:goPkg:outputFile]")
	}
	im.ModuleName = parts[0]
	im.Pkg = parts[1]
	im.RelOutputFile = parts[2]
	return nil
}

type ImportMaps []ImportMap

func (ims *ImportMaps) Set(text string) error {
	panic("method only here to make opts happy")
}

type ImportMap struct {
	ModuleName   string
	Name         string
	Path         string
	RelOutputDir *string `json:",omitempty"`
	alias        bool
}

func (im *ImportMap) Set(text string) error {
	parts := strings.Split(text, `:`)
	lp := len(parts)
	if lp < 2 || lp > 4 {
		return fmt.Errorf("expecting module to go map of the form [module:path] or [module:name:path] or [module:name:path:rel_output_dir]")
	}
	im.ModuleName = parts[0]
	if lp == 2 {
		im.Path = parts[1]
		im.Name = pkgFromImport(im.Path)
	}
	if lp >= 3 {
		im.Name = parts[1]
		im.Path = parts[2]
		im.alias = true
	}
	if lp == 4 {
		im.RelOutputDir = &parts[3]
	}
	return nil
}

type snResolver func(sn adlast.ScopedName) (*adlast.Decl, bool)

type baseGen struct {
	cli      *goadlcCmd
	resolver snResolver
	// declMap    map[string]adlast.Decl
	modulePath string
	midPath    string
	moduleName string
	// name       string
	imports   imports
	goAdlPath string
	stdLibGen bool
}

func (in *goadlcCmd) Run() error {
	in.rt.Config(in)

	importMap := map[string]importSpec{}
	for _, im := range in.ModuleMap {
		if _, ok := importMap[im.ModuleName]; ok {
			return fmt.Errorf("duplicate module in --module-map '%s'", im.ModuleName)
		}
		importMap[im.ModuleName] = importSpec{
			Path:    im.Path,
			Name:    im.Name,
			Aliased: im.alias,
		}
	}

	cwd, err := os.Getwd()
	dFs := os.DirFS(cwd)
	if err != nil {
		return fmt.Errorf("can't get cwd : %v", err)
	}
	if len(in.Files) == 0 {
		return fmt.Errorf("no file or pattern specified")
	}
	for _, p := range in.Files {
		matchs, err := fs.Glob(dFs, p)
		if err != nil {
			return fmt.Errorf("error globbing file : %v", err)
		}
		in.files = append(in.files, matchs...)
	}
	if in.Debug {
		fmt.Fprintf(os.Stderr, "found files:\n")
		for _, f := range in.files {
			fmt.Fprintf(os.Stderr, "  %v\n", f)
		}
	}

	jb := func(fd io.Reader) (map[string]adlast.Module, map[string]adlast.Decl, error) {
		combinedAst := make(map[string]adlast.Module)
		declMap := make(map[string]adlast.Decl)
		dec := goadl.NewDecoder(fd, goadl.Texpr_StringMap[adlast.Module](goadl.Texpr_Module()), goadl.RESOLVER)
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

	modules := []namedModule{}
	combinedAst, declMap, err := loadAdl(in, &modules, jb)
	_ = combinedAst
	// _ = declMap
	if err != nil {
		os.Exit(1)
	}

	modulePath, midPath, err := in.modpath()
	if err != nil {
		return err
	}

	resolver := func(sn adlast.ScopedName) (*adlast.Decl, bool) {
		mod, ok := combinedAst[sn.ModuleName]
		if !ok {
			si := goadl.RESOLVER.Resolve(sn)
			if si != nil {
				return &si.SD.Decl, true
			}
			for k := range combinedAst {
				fmt.Printf("-- %v\n", k)
			}
			panic(fmt.Errorf("%v", sn.ModuleName))
			return nil, false
		}
		decl, ok := mod.Decls[sn.Name]
		if !ok {
			panic(fmt.Errorf("%v", sn.Name))
			return nil, false
		}
		return &decl, true
	}

	for _, m := range modules {
		modCodeGenDir := strings.Split(m.name, ".")
		// modCodeGenPkg := pkgFromImport(strings.ReplaceAll(m.name, ".", "/"))
		modCodeGenPkg := modCodeGenDir[len(modCodeGenDir)-1]
		path := in.Outputdir + "/" + strings.Join(modCodeGenDir, "/")
		for _, mm := range in.ModuleMap {
			if mm.ModuleName == m.name {
				modCodeGenPkg = mm.Name
				if mm.RelOutputDir != nil {
					path = filepath.Join(in.Outputdir, *mm.RelOutputDir)
				}
			}
		}
		// baseGen := in.newBaseGen(resolver, declMap, importMap, modulePath, midPath, m.name)
		declBody := &generator{
			baseGen: in.newBaseGen(resolver, declMap, importMap, modulePath, midPath, m.name),
			rr:      templateRenderer{t: templates},
		}
		astBody := &generator{
			baseGen: in.newBaseGen(resolver, declMap, importMap, modulePath, midPath, m.name),
			rr:      templateRenderer{t: templates},
		}
		declsNames := []string{}
		for k := range m.module.Decls {
			declsNames = append(declsNames, k)
		}
		goslices.Sort(declsNames)
		for _, k := range declsNames {
			decl := m.module.Decls[k]
			declBody.generalDeclV3(declBody, decl)
			if !in.ExcludeAst {
				astBody.generalTexpr(astBody, decl)
				astBody.generalReg(astBody, decl)
			}
		}
		err := in.writeFile(m.name, modCodeGenPkg, declBody, filepath.Join(path, modCodeGenDir[len(modCodeGenDir)-1]+".go"), in.NoGoFmt, false)
		if err != nil {
			return err
		}
		if !in.ExcludeAst {
			override := false
			for _, astinfo := range in.GenAstInfo {
				if astinfo.ModuleName == m.name {
					err := in.writeFile(m.name, astinfo.Pkg, astBody, filepath.Join(in.Outputdir, astinfo.RelOutputFile), in.NoGoFmt, true)
					if err != nil {
						return err
					}
					override = true
					break
				}
			}
			if !override {
				err := in.writeFile(m.name, modCodeGenPkg, astBody, filepath.Join(path, modCodeGenDir[len(modCodeGenDir)-1]+"_ast.go"), in.NoGoFmt, true)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (in *goadlcCmd) newBaseGen(
	resolver func(sn adlast.ScopedName) (*adlast.Decl, bool),
	declMap map[string]adlast.Decl,
	importMap map[string]importSpec,
	modulePath, midPath string,
	moduleName string,
	//  name string,

) *baseGen {
	imports := newImports(
		in.reservedImports(),
		importMap,
	)
	return &baseGen{
		cli:      in,
		resolver: resolver,
		// declMap:    declMap,
		modulePath: modulePath,
		midPath:    midPath,
		moduleName: moduleName,
		// name:       name,
		imports:   imports,
		goAdlPath: in.GoAdlPath,
		stdLibGen: in.StdLibGen,
	}
}

func (in *goadlcCmd) writeFile(
	moduleName string,
	modCodeGenPkg string,
	body *generator,
	path string,
	noGoFmt bool,
	genAst bool,
) error {
	var err error
	dir, file := filepath.Split(path)
	_ = file

	if d, err := os.Stat(dir); err != nil {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	} else {
		if !d.IsDir() {
			return fmt.Errorf("directory expected %v", dir)
		}
	}

	header := &generator{
		baseGen: body.baseGen,
		rr:      templateRenderer{t: templates},
	}
	header.rr.Render(headerParams{
		Pkg: modCodeGenPkg,
	})
	useImports := []importSpec{}
	for _, spec := range body.imports.specs {
		if body.imports.used[spec.Path] {
			useImports = append(useImports, spec)
		}
	}
	if in.StdLibGen && genAst {
		if moduleName == "sys.adlast" {
			useImports = append(useImports, importSpec{
				Path:    in.GoAdlPath + "/sys/adlast",
				Name:    ".",
				Aliased: true,
			})
		}
		if moduleName == "sys.types" {
			useImports = append(useImports, importSpec{
				Path:    in.GoAdlPath + "/sys/types",
				Name:    ".",
				Aliased: true,
			})
		}
	}

	header.rr.Render(importsParams{
		Imports: useImports,
	})
	header.rr.buf.Write(body.rr.Bytes())
	unformatted := header.rr.Bytes()

	var formatted []byte
	if !noGoFmt {
		formatted, err = format.Source(unformatted)
		if err != nil {
			formatted = unformatted
		}
	} else {
		formatted = unformatted
	}
	var fd *os.File = nil
	fd, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}
	err = fd.Truncate(0)
	if err != nil {
		return err
	}
	_, err = fd.Seek(0, 0)
	if err != nil {
		return err
	}
	defer func() {
		fd.Sync()
		fd.Close()
	}()
	_, err = fd.Write(formatted)
	if in.Debug {
		fmt.Fprintf(os.Stderr, "wrote file %s\n", path)
	}
	return err
}

func (in *goadlcCmd) reservedImports() []importSpec {
	return []importSpec{
		{Path: "encoding/json"},
		{Path: "reflect"},
		{Path: "strings"},
		{Path: "fmt"},
		{Path: in.GoAdlPath, Aliased: true, Name: "goadl"},
		{Path: in.GoAdlPath + "/sys/adlast", Aliased: true, Name: "adlast"},
	}
}

func (bg *baseGen) Import(pkg string) (string, error) {
	if bg.stdLibGen && pkg == "goadl" {
		return "", nil
	}
	if spec, ok := bg.imports.byName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	} else {
		bg.imports.addPath(spec.Path)
		return spec.Name + ".", nil
	}
}

type generator struct {
	*baseGen
	rr templateRenderer
}

func (base *baseGen) generalDeclV3(
	in *generator,
	decl adlast.Decl,
) {
	adlast.Handle_DeclType[any](
		decl.Type_.Branch,
		func(s adlast.Struct) any {
			in.rr.Render(structParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParam{s.TypeParams, false, base.stdLibGen},
				Fields: slices.Map(s.Fields, func(f adlast.Field) fieldParams {
					return makeFieldParam(f)
				}),
			})
			return nil
		},
		func(u adlast.Union) any {
			in.rr.Render(unionParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParam{u.TypeParams, false, base.stdLibGen},
				Branches: slices.Map[adlast.Field, fieldParams](u.Fields, func(f adlast.Field) fieldParams {
					return makeFieldParam(f)
				}),
			})
			return nil
		},
		func(td adlast.TypeDef) any {
			in.rr.Render(typeAliasParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParam{td.TypeParams, false, base.stdLibGen},
				RType:      in.GoType(td.TypeExpr),
			})
			return nil
		},
		func(nt adlast.NewType) any {
			in.rr.Render(newTypeParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParam{nt.TypeParams, false, base.stdLibGen},
				RType:      in.GoType(nt.TypeExpr),
			})
			return nil
		},
		nil,
	)
}

func (base *baseGen) generalTexpr(
	body *generator,
	decl adlast.Decl,
) {
	tp := typeParamsFromDecl(decl)
	tp.stdlib = base.stdLibGen
	body.rr.Render(texprParams{
		G:          body,
		ModuleName: base.moduleName,
		Name:       decl.Name,
		TypeParams: tp,
		Decl:       decl,
	})
}

func (base *baseGen) generalReg(
	body *generator,
	decl adlast.Decl,
) {
	tp := typeParamsFromDecl(decl)
	body.rr.Render(scopedDeclParams{
		G:          body,
		ModuleName: base.moduleName,
		Name:       decl.Name,
		Decl:       decl,
		TypeParams: tp,
		// this is needed to generate registration info for encoding of branches
		// only needed for unions
		Fields: adlast.Handle_DeclType[[]fieldParams](
			decl.Type_.Branch,
			func(struct_ adlast.Struct) []fieldParams {
				return []fieldParams{}
			},
			func(u adlast.Union) []fieldParams {
				return slices.Map[adlast.Field, fieldParams](u.Fields, func(f adlast.Field) fieldParams {
					return makeFieldParam(f)
				})
			},
			func(type_ adlast.TypeDef) []fieldParams {
				return []fieldParams{}
			},
			func(newtype_ adlast.NewType) []fieldParams {
				return []fieldParams{}
			},
			nil,
		),
	})
}

func makeFieldParam(f adlast.Field) fieldParams {
	return types.Handle_Maybe[any, fieldParams](
		f.Default.Branch,
		func(nothing struct{}) fieldParams {
			return fieldParams{
				Field:      f,
				HasDefault: false,
			}
		},
		func(just any) fieldParams {
			// fmt.Printf("???????1 %v\n", reflect.TypeOf(just))
			// val := reflect.ValueOf(just).Interface()
			return fieldParams{
				Field:      f,
				HasDefault: true,
				Just:       just,
			}
		},
		nil,
	)
}

func (in *generator) ToTitle(s string) string {
	return strings.ToTitle(s)
}

func jsonPrimitiveDefaultToGo(primitive string, defVal interface{}) string {
	switch defVal.(type) {
	case string:
		return fmt.Sprintf(`%q`, defVal)
	}
	return fmt.Sprintf(`%v`, defVal)
}

func defunctionalizeTe(m map[string]adlast.TypeExpr, te adlast.TypeExpr) adlast.TypeExpr {

	p0 := slices.Map[adlast.TypeExpr, adlast.TypeExpr](te.Parameters, func(a adlast.TypeExpr) adlast.TypeExpr {
		return defunctionalizeTe(m, a)
	})

	if tp, ok := te.TypeRef.Branch.(adlast.TypeRef_TypeParam); ok {
		if te0, ok := m[tp.V]; !ok {
			panic(fmt.Errorf("type param not found"))
			// return adlast.TypeExpr{
			// 	TypeRef:    te.TypeRef,
			// 	Parameters: p0,
			// }
		} else {
			if len(te.Parameters) != 0 {
				panic(fmt.Errorf("type param cannot have type params, not a concrete type"))
			}
			return te0
		}
	}

	return adlast.TypeExpr{
		TypeRef:    te.TypeRef,
		Parameters: p0,
	}
}
