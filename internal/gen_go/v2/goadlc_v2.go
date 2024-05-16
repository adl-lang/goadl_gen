package gen_go_v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
	SkipGenTexpr      bool     `opts:"short=t" help:"Don't generate type expr functions"`
	SkipGenScopedDecl bool     `opts:"short=s" help:"Don't generate scoped decl and init registration functions"`
	Files             []string `opts:"mode=arg"`
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
				cli:        in,
				declMap:    declMap,
				modulePath: modulePath,
				midPath:    midPath,
				moduleName: m.name,
				name:       name,
				imports:    newImports(reservedImports),
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

var reservedImports []importSpec = []importSpec{
	{Path: "encoding/json"},
	{Path: "reflect"},
	{Path: "strings"},
	{Path: "fmt"},
	{Path: "github.com/adl-lang/goadl_rt/v2", Aliased: true, Name: "goadl"},
}

func (bg *baseGen) Import(pkg string) (string, error) {
	if spec, ok := bg.imports.byName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	} else {
		bg.imports.add(spec.Path)
		return spec.Name, nil
	}
}

type baseGen struct {
	cli        *goadlcV2Cmd
	declMap    moduleMap[goadl.Decl]
	modulePath string
	midPath    string
	moduleName string
	name       string
	imports    imports
}

type generator struct {
	*baseGen
	rr templateRenderer
}

// type typeMapField goadl.Field

// func (f typeMapField) TpArgs() string {
// 	if _, ok := f.TypeExpr.TypeRef.Branch.(goadl.TypeRefBranch_TypeParam); ok {
// 		return "[any]"
// 	}
// 	if len(f.TypeExpr.Parameters) == 0 {
// 		return ""
// 	}
// 	return "[any" + strings.Repeat(",any", len(f.TypeExpr.Parameters)-1) + "]"
// }

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
	}
	goadl.HandleE_DeclType[any](
		decl.Type.Branch,
		body.generateStruct,
		body.generateUnion,
		body.generateTypeAlias,
		body.generateNewType,
	)

	if !base.cli.SkipGenTexpr {
		body.rr.Render(texprmonoParams{
			G:          body,
			ModuleName: base.moduleName,
			// Name:       goEscape(base.name),
			// TypeParams: getTypeParams(decl),
			Name:       base.name,
			TypeParams: typeParamsFromDecl(decl),
			Decl:       decl,
		})
	}
	if !base.cli.SkipGenScopedDecl {
		body.rr.Render(scopedDeclParams{
			G:          body,
			ModuleName: base.moduleName,
			Name:       base.name,
			Decl:       decl,
			TypeParams: typeParamsFromDecl(decl),
			Fields: goadl.Handle_DeclType[[]fieldParams](
				decl.Type.Branch,
				func(struct_ goadl.Struct) []fieldParams {
					return []fieldParams{}
					// struct_.Fields[0].SerializedName
					// return struct_.Fields
				},
				func(u goadl.Union) []fieldParams {
					return slices.Map[goadl.Field, fieldParams](u.Fields, func(f goadl.Field) fieldParams {
						return makeFieldParam(f)
						// return fieldParams{
						// 	Field:      f,
						// 	HasDefault: f.Default.Just != nil,
						// 	Just:       *f.Default.Just,
						// 	// Name:           goEscape(f.Name),
						// 	// SerializedName: f.SerializedName,
						// 	// TypeParams:     new_typeParams(u.TypeParams),
						// 	// Type:           base.GoType(f.TypeExpr),
						// }
					})
				},
				func(type_ goadl.TypeDef) []fieldParams {
					return []fieldParams{}
				},
				func(newtype_ goadl.NewType) []fieldParams {
					return []fieldParams{}
				},
				nil,
			),
		})
	}

	header.rr.Render(headerParams{
		Pkg: modCodeGen.Directory[len(modCodeGen.Directory)-1],
	})
	imports := []importSpec{}
	for _, spec := range body.imports.specs {
		if body.imports.used[spec.Path] {
			imports = append(imports, spec)
		}
	}
	header.rr.Render(importsParams{
		// Rt: "github.com/adl-lang/goadl_rt/v2",
		// RtAs: "goadl",
		Imports: imports,
	})
	header.rr.buf.Write(body.rr.Bytes())
	return header.rr.Bytes()
}

