package gotypes

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadl_rt/v3/sys/types"
	"github.com/adl-lang/goadlc/internal/cli/gogen"
	"github.com/adl-lang/goadlc/internal/cli/goimports"
	"github.com/adl-lang/goadlc/internal/cli/gomod"
	"github.com/adl-lang/goadlc/internal/cli/loader"

	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

func (in *GoTypes) Run() error {
	lr := in.Loader
	gm := in.GoMod
	eg := &errgroup.Group{}
	for _, m := range lr.Modules {
		fn := thunk_gen_module(m, in, gm)
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
		// if in.Root.Debug {
		// 	fmt.Fprintf(os.Stderr, "out: '%s' root: '%s'\n", oabs, rabs)
		// }
		var midPath string
		if oabs != rabs {
			midPath = oabs[len(rabs)+1:]
		}
		path := in.Outputdir + "/" + strings.Join(modCodeGenDir, "/")
		declBody := &gogen.Generator{
			BaseGen: gogen.NewBaseGen(gm.ModulePath, midPath, m.Name, in, *in.Loader),
			Rr:      gogen.TemplateRenderer{Tmpl: templates},
		}
		astBody := &gogen.Generator{
			BaseGen: gogen.NewBaseGen(gm.ModulePath, midPath, m.Name, in, *in.Loader),
			Rr:      gogen.TemplateRenderer{Tmpl: templates},
		}
		declsNames := []string{}
		for k := range m.Module_.Decls {
			declsNames = append(declsNames, k)
		}
		slices.Sort(declsNames)
		for _, k := range declsNames {
			decl := m.Module_.Decls[k]
			jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_GoCustomType(), goadl.RESOLVER)
			gct, err := goadl.GetAnnotation(decl.Annotations, gogen.GoCustomTypeSN, jb)
			if err != nil {
				panic(err)
			}
			if gct != nil {
				if !in.ExcludeAst {
					generalTexpr(astBody, decl)
					generalReg(astBody, decl)
				}
			} else {
				generalDeclV3(declBody, decl)
				if !in.ExcludeAst {
					generalTexpr(astBody, decl)
					generalReg(astBody, decl)
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

func (in *GoTypes) writeFile(
	moduleName string,
	modCodeGenPkg string,
	body *gogen.Generator,
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

	header := &gogen.Generator{
		BaseGen: body.BaseGen,
		Rr:      gogen.TemplateRenderer{Tmpl: templates},
	}
	header.Rr.Render(headerParams{
		Pkg: modCodeGenPkg,
	})
	useImports := []goimports.ImportSpec{}
	for _, spec := range body.Imports.Specs {
		if body.Imports.Used[spec.Path] {
			useImports = append(useImports, spec)
		}
	}
	if _, ok := in.specialTexpr()[moduleName]; genAst && ok && in.StdLibGen {
		useImports = append(useImports, goimports.ImportSpec{
			Path:    filepath.Join(in.GoAdlPath, strings.ReplaceAll(moduleName, ".", "/")),
			Name:    ".",
			Aliased: true,
		})
	}

	header.Rr.Render(importsParams{
		Imports: useImports,
	})
	header.Rr.Buf.Write(body.Rr.Bytes())
	unformatted := header.Rr.Bytes()

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

func (in *GoTypes) ReservedImports() []goimports.ImportSpec {
	return []goimports.ImportSpec{
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

func (bg *GoTypes) GoImport(pkg string, currModuleName string, imports goimports.Imports) (string, error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in GoImport %v\n%v", r, string(debug.Stack()))
			panic(r)
		}
	}()
	if _, ok := bg.specialTexpr()[currModuleName]; ok && bg._GoTypes.StdLibGen && pkg == "goadl" {
		return "", nil
	}
	if spec, ok := imports.ByName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	} else {
		imports.AddPath(spec.Path)
		return spec.Name + ".", nil
	}
}

func (bg *GoTypes) IsStdLibGen() bool {
	return bg._GoTypes.StdLibGen
}

func (bg *GoTypes) GoAdlImportPath() string {
	return bg._GoTypes.GoAdlPath
}

func generalDeclV3(
	in *gogen.Generator,
	decl adlast.Decl,
) {
	typeParams := gogen.TypeParamsFromDecl(decl)
	adlast.Handle_DeclType[any](
		decl.Type_,
		func(s adlast.Struct) any {

			in.Rr.Render(structParams{
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
			in.Rr.Render(unionParams{
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
			in.Rr.Render(typeAliasParams{
				G:           in,
				Name:        decl.Name,
				TypeParams:  typeParams,
				TypeExpr:    td.TypeExpr,
				Annotations: decl.Annotations,
			})
			return nil
		},
		func(nt adlast.NewType) any {
			in.Rr.Render(newTypeParams{
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

func generalTexpr(
	body *gogen.Generator,
	decl adlast.Decl,
) {
	if typ, ok := decl.Type_.Cast_type_(); ok {
		if len(typ.TypeParams) != 0 {
			// in go "type X<A any> = ..." isn't valid, skipping
			return
		}
	}
	type_name := decl.Name
	tp := gogen.TypeParamsFromDecl(decl)

	jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_GoCustomType(), goadl.RESOLVER)
	gct, err := goadl.GetAnnotation(decl.Annotations, gogen.GoCustomTypeSN, jb)
	if err != nil {
		panic(err)
	}
	if gct != nil {
		pkg := gct.Gotype.Import_path[strings.LastIndex(gct.Gotype.Import_path, "/")+1:]
		spec := goimports.ImportSpec{
			Path:    gct.Gotype.Import_path,
			Name:    gct.Gotype.Pkg,
			Aliased: gct.Gotype.Pkg != pkg,
		}
		body.Imports.AddSpec(spec)
		type_name = gct.Gotype.Pkg + "." + gct.Gotype.Name
		tp.TypeConstraints = gct.Gotype.Type_constraints
	}

	// tp.stdlib = base.cli.StdLibGen
	body.Rr.Render(aTexprParams{
		G:          body,
		ModuleName: body.ModuleName,
		Name:       decl.Name,
		TypeName:   type_name,
		TypeParams: tp,
	})
}

func generalReg(
	body *gogen.Generator,
	decl adlast.Decl,
) {
	tp := gogen.TypeParamsFromDecl(decl)
	body.Rr.Render(scopedDeclParams{
		G:          body,
		ModuleName: body.ModuleName,
		Name:       decl.Name,
		Decl:       decl,
		TypeParams: tp,
	})
}

func makeFieldParam(
	f adlast.Field,
	declName string,
	gen *gogen.Generator,
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

func containsTypeToken(str adlast.Struct) bool {
	for _, fld := range str.Fields {
		if pr, ok := fld.TypeExpr.TypeRef.Cast_primitive(); ok && pr == "TypeToken" {
			return true
		}
	}
	return false
}
