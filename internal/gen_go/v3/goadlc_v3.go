package gen_go

import (
	"archive/zip"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"path/filepath"
	goslices "slices"
	"sort"
	"strings"

	"github.com/mattn/go-zglob"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadl_rt/v3/sys/types"
	"github.com/adl-lang/goadlc/internal/fn/slices"
	"github.com/adl-lang/goadlc/internal/root"
)

func NewGenGoV3(rt *root.RootObj) any {
	wk, err := os.MkdirTemp("", "goadlc-")
	if err != nil {
		fmt.Fprintf(os.Stderr, `WARNING: os.MkdirTemp("", "goadlc-") %v\n`, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, `WARNING: error getting current working directory %v\n`, err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, `WARNING: error getting UserCacheDir %v\n`, err)
	}

	return &goadlcCmd{
		rt:           rt,
		UserCacheDir: filepath.Join(cacheDir, "adl-bundles"),
		WorkingDir:   wk,
		Outputdir:    cwd,
		// ModuleMap:  []ImportMap{},
		BundleMap: []BundleMap{},
		// GenAstInfo:  []GenAstInfo{},
		GoAdlPath:   "github.com/adl-lang/goadl_rt/v3",
		MergeAdlext: "adl-go",
	}
}

type goadlcCmd struct {
	rt *root.RootObj

	UserCacheDir string   `help:"The directory used to place cached files (e.g. download adl source)."`
	WorkingDir   string   `help:"The temp directory used to place intermediate files."`
	ChangePWD    string   `help:"The directory to change to after root read cfg but before running (used in dev)"`
	Searchdir    []string `opts:"short=I" help:"Add the specifed directory to the ADL searchpath"`
	Outputdir    string   `opts:"short=O" help:"Set the directory where generated code is written "`
	MergeAdlext  string   `help:"Add the specifed adl file extension to merged on loading"`
	Debug        bool     `help:"Print extra diagnostic information, especially about files being read/written"`
	NoGoFmt      bool     `help:"Don't run 'go fmt' on the generated files"`
	GoAdlPath    string   `help:"The path to the Go ADL runtime import"`
	ModulePath   string   `help:"The path of the Go module for the generated code. Overrides the module-path from the '--go-mod-file' flag."`
	GoModFile    string   `help:"Path of a go.mod file. If the file exists, the module-path is used for generated imports."`
	ExcludeAst   bool     `opts:"short=t" help:"Don't generate type expr, scoped decl and init registration functions"`
	// ModuleMap   ImportMaps `opts:"short=M" help:"Mapping from ADL module name to Go import specifiction"`
	BundleMap BundleMaps `help:"Mapping from ADL bundle to go module. Of the form [module_prefix:go_module_path]"`
	StdLibGen bool       `help:"Used for bootstrapping, only use when generating the sys.aldast & sys.types modules"`

	// NoOverwrite    bool     `help:"Don't update files that haven't changed"`
	// Manifest       string   `help:"Write a manifest file recording generated files"`
	// CombinedOutput string   `help:"The json file to which all adl modules will be written"`

	Files []string `opts:"mode=arg" help:"File or pattern"`
	files []string
}

type BundleMaps []BundleMap

type BundleMap struct {
	AdlModuleNamePrefix string
	GoModPath           string
	AdlSrc              string
	GoModVersion        *string
	Path                *string
}

func (ims *BundleMaps) Set(text string) error {
	panic("method only here to make opts happy")
}

type ImportMaps []ImportMap

func (im *BundleMap) Set(text string) error {
	parts := strings.Split(text, `|`)
	lp := len(parts)
	if lp < 2 || lp > 4 {
		return fmt.Errorf("expecting bundle to go map of the form [module_prefix|go_module_path|adl_src]")
	}
	im.AdlModuleNamePrefix = parts[0]
	im.GoModPath = parts[1]
	if lp >= 3 {
		im.GoModVersion = &parts[2]
	}
	return nil
}

func (ims *ImportMaps) Set(text string) error {
	panic("method only here to make opts happy")
}

type ImportMap struct {
	ModuleName string
	Name       string
	Path       string
	// RelOutputDir *string `json:",omitempty"`
	alias bool
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
	// if lp == 4 {
	// 	im.RelOutputDir = &parts[3]
	// }
	return nil
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
	return in.generate(in.setup())
}

