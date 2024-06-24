package gen_go

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	goslices "slices"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadl_rt/v3/sys/types"

	// "github.com/adl-lang/goadlc/internal/fn/slices"
	"github.com/adl-lang/goadlc/internal/fn/slices"
	"github.com/adl-lang/goadlc/internal/gen_go/gomod"
	"github.com/adl-lang/goadlc/internal/gen_go/load"
	"github.com/adl-lang/goadlc/internal/root"
)

func NewGenGoV3(
	rt *root.RootObj,
	goCmd *gomod.GoCmd,
	loadCmd *load.LoadTask,
) any {
	// wk, err := os.MkdirTemp("", "goadlc-")
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, `WARNING: os.MkdirTemp("", "goadlc-") %v\n`, err)
	// }
	// // cwd, err := os.Getwd()
	// // if err != nil {
	// // 	fmt.Fprintf(os.Stderr, `WARNING: error getting current working directory %v\n`, err)
	// // }
	// cacheDir, err := os.UserCacheDir()
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, `WARNING: error getting UserCacheDir %v\n`, err)
	// }

	return &goadlcCmd{
		rt:     rt,
		Loader: *loadCmd,
		GoMod:  *goCmd,
		// Loader: load.LoadTask{
		// 	BundleMaps:   []load.BundleMap{},
		// 	WorkingDir:   wk,
		// 	MergeAdlext:  "adl-go",
		// 	UserCacheDir: filepath.Join(cacheDir, "adl-bundles"),
		// 	// Outputdir:    cwd,
		// },
		// GenAstInfo:  []GenAstInfo{},
		GoAdlPath: "github.com/adl-lang/goadl_rt/v3",
	}
}

type goadlcCmd struct {
	rt *root.RootObj

	Loader load.LoadTask
	GoMod  gomod.GoCmd

	ChangePWD  string `opts:"group=type" help:"The directory to change to after root read cfg but before running (used in dev)"`
	NoGoFmt    bool   `opts:"group=type" help:"Don't run 'go fmt' on the generated files"`
	GoAdlPath  string `opts:"group=type" help:"The path to the Go ADL runtime import"`
	ExcludeAst bool   `opts:"group=type,short=t" help:"Don't generate type expr, scoped decl and init registration functions"`
	StdLibGen  bool   `opts:"group=type" help:"Used for bootstrapping, only use when generating the sys.aldast & sys.types modules"`

	// NoOverwrite    bool     `help:"Don't update files that haven't changed"`
	// Manifest       string   `help:"Write a manifest file recording generated files"`
	// CombinedOutput string   `help:"The json file to which all adl modules will be written"`

	// files []string
}

type snResolver func(sn adlast.ScopedName) (*adlast.Decl, bool)

type baseGen struct {
	cli      *goadlcCmd
	resolver snResolver
	// typetoken_flds func(sn adlast.ScopedName, tbind []goadl.TypeBinding) []adlast.Field
	modulePath string
	midPath    string
	moduleName string
	imports    imports
}

func (in *goadlcCmd) Run() error {
	err := in.rt.Config(in)
	if err != nil {
		err = fmt.Errorf("  Error with config file. error : %v", err)
		return err
	}
	if in.ChangePWD != "" {
		err := os.Chdir(in.ChangePWD)
		if err != nil {
			return err
		}
	}
	res, err := in.Loader.Load()
	if err != nil {
		return err
	}
	gm, err := in.GoMod.Modpath()
	if err != nil {
		return err
	}
	return in.generate(res, gm)
}

func containsTypeToken(str adlast.Struct) bool {
	for _, fld := range str.Fields {
		if pr, ok := fld.TypeExpr.TypeRef.Cast_primitive(); ok && pr == "TypeToken" {
			return true
		}
	}
	return false
}