func (*generator) JsonEncode(val any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := enc.Encode(val)
	if err != nil {
		panic(err)
	}
	return string(bytes.Trim(buf.Bytes(), "\n"))
	// return  buf.String()
}

func (in *generator) generateStruct(s goadl.DeclTypeBranch_Struct_) (interface{}, error) {
	in.rr.Render(structParams{
		G: in,
		// Name:       goEscape(in.name),
		Name:       in.name,
		TypeParams: typeParam{s.TypeParams, false},
		Fields: slices.Map(s.Fields, func(f goadl.Field) fieldParams {
			return makeFieldParam(f)
			// fp := fieldParams{
			// 	// Name:           goEscape(f.Name),
			// 	// SerializedName: f.SerializedName,
			// 	// TypeParams:     typeParam{s.TypeParams, false},
			// 	// Type:           in.GoType(f.TypeExpr),
			// 	Field:      f,
			// 	HasDefault: f.Default.Just != nil,
			// }
			// return fp
		}),
	})
	return nil, nil
}

func makeFieldParam(f goadl.Field) fieldParams {
	fp := fieldParams{
		Field:      f,
		HasDefault: f.Default.Just != nil,
	}
	if f.Default.Just != nil {
		fp.Just = *f.Default.Just
	}
	return fp
}

func (in *generator) ToTitle(s string) string {
	return strings.ToTitle(s)
}

func (in *generator) generateUnion(u goadl.DeclTypeBranch_Union_) (interface{}, error) {
	in.rr.Render(unionParams{
		G: in,
		// Name:       goEscape(in.name),
		Name:       in.name,
		TypeParams: new_typeParams(u.TypeParams),
		Branches: slices.Map[goadl.Field, fieldParams](u.Fields, func(f goadl.Field) fieldParams {
			return makeFieldParam(f)
			// return fieldParams{
			// 	Field:      f,
			// 	HasDefault: f.Default.Just != nil,
			// 	Just:       *f.Default.Just,
			// 	// Name:           goEscape(f.Name),
			// 	// SerializedName: f.SerializedName,
			// 	// TypeParams:     new_typeParams(u.TypeParams),
			// 	// Type:           in.GoType(f.TypeExpr),
			// }
		}),
	})
	return nil, nil
}

