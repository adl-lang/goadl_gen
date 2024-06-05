package gen_go

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
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
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in GoType %v\n%v", r, string(debug.Stack()))
			panic(r)
		}
	}()
	_type := adlast.Handle_TypeRef(
		typeExpr.TypeRef,
		func(primitive string) goTypeExpr {
			_type := in.PrimitiveMap(primitive, typeExpr.Parameters)
			return _type
		},
		func(tp string) goTypeExpr {
			return goTypeExpr{"", tp, typeParam{
				ps:               []string{string(tp)},
				type_constraints: []string{},
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
				panic(fmt.Errorf("error getting go_custom_type annotation for %v. err : %w", decl.Name, err))
			}
			if gct != nil {
				pkg := gct.Gotype.Import_path[strings.LastIndex(gct.Gotype.Import_path, "/")+1:]
				spec := importSpec{
					Path:    gct.Gotype.Import_path,
					Name:    gct.Gotype.Pkg,
					Aliased: gct.Gotype.Pkg != pkg,
				}
				in.imports.addSpec(spec)
				return goTypeExpr{
					Pkg:  gct.Gotype.Pkg,
					Type: gct.Gotype.Name,
					TypeParams: typeParam{
						ps: slices.Map(typeExpr.Parameters, func(a adlast.TypeExpr) string {
							pt := in.GoType(a)
							return pt.String()
						}),
						type_constraints: []string{},
					},
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
) goTypeExpr {
	r, has := primitiveMap[p]
	if has {
		return goTypeExpr{"", r, typeParam{}, false}
	}
	elem := in.GoType(params[0])
	switch p {
	case "TypeToken":
		pkg, err := in.GoImport("adlast")
		if err != nil {
			panic(err)
		}
		tp := typeParam{
			ps: []string{elem.String()},
		}
		return goTypeExpr{"", pkg + "ATypeExpr", tp, false}
	case "Vector":
		return goTypeExpr{"", "[]" + elem.sansTypeParam(), elem.TypeParams, elem.IsTypeParam}
	case "StringMap":
		return goTypeExpr{"", "map[string]" + elem.sansTypeParam(), elem.TypeParams, elem.IsTypeParam}
	case "Nullable":
		return goTypeExpr{"", "*" + elem.sansTypeParam(), elem.TypeParams, elem.IsTypeParam}
	}
	panic(fmt.Errorf("no such primitive '%s'", p))
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
		// "TypeToken":  "struct{}",
		// "`Vector<T>`":    0,
		// "`StringMap<T>`": 0,
		// "`Nullable<T>`":  0,
	}
)