func (in *goadlcCmd) zippedBundle(bm BundleMap) (string, error) {
	path := bm.AdlSrc[len("https://"):strings.LastIndex(bm.AdlSrc, "/")]
	file := bm.AdlSrc[strings.LastIndex(bm.AdlSrc, "/"):]
	zipdir := filepath.Join(in.UserCacheDir, "download", path)
	zipfile := filepath.Join(zipdir, file)
	if _, err := os.Stat(zipfile); err != nil {
		if err := os.MkdirAll(zipdir, 0777); err != nil {
			return "", fmt.Errorf("error creating dir for zip adlsrc '%s' err: %w", zipdir, err)
		}
		if in.Debug {
			fmt.Fprintf(os.Stderr, "created zip download dir '%s'\n", zipdir)
		}
		file, err := os.Create(zipfile)
		if err != nil {
			return "", err
		}
		defer file.Close()
		resp, err := http.Get(bm.AdlSrc)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("bad status: %s", resp.Status)
		}
		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return "", err
		}
		zarch, err := zip.OpenReader(zipfile)
		if err != nil {
			return "", err
		}
		if in.Debug {
			fmt.Printf("Unzipping %v\n", zipfile)
		}

		cachePath := bm.AdlSrc[len("https://") : len(bm.AdlSrc)-len(".zip")]
		cacheDir := filepath.Join(in.UserCacheDir, "cache", cachePath)
		if err := os.MkdirAll(cacheDir, 0777); err != nil {
			return "", fmt.Errorf("error creating dir for cache adlsrc '%s' err: %w", cacheDir, err)
		}
		for _, zf := range zarch.File {
			if zf.FileInfo().IsDir() {
				continue
			}
			name := zf.Name[strings.Index(zf.Name, "/")+1:]
			if in.Debug {
				fmt.Printf("  %v\n", name)
			}
			dst := filepath.Join(cacheDir, name)
			if err := os.MkdirAll(filepath.Dir(dst), 0777); err != nil {
				return "", err
			}
			w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0444)
			if err != nil {
				return "", err
			}
			r, err := zf.Open()
			if err != nil {
				w.Close()
				return "", err
			}
			lr := &io.LimitedReader{R: r, N: int64(zf.UncompressedSize64) + 1}
			_, err = io.Copy(w, lr)
			r.Close()
			if err != nil {
				w.Close()
				return "", err
			}
			if err := w.Close(); err != nil {
				return "", err
			}
			if lr.N <= 0 {
				return "", fmt.Errorf("uncompressed size of file %s is larger than declared size (%d bytes)", zf.Name, zf.UncompressedSize64)
			}
		}
		return cacheDir, nil
	}

	if in.Debug {
		fmt.Fprintf(os.Stderr, "cached zip  '%s'\n", zipfile)
	}
	cachePath := bm.AdlSrc[len("https://") : len(bm.AdlSrc)-len(".zip")]
	cacheDir := filepath.Join(in.UserCacheDir, "cache", cachePath)
	return cacheDir, nil
}

