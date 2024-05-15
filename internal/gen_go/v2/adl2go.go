package gen_go_v2

import (
	"fmt"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/adl-lang/goadlc/internal/gen_go/fn/slices"
)

type goTypeExpr struct {
	Pkg        string
	Type       string
	TypeParams typeParam
	TypeParam  bool
}

func (g goTypeExpr) String() string {
	if g.TypeParam {
		return g.Type
	}
	if g.Pkg != "" {
		return g.Pkg + "." + g.Type + g.TypeParams.RSide()
	}
	return g.Type + g.TypeParams.RSide()
}

func (in *baseGen) GoType(
	typeExpr goadl.TypeExpr,
) goTypeExpr {
	_type := goadl.Handle_TypeRef(
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
		func(ref goadl.ScopedName) goTypeExpr {
			packageName := ""
			if in.moduleName != ref.ModuleName {
				if in.midPath != "" {
					pkg := in.modulePath + "/" + in.midPath + "/" + strings.ReplaceAll(ref.ModuleName, ".", "/")
					packageName = in.imports.add(pkg)
				} else {
					pkg := in.modulePath + "/" + strings.ReplaceAll(ref.ModuleName, ".", "/")
					packageName = in.imports.add(pkg)
				}
			}
			goTypeParams := slices.Map(typeExpr.Parameters, func(a goadl.TypeExpr) goTypeExpr {
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
				TypeParam: false,
			}
		},
	)
	return _type
}

func (in *baseGen) PrimitiveMap(
	p string,
	params []goadl.TypeExpr,
) (_type goTypeExpr) {
	r, has := primitiveMap[p]
	if has {
		return goTypeExpr{"", r, typeParam{}, false}
	}
	elem := in.GoType(params[0])
	switch p {
	case "Vector":
		return goTypeExpr{"", "[]" + elem.String(), elem.TypeParams, elem.TypeParam} // "[]" + elem.String(), elem.TypeParams
	case "StringMap":
		return goTypeExpr{"", "map[string]" + elem.String(), elem.TypeParams, elem.TypeParam} // "[]" + elem.String(), elem.TypeParams
		// return "map[string]" + elem.String(), elem.TypeParams
	case "Nullable":
		return goTypeExpr{"", "*" + elem.String(), elem.TypeParams, elem.TypeParam} // "[]" + elem.String(), elem.TypeParams
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
		"Void":       "goadl.Void",
		"Json":       "interface{}",
		// "`Vector<T>`":    0,
		// "`StringMap<T>`": 0,
		// "`Nullable<T>`":  0,
	}
)
