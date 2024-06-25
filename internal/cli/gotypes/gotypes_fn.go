package gotypes

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadl_rt/v3/sys/types"
	"github.com/adl-lang/goadlc/internal/cli/gomod"
	"github.com/adl-lang/goadlc/internal/cli/loader"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

func (in *GoTypes) Run() error {
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
	gm, err := in.GoMod.Modpath(in.Root.Debug)
	if err != nil {
		return err
	}
	return in.generate(res, gm)
}

func (in *GoTypes) generate(
	lr *loader.LoadResult,
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

	eg := &errgroup.Group{}
	for _, m := range lr.Modules {
		fn := thunk_gen_module(m, in, resolver, gm)
		// fn()
		eg.Go(fn)
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("error generating module : %w", err)
	}

	return nil
}

func thunk_gen_module(
	m loader.NamedModule,
	in *GoTypes,
	resolver func(sn adlast.ScopedName) (*adlast.Decl, bool),
	gm *gomod.GoModResult,
) func() error {
	fn := func() error {
		modCodeGenDir := strings.Split(m.Name, ".")
		modCodeGenPkg := modCodeGenDir[len(modCodeGenDir)-1]

		oabs, err1 := filepath.Abs(in.Outputdir)
		rabs, err2 := filepath.Abs(gm.RootDir)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("error get abs dir %w %w", err1, err2)
		}
		if !strings.HasPrefix(oabs, rabs) {
			return fmt.Errorf("output dir must be inside root of go.mod out: %s root: %s", oabs, rabs)
		}
		midPath := oabs[len(rabs):]
		if in.Root.Debug {
			fmt.Fprintf(os.Stderr, "midpath '%s'\n", midPath)
		}

		path := in.Outputdir + "/" + strings.Join(modCodeGenDir, "/")
		declBody := &generator{
			baseGen: in.newBaseGen(resolver, gm.ModulePath, midPath, m.Name),
			rr:      templateRenderer{t: templates},
		}
		astBody := &generator{
			baseGen: in.newBaseGen(resolver, gm.ModulePath, midPath, m.Name),
			rr:      templateRenderer{t: templates},
		}
		declsNames := []string{}
		for k := range m.Module_.Decls {
			declsNames = append(declsNames, k)
		}
		slices.Sort(declsNames)
		for _, k := range declsNames {
			decl := m.Module_.Decls[k]
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
				err := in.writeFile(m.Name, "goadl", astBody, filepath.Join(in.Outputdir, fname), in.NoGoFmt, true)
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
		return nil
	}
	return fn
}

func (in *GoTypes) newBaseGen(
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

func (in *GoTypes) writeFile(
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
	if in.Root.Debug {
		fmt.Fprintf(os.Stderr, "wrote file %s\n", path)
	}
	return err
}

func (in *GoTypes) reservedImports() []importSpec {
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

func (in *GoTypes) specialTexpr() map[string]struct{} {
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
				Fields: lo.Map(s.Fields, func(f adlast.Field, _ int) fieldParams {
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
				Branches: lo.Map[adlast.Field, fieldParams](u.Fields, func(f adlast.Field, _ int) fieldParams {
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

func containsTypeToken(str adlast.Struct) bool {
	for _, fld := range str.Fields {
		if pr, ok := fld.TypeExpr.TypeRef.Cast_primitive(); ok && pr == "TypeToken" {
			return true
		}
	}
	return false
}