func (in *goadlcCmd) setup() (
	combinedAst map[string]adlast.Module,
	// importMap map[string]importSpec,
	modulePath string,
	midPath string,
	modules []namedModule,
	setupErr error,
) {
	setupErr = in.rt.Config(in)
	if setupErr != nil {
		setupErr = fmt.Errorf("  Error with config file. error : %v", setupErr)
		return
	}

	if in.ChangePWD != "" {
		err := os.Chdir(in.ChangePWD)
		if err != nil {
			setupErr = err
			return
		}
	}

	for _, bm := range in.BundleMap {
		if strings.HasPrefix(bm.AdlSrc, "file://") {
			in.Searchdir = append(in.Searchdir, bm.AdlSrc[len("file://"):])
		}
		if strings.HasPrefix(bm.AdlSrc, "https://") && strings.HasSuffix(bm.AdlSrc, ".zip") {
			path, err := in.zippedBundle(bm)
			if err != nil {
				setupErr = err
				return
			}
			if bm.Path != nil {
				in.Searchdir = append(in.Searchdir, filepath.Join(path, *bm.Path))
			} else {
				in.Searchdir = append(in.Searchdir, path)
			}
		}
		// if bm.AdlSrc == "adlstdlib" {
		// }
	}

	// importMap = map[string]importSpec{}
	// for _, im := range in.ModuleMap {
	// 	if _, ok := importMap[im.ModuleName]; ok {
	// 		setupErr = fmt.Errorf("duplicate module in --module-map '%s'", im.ModuleName)
	// 		return
	// 	}
	// 	importMap[im.ModuleName] = importSpec{
	// 		Path:    im.Path,
	// 		Name:    im.Name,
	// 		Aliased: im.alias,
	// 	}
	// }

	if len(in.Files) == 0 {
		setupErr = fmt.Errorf("no file or pattern specified")
		return
	}
	for _, p := range in.Files {
		matchs, err := zglob.Glob(p)
		sort.Strings(matchs)
		if err != nil {
			setupErr = fmt.Errorf("error globbing file : %v", err)
			return
		}
		in.files = append(in.files, matchs...)
	}
	if len(in.files) == 0 {
		setupErr = fmt.Errorf("no files found")
		return
	}
	if in.Debug {
		fmt.Fprintf(os.Stderr, "found files:\n")
		for _, f := range in.files {
			fmt.Fprintf(os.Stderr, "  %v\n", f)
		}
	}

	jb := func(fd io.Reader) (map[string]adlast.Module, map[string]adlast.Decl, error) {
		combinedAst = make(map[string]adlast.Module)
		declMap := make(map[string]adlast.Decl)
		dec := goadl.CreateJsonDecodeBinding(adlast.Texpr_StringMap[adlast.Module](goadl.Texpr_Module()), goadl.RESOLVER)
		err := dec.Decode(fd, &combinedAst)
		if err != nil {
			panic(fmt.Errorf("%w", err))
			// return nil, nil, err
		}
		for k, v := range combinedAst {
			for dk, dv := range v.Decls {
				declMap[k+"::"+dk] = dv
			}
		}
		return combinedAst, declMap, nil
	}

	modules = []namedModule{}
	var declMap map[string]adlast.Decl
	combinedAst, declMap, setupErr = loadAdl(in, &modules, jb)
	_ = declMap
	if setupErr != nil {
		os.Exit(1)
	}

	modulePath, midPath, setupErr = in.modpath()
	if setupErr != nil {
		return
	}
	return
}

func containsTypeToken(str adlast.Struct) bool {
	for _, fld := range str.Fields {
		if pr, ok := fld.TypeExpr.TypeRef.Cast_primitive(); ok && pr == "TypeToken" {
			return true
		}
	}
	return false
}

func getTypeToken(str adlast.Struct, tbind []goadl.TypeBinding) ([]adlast.Field, bool) {
	concrete := true
	ttFld := []adlast.Field{}
	for _, fld := range str.Fields {
		if pr, ok := fld.TypeExpr.TypeRef.Cast_primitive(); ok && pr == "TypeToken" {
			monoTe, c0 := goadl.SubstituteTypeBindings(tbind, fld.TypeExpr.Parameters[0])
			concrete = concrete && c0
			fld0 := adlast.Make_Field(
				fld.Name,
				fld.SerializedName,
				monoTe,
				types.Make_Maybe_nothing[any](),
				fld.Annotations,
			)
			ttFld = append(ttFld, fld0)
		}
	}
	return ttFld, concrete
}

