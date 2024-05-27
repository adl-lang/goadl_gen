package gen_go_v2

import (
	"fmt"
	"os"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/adlc/config/go_"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/gen_go/fn/slices"
)

type goTypeExpr struct {
	Pkg         string
	Type        string
	TypeParams  typeParam
	IsTypeParam bool
}

func (g goTypeExpr) String() string {
	if g.IsTypeParam {
		if g.Pkg != "" {
			return g.Pkg + "." + g.Type
		}
		return g.Type
	}
	if g.Pkg != "" {
		return g.Pkg + "." + g.Type + g.TypeParams.RSide()
	}
	return g.Type + g.TypeParams.RSide()
}

func (g goTypeExpr) Complete() string {
	if g.Pkg != "" {
		return g.Pkg + "." + g.Type + g.TypeParams.RSide()
	}
	return g.Type + g.TypeParams.RSide()
}

func (g goTypeExpr) sansTypeParam() string {
	if g.Pkg != "" {
		return g.Pkg + "." + g.Type
	}
	return g.Type
}

func (in *baseGen) GoType(
	typeExpr adlast.TypeExpr,
) goTypeExpr {
	_type := adlast.Handle_TypeRef(
		typeExpr.TypeRef.Branch,
		func(primitive string) goTypeExpr {
			_type := in.PrimitiveMap(primitive, typeExpr.Parameters)
			return _type
		},
		func(tp string) goTypeExpr {
			return goTypeExpr{"", tp, typeParam{
				ps: []string{string(tp)},
				// isTypeParam: true,
			}, true}
		},
		func(ref adlast.ScopedName) goTypeExpr {
			decl, ok := in.resolver(ref)
			if !ok {
				panic(fmt.Errorf("cannot find decl '%v", ref))
			}
			jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_GoCustomType(), goadl.RESOLVER)
			gct, err := goadl.GetAnnotation(decl.Annotations, goCustomTypeSN, jb)
			if err != nil {
				panic(err)
			}
			if gct != nil {
				pkg := gct.Helpers.Import_path[strings.LastIndex(gct.Helpers.Import_path, "/")+1:]
				spec := importSpec{
					Path:    gct.Helpers.Import_path,
					Name:    gct.Helpers.Pkg,
					Aliased: gct.Helpers.Pkg != pkg,
				}
				in.imports.addSpec(spec)
				if in.cli.Debug {
					fmt.Fprintf(os.Stderr, "GoType GoCustomType %v %v %v\n", gct, spec, pkg)
				}
				return goTypeExpr{
					Pkg:  gct.Gotype.Pkg,
					Type: gct.Gotype.Name,
					TypeParams: typeParam{
						ps: slices.Map[go_.TypeParam, string](gct.Gotype.Type_params, func(a go_.TypeParam) string {
							return a.Name
						}),
					},
					// TypeParams: typeParam{
					// 	ps: slices.Map(goTypeParams, func(a goTypeExpr) string { return a.String() }),
					// },
					IsTypeParam: false,
				}
			}

			packageName := ""
			if in.moduleName != ref.ModuleName {
				packageName = in.imports.addModule(ref.ModuleName, in.modulePath, in.midPath)
			}
			goTypeParams := slices.Map(typeExpr.Parameters, func(a adlast.TypeExpr) goTypeExpr {
				return in.GoType(a)
			})
			// generic := ""
			// if len(goTypeParams) > 0 {
			// 	generic = "[" + strings.Join(slices.Map(goTypeParams, func(a goTypeExpr) string { return a.String() }), ",") + "]"
			// }
			return goTypeExpr{
				Pkg:  packageName,
				Type: ref.Name,
				TypeParams: typeParam{
					ps: slices.Map(goTypeParams, func(a goTypeExpr) string { return a.String() }),
				},
				IsTypeParam: false,
			}
		},
		nil,
	)
	return _type
}

func (in *baseGen) PrimitiveMap(
	p string,
	params []adlast.TypeExpr,
) (_type goTypeExpr) {
	// if p == "Void" {
	// 	pkg, err := in.Import("goadl")
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	return goTypeExpr{"", pkg + "Void", typeParam{}, false}
	// }
	r, has := primitiveMap[p]
	if has {
		return goTypeExpr{"", r, typeParam{}, false}
	}
	elem := in.GoType(params[0])
	switch p {
	case "Vector":
		return goTypeExpr{"", "[]" + elem.sansTypeParam(), elem.TypeParams, elem.IsTypeParam} // "[]" + elem.String(), elem.TypeParams
	case "StringMap":
		return goTypeExpr{"", "map[string]" + elem.sansTypeParam(), elem.TypeParams, elem.IsTypeParam} // "[]" + elem.String(), elem.TypeParams
		// return "map[string]" + elem.String(), elem.TypeParams
	case "Nullable":
		return goTypeExpr{"", "*" + elem.sansTypeParam(), elem.TypeParams, elem.IsTypeParam} // "[]" + elem.String(), elem.TypeParams
		// return "*" + elem.String(), elem.TypeParams
	}
	panic(fmt.Errorf("no such primitive '%s'", p))
	// return "", typeParam{}
}

var goKeywords = map[string]string{
	"break":       "break_",
	"default":     "default_",
	"func":        "func_",
	"interface":   "interface_",
	"select":      "select_",
	"case":        "case_",
	"defer":       "defer_",
	"go":          "go_",
	"map":         "map_",
	"struct":      "struct_",
	"chan":        "chan_",
	"else":        "else_",
	"goto":        "goto_",
	"package":     "package_",
	"switch":      "switch_",
	"const":       "const_",
	"fallthrough": "fallthrough_",
	"if":          "if_",
	"range":       "range_",
	"type":        "type_",
	"continue":    "continue_",
	"for":         "for_",
	"import":      "import_",
	"return":      "return_",
	"var":         "var_",
}

func (*baseGen) GoEscape(n string) string {
	if g, h := goKeywords[n]; h {
		return g
	}
	return n
}

var (
	primitiveMap = map[string]string{
		"Int8":       "int8",
		"Int16":      "int16",
		"Int32":      "int32",
		"Int64":      "int64",
		"Word8":      "uint8",
		"Word16":     "uint16",
		"Word32":     "uint32",
		"Word64":     "uint64",
		"Bool":       "bool",
		"Float":      "float64",
		"Double":     "float64",
		"String":     "string",
		"ByteVector": "[]byte",
		"Void":       "struct{}",
		"Json":       "any",
		// "`Vector<T>`":    0,
		// "`StringMap<T>`": 0,
		// "`Nullable<T>`":  0,
	}
)