func (in *goadlcCmd) generate(
	lr *load.LoadResult,
	gm *gomod.GoModResult,
) error {
	resolver := func(sn adlast.ScopedName) (*adlast.Decl, bool) {
		if mod, ok := lr.CombinedAst[sn.ModuleName]; ok {
			decl, ok := mod.Decls[sn.Name]
			if !ok {
				panic(fmt.Errorf("%v", sn.Name))
			}
			return &decl, true
		}
		// resolve adlast, types & go_ even if not provided as input adl source
		si := goadl.RESOLVER.Resolve(sn)
		if si != nil {
			return &si.Decl, true
		}
		for k := range lr.CombinedAst {
			fmt.Printf("-- %v\n", k)
		}
		panic(fmt.Errorf("%v", sn.ModuleName))
	}

	for _, m := range lr.Modules {
		modCodeGenDir := strings.Split(m.Name, ".")
		// modCodeGenPkg := pkgFromImport(strings.ReplaceAll(m.name, ".", "/"))
		modCodeGenPkg := modCodeGenDir[len(modCodeGenDir)-1]
		path := in.GoMod.Outputdir + "/" + strings.Join(modCodeGenDir, "/")
		// for _, mm := range in.ModuleMap {
		// 	if mm.ModuleName == m.name {
		// 		modCodeGenPkg = mm.Name
		// 		if mm.RelOutputDir != nil {
		// 			path = filepath.Join(in.Outputdir, *mm.RelOutputDir)
		// 		}
		// 	}
		// }
		// baseGen := in.newBaseGen(resolver, declMap, importMap, modulePath, midPath, m.name)
		declBody := &generator{
			baseGen: in.newBaseGen(resolver, gm.ModulePath, gm.MidPath, m.Name),
			rr:      templateRenderer{t: templates},
		}
		astBody := &generator{
			baseGen: in.newBaseGen(resolver, gm.ModulePath, gm.MidPath, m.Name),
			rr:      templateRenderer{t: templates},
		}
		declsNames := []string{}
		for k := range m.Module.Decls {
			declsNames = append(declsNames, k)
		}
		goslices.Sort(declsNames)
		for _, k := range declsNames {
			decl := m.Module.Decls[k]
			jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_GoCustomType(), goadl.RESOLVER)
			gct, err := goadl.GetAnnotation(decl.Annotations, goCustomTypeSN, jb)
			if err != nil {
				panic(err)
			}
			if gct != nil {
				if !in.ExcludeAst {
					astBody.generalTexpr(astBody, decl)
					astBody.generalReg(astBody, decl)
				}
			} else {
				declBody.generalDeclV3(declBody, decl)
				if !in.ExcludeAst {
					astBody.generalTexpr(astBody, decl)
					astBody.generalReg(astBody, decl)
				}
			}
		}
		err := in.writeFile(m.Name, modCodeGenPkg, declBody, filepath.Join(path, modCodeGenDir[len(modCodeGenDir)-1]+".go"), in.NoGoFmt, false)
		if err != nil {
			return err
		}
		if !in.ExcludeAst {
			override := false
			fname := modCodeGenDir[len(modCodeGenDir)-1] + "_ast.go"
			if _, ok := in.specialTexpr()[m.Name]; ok && in.StdLibGen {
				err := in.writeFile(m.Name, "goadl", astBody, filepath.Join(in.GoMod.Outputdir, fname), in.NoGoFmt, true)
				if err != nil {
					return err
				}
				override = true
			}
			if !override {
				err := in.writeFile(m.Name, modCodeGenPkg, astBody, filepath.Join(path, fname), in.NoGoFmt, true)
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
	modulePath, midPath string,
	moduleName string,
) *baseGen {
	imports := newImports(
		in.reservedImports(),
		in.Loader.BundleMaps,
	)
	return &baseGen{
		cli:      in,
		resolver: resolver,
		// typetoken_flds: typetoken_flds,
		modulePath: modulePath,
		midPath:    midPath,
		moduleName: moduleName,
		imports:    imports,
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
	if _, ok := in.specialTexpr()[moduleName]; genAst && ok && in.StdLibGen {
		useImports = append(useImports, importSpec{
			Path:    filepath.Join(in.GoAdlPath, strings.ReplaceAll(moduleName, ".", "/")),
			Name:    ".",
			Aliased: true,
		})
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
	if in.rt.Debug {
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
		{Path: in.GoAdlPath + "/sys/adlast", Aliased: false, Name: "adlast"},
		{Path: in.GoAdlPath + "/adljson", Aliased: false, Name: "adljson"},
		{Path: in.GoAdlPath + "/customtypes", Aliased: false, Name: "customtypes"},
	}
}

func (in *goadlcCmd) specialTexpr() map[string]struct{} {
	return map[string]struct{}{
		"sys.adlast":      {},
		"sys.types":       {},
		"adlc.config.go_": {},
	}
}

type generator struct {
	*baseGen
	rr        templateRenderer
	genAdlAst bool
}

func (base *baseGen) generalDeclV3(
	in *generator,
	decl adlast.Decl,
) {
	typeParams := typeParamsFromDecl(decl)
	adlast.Handle_DeclType[any](
		decl.Type_,
		func(s adlast.Struct) any {

			in.rr.Render(structParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParams,
				Fields: slices.Map(s.Fields, func(f adlast.Field) fieldParams {
					return makeFieldParam(f, decl.Name, in)
				}),
				ContainsTypeToken: containsTypeToken(s),
			})
			return nil
		},
		func(u adlast.Union) any {
			in.rr.Render(unionParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParams,
				Branches: slices.Map[adlast.Field, fieldParams](u.Fields, func(f adlast.Field) fieldParams {
					return makeFieldParam(f, decl.Name, in)
				}),
			})
			return nil
		},
		func(td adlast.TypeDef) any {
			if typ, ok := decl.Type_.Cast_type_(); ok {
				if len(typ.TypeParams) != 0 {
					// in go "type X<A any> = ..." isn't valid, skipping
					return nil
				}
			}
			in.rr.Render(typeAliasParams{
				G:           in,
				Name:        decl.Name,
				TypeParams:  typeParams,
				TypeExpr:    td.TypeExpr,
				Annotations: decl.Annotations,
			})
			return nil
		},
		func(nt adlast.NewType) any {
			in.rr.Render(newTypeParams{
				G:           in,
				Name:        decl.Name,
				TypeParams:  typeParams,
				TypeExpr:    nt.TypeExpr,
				Annotations: decl.Annotations,
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
	if typ, ok := decl.Type_.Cast_type_(); ok {
		if len(typ.TypeParams) != 0 {
			// in go "type X<A any> = ..." isn't valid, skipping
			return
		}
	}
	type_name := decl.Name
	tp := typeParamsFromDecl(decl)

	jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_GoCustomType(), goadl.RESOLVER)
	gct, err := goadl.GetAnnotation(decl.Annotations, goCustomTypeSN, jb)
	if err != nil {
		panic(err)
	}
	if gct != nil {
		pkg := gct.Gotype.Import_path[strings.LastIndex(gct.Gotype.Import_path, "/")+1:]
		spec := importSpec{
			Path:    gct.Gotype.Import_path,
			Name:    gct.Gotype.Pkg,
			Aliased: gct.Gotype.Pkg != pkg,
		}
		base.imports.addSpec(spec)
		type_name = gct.Gotype.Pkg + "." + gct.Gotype.Name
		tp.type_constraints = gct.Gotype.Type_constraints
	}

	// tp.stdlib = base.cli.StdLibGen
	body.rr.Render(aTexprParams{
		G:          body,
		ModuleName: base.moduleName,
		Name:       decl.Name,
		TypeName:   type_name,
		TypeParams: tp,
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
	})
}

func makeFieldParam(
	f adlast.Field,
	declName string,
	gen *generator,
) fieldParams {
	isVoid := false
	if pr, ok := f.TypeExpr.TypeRef.Cast_primitive(); ok {
		if pr == "Void" {
			isVoid = true
		}
	}
	return types.Handle_Maybe[any, fieldParams](
		f.Default,
		func(nothing struct{}) fieldParams {
			return fieldParams{
				Field:      f,
				DeclName:   declName,
				G:          gen,
				HasDefault: false,
				IsVoid:     isVoid,
			}
		},
		func(just any) fieldParams {
			return fieldParams{
				Field:      f,
				DeclName:   declName,
				G:          gen,
				HasDefault: true,
				Just:       just,
				IsVoid:     isVoid,
			}
		},
		nil,
	)
}

func (in *generator) ToTitle(s string) string {
	return strings.ToTitle(s)
}