func (in *goadlcCmd) generate(
	combinedAst map[string]adlast.Module,
	// importMap map[string]importSpec,
	modulePath string,
	midPath string,
	modules []namedModule,
	setupErr error,
) error {
	if setupErr != nil {
		return setupErr
	}
	resolver := func(sn adlast.ScopedName) (*adlast.Decl, bool) {
		if mod, ok := combinedAst[sn.ModuleName]; ok {
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
		for k := range combinedAst {
			fmt.Printf("-- %v\n", k)
		}
		panic(fmt.Errorf("%v", sn.ModuleName))
	}

	for _, m := range modules {
		modCodeGenDir := strings.Split(m.name, ".")
		// modCodeGenPkg := pkgFromImport(strings.ReplaceAll(m.name, ".", "/"))
		modCodeGenPkg := modCodeGenDir[len(modCodeGenDir)-1]
		path := in.Outputdir + "/" + strings.Join(modCodeGenDir, "/")
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
			baseGen: in.newBaseGen(resolver, modulePath, midPath, m.name),
			rr:      templateRenderer{t: templates},
		}
		astBody := &generator{
			baseGen: in.newBaseGen(resolver, modulePath, midPath, m.name),
			rr:      templateRenderer{t: templates},
		}
		declsNames := []string{}
		for k := range m.module.Decls {
			declsNames = append(declsNames, k)
		}
		goslices.Sort(declsNames)
		for _, k := range declsNames {
			decl := m.module.Decls[k]
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
		err := in.writeFile(m.name, modCodeGenPkg, declBody, filepath.Join(path, modCodeGenDir[len(modCodeGenDir)-1]+".go"), in.NoGoFmt, false)
		if err != nil {
			return err
		}
		if !in.ExcludeAst {
			override := false
			fname := modCodeGenDir[len(modCodeGenDir)-1] + "_ast.go"
			if _, ok := in.specialTexpr()[m.name]; ok && in.StdLibGen {
				err := in.writeFile(m.name, "goadl", astBody, filepath.Join(in.Outputdir, fname), in.NoGoFmt, true)
				if err != nil {
					return err
				}
				override = true
			}
			// for _, astinfo := range in.GenAstInfo {
			// 	if astinfo.ModuleName == m.name {
			// 		err := in.writeFile(m.name, astinfo.Pkg, astBody, filepath.Join(in.Outputdir, astinfo.RelOutputFile), in.NoGoFmt, true)
			// 		if err != nil {
			// 			return err
			// 		}
			// 		override = true
			// 		break
			// 	}
			// }
			if !override {
				err := in.writeFile(m.name, modCodeGenPkg, astBody, filepath.Join(path, fname), in.NoGoFmt, true)
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
	// typetoken_flds func(sn adlast.ScopedName, tbind []goadl.TypeBinding) []adlast.Field,
	// importMap map[string]importSpec,
	// bundleMap BundleMaps,
	modulePath, midPath string,
	moduleName string,
) *baseGen {
	imports := newImports(
		in.reservedImports(),
		in.BundleMap,
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
	adlast.Handle_DeclType[any](
		decl.Type_,
		func(s adlast.Struct) any {

			typeTokenFields := []typeTokenField{}
			for _, fld := range s.Fields {
				if ref, ok := fld.TypeExpr.TypeRef.Cast_reference(); ok {
					decl1, ok := in.resolver(ref)
					if !ok {
						panic(fmt.Errorf("missing decl %v", ref))
					}
					if str, ok := decl1.Type_.Cast_struct_(); ok {
						tbind := goadl.CreateDecBoundTypeParams(goadl.TypeParamsFromDecl(*decl1), fld.TypeExpr.Parameters)
						refFields, concrete := getTypeToken(str, tbind)
						if !concrete {
							in.rr.buf.Write([]byte(fmt.Sprintf("// %s::%s Type Param is passed through to a TypeToken, not generating TypeTokenTexprs method\n", decl.Name, fld.Name)))
							typeTokenFields = []typeTokenField{}
							break
						}
						if len(refFields) != 0 {
							typeTokenFields = append(typeTokenFields, typeTokenField{
								Field:     fld,
								RefFields: refFields,
							})
						}
					}
				}
			}
			in.rr.Render(structParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParam{s.TypeParams, []string{}, false, base.cli.StdLibGen},
				Fields: slices.Map(s.Fields, func(f adlast.Field) fieldParams {
					return makeFieldParam(f, decl.Name, in)
				}),
				TypeTokenFields:   typeTokenFields,
				ContainsTypeToken: containsTypeToken(s),
				// ContainsTypeToken: containsTypeToken(s),
				// RefToTypeToken:    refs_typetoken,
			})
			return nil
		},
		func(u adlast.Union) any {
			in.rr.Render(unionParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParam{u.TypeParams, []string{}, false, base.cli.StdLibGen},
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
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParam{td.TypeParams, []string{}, false, base.cli.StdLibGen},
				TypeExpr:   td.TypeExpr,
			})
			return nil
		},
		func(nt adlast.NewType) any {
			in.rr.Render(newTypeParams{
				G:          in,
				Name:       decl.Name,
				TypeParams: typeParam{nt.TypeParams, []string{}, false, base.cli.StdLibGen},
				TypeExpr:   nt.TypeExpr,
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

	tp.stdlib = base.cli.StdLibGen
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
			// fmt.Printf("???????1 %v\n", reflect.TypeOf(just))
			// val := reflect.ValueOf(just).Interface()
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