func (in *generator) generateTypeAlias(td goadl.DeclTypeBranch_Type_) (interface{}, error) {
	in.rr.Render(typeAliasParams{
		G: in,
		// Name:       goEscape(in.name),
		Name:       in.name,
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
		G: in,
		// Name:       goEscape(in.name),
		Name:       in.name,
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

func (bg *baseGen) GoValueDebug(d_tp typeParam, t_gt goTypeExpr, te goadl.TypeExpr, val any) string {
	return strings.Join([]string{toJ(d_tp), toJ(t_gt), toJ(te), toJ(val)}, "\n")
}

func toJ(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func (bg *baseGen) GoValue(decl_tp typeParam, te goadl.TypeExpr, val any) string {
	return goadl.Handle_TypeRef[string](
		te.TypeRef.Branch,
		func(primitive string) string {
			return bg.GoValuePrimitive(decl_tp, te, primitive, val)
		},
		func(typeParam string) string {
			// return typeParam
			panic("???GoValue:typeParam " + typeParam)
		},
		func(ref goadl.ScopedName) string {
			return bg.GoValueScopedName(decl_tp, te, ref, val)
		},
	)
}

func (bg *baseGen) GoValuePrimitive(decl_tp typeParam, te goadl.TypeExpr, primitive string, val any) string {
	switch primitive {
	case "Int8", "Int16", "Int32", "Int64",
		"Word8", "Word16", "Word32", "Word64",
		"Bool", "Float", "Double":
		return fmt.Sprintf("%v", val)
	case "String":
		return fmt.Sprintf(`"%s"`, val)
	// case "ByteVector":
	case "Void":
		return "nil"
	case "Json":
		panic("todo")
	case "Vector":
		rv := reflect.ValueOf(val)
		vs := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			v := rv.Index(i)
			vs[i] = bg.GoValue(decl_tp, te.Parameters[0], v.Interface())
		}
		vss := strings.Join(vs, ",\n")
		return fmt.Sprintf("[]%s{\n%s,\n}", bg.GoType(te.Parameters[0]), vss)
	case "StringMap":
		m := val.(map[string]any)
		vs := make(kvBy, 0, len(m))
		for k, v := range m {
			vs = append(vs, kv{k, bg.GoValue(decl_tp, te.Parameters[0], v)})
		}
		sort.Sort(vs)
		return fmt.Sprintf("map[string]%s{\n%s,\n}", bg.GoType(te.Parameters[0]), vs)
	case "Nullable":
		if val == nil {
			return "nil"
		}
		return bg.GoValue(decl_tp, te.Parameters[0], val)
	}
	panic("??? GoValuePrimitive")
}

func (bg *baseGen) GoValueScopedName(
	decl_tp typeParam,
	te goadl.TypeExpr,
	ref goadl.ScopedName,
	val any,
) string {
	gt := bg.GoType(te)
	decl, ok := bg.declMap[ref.ModuleName+"::"+ref.Name]
	if !ok {
		panic("decl not in map :" + ref.ModuleName + "::" + ref.Name)
	}
	vs := goadl.Handle_DeclType[[]string](
		decl.Type.Branch,
		func(struct_ goadl.Struct) []string {
			m := val.(map[string]any)
			return slices.FlatMap[goadl.Field, string](struct_.Fields, func(f goadl.Field) []string {
				ret := []string{}
				if v, ok := m[f.SerializedName]; ok {
					ret = append(ret, fmt.Sprintf(`%s: %s`, public(f.Name), bg.GoValue(decl_tp, f.TypeExpr, v)))
				}
				if _, ok := m[f.SerializedName]; !ok && f.Default.Just != nil {
					ret = append(ret, fmt.Sprintf(`%s: %s`, public(f.Name), bg.GoValue(decl_tp, f.TypeExpr, *f.Default.Just)))
				}
				return ret
			})
		},
		func(union_ goadl.Union) []string {
			var (
				k string
				v any
			)
			switch t := val.(type) {
			case string:
				k = t
				v = nil
			case map[string]any:
				if len(t) != 1 {
					panic(fmt.Sprintf("expect an object with one and only element received %v", len(t)))
				}
				for k0, v0 := range t {
					k = k0
					v = v0
				}
			default:
				panic(fmt.Errorf("union: expect an object received %v '%v'", reflect.TypeOf(val), val))
			}
			var fld *goadl.Field
			for _, f0 := range union_.Fields {
				if f0.SerializedName == k {
					fld = &f0
					break
				}
			}
			if fld == nil {
				panic(fmt.Errorf("unexpected branch - no type registered '%v'", k))
			}
			fte := bg.GoType(fld.TypeExpr)
			// gte := bg.GoType(te)
			// tp := typeParam{
			// 	ps: slices.Map[string, string](fte.TypeParams.ps, func(a string) string {
			// 		fld.TypeExpr.Parameters
			// 	}),
			// }
			a0, _ := json.MarshalIndent(decl_tp, "", "  ")
			a1, _ := json.MarshalIndent(fld, "", "  ")
			a2, _ := json.MarshalIndent(fte, "", "  ")
			_ = v
			return []string{
				fmt.Sprintf("%+v\n%+v\n%+v\n%+v",
					string(a0), string(a1), string(a2), toJ(gt)),
			}

			// return []string{fmt.Sprintf(`%sBranch_%s%s{%v}`,
			// 	decl.Name,
			// 	fld.Name,
			// 	fte.TypeParams.RSide(),
			// 	// v,
			// 	bg.GoValue(decl_tp, fld.TypeExpr, v),
			// )}
		},
		func(type_ goadl.TypeDef) []string {
			return []string{"todo - type"}
		},
		func(newtype_ goadl.NewType) []string {
			return []string{"todo - newtype"}
		},
		nil,
	)

	return fmt.Sprintf("%s{\n%s,\n}", gt.String(), strings.Join(vs, ",\n"))
	// return fmt.Sprintf("%s{%v,\n}", gt.String(), val)

	// panic("todo")
}

type kv struct {
	k string
	v string
}

type kvBy []kv

func (kv kv) String() string {
	return fmt.Sprintf(`"%s" : %s`, kv.k, kv.v)
}
func (elems kvBy) String() string {
	var b strings.Builder
	// b.Grow(n)
	b.WriteString(elems[0].String())
	for _, s := range elems[1:] {
		b.WriteString(",\n")
		b.WriteString(s.String())
	}
	return b.String()
}

func (a kvBy) Len() int           { return len(a) }
func (a kvBy) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a kvBy) Less(i, j int) bool { return a[i].k < a[j].v }
