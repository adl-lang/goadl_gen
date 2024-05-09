package gen_go_v2

import (
	"fmt"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/adl-lang/goadlc/internal/gen_go/fn/slices"
)

type goTypeExpr struct {
	Pkg       string
	Type      string
	Params    string
	TypeParam bool
}

func (g goTypeExpr) String() string {
	if g.Pkg != "" {
		return g.Pkg + "." + g.Type + g.Params
	}
	return g.Type + g.Params
}

func (in *generator) GoType(
	typeExpr goadl.TypeExpr,
	// moduleName string,
	// declMap map[string]goadl.Decl,
	// imports *[]string,
) goTypeExpr {
	_type, _ := goadl.HandleTypeRef(
		typeExpr.TypeRef.Branch,
		func(primitive goadl.TypeRefBranch_Primitive) (goTypeExpr, error) {
			_type := in.PrimitiveMap(string(primitive), typeExpr.Parameters)
			return goTypeExpr{"", _type, "", false}, nil
		},
		func(typeParam goadl.TypeRefBranch_TypeParam) (goTypeExpr, error) {
			return goTypeExpr{"", string(typeParam), "", true}, nil
		},
		func(ref goadl.TypeRefBranch_Reference) (goTypeExpr, error) {
			packageName := ""
			if in.moduleName != ref.ModuleName {
				pkg := in.modulePath + "/" + strings.ReplaceAll(ref.ModuleName, ".", "/")
				packageName = in.imports.add(pkg)
				// (*imports) = append((*imports), ref.ModuleName)
			}
			goTypeParams := slices.Map(typeExpr.Parameters, func(a goadl.TypeExpr) goTypeExpr {
				return in.GoType(a)
			})
			generic := ""
			if len(goTypeParams) > 0 {
				generic = "[" + strings.Join(slices.Map(goTypeParams, func(a goTypeExpr) string { return a.String() }), ",") + "]"
			}
			return goTypeExpr{
				Pkg:       packageName,
				Type:      ref.Name,
				Params:    generic,
				TypeParam: false,
			}, nil
		},
	)
	return _type
}

func (in *generator) PrimitiveMap(
	p string,
	// moduleName string,
	params []goadl.TypeExpr,
	// declMap map[string]goadl.Decl,
	// imports *[]string,
) (_type string) {
	r, has := primitiveMap[p]
	if has {
		return r
	}
	elem := in.GoType(params[0])
	// rightTypeParams := ""
	// if len(params[0].Parameters) > 0 {
	// 	x := slices.Map(params[0].Parameters, func(te goadl.TypeExpr) string {
	// 		gt := in.GoType(te) //, moduleName, declMap, imports)
	// 		if gt.TypeParam {
	// 			return gt.Type
	// 		}
	// 		return gt.String()
	// 	})
	// 	rightTypeParams = "[" + strings.Join(x, ",") + "]"
	// }

	if p == "Vector" {
		return "[]" + elem.String()
		// prefix = "[]"
		// _type, _ = goadl.HandleTypeRef[string](
		// 	params[0].TypeRef.Branch,
		// 	func(primitive goadl.TypeRefBranch_Primitive) (string, error) {
		// 		// TODO deal with Vector<Vector/StringMap/Nullable<>>
		// 		return string(primitive) + "/*--*/" + rightTypeParams, nil
		// 	},
		// 	func(typeParam goadl.TypeRefBranch_TypeParam) (string, error) {
		// 		return string(typeParam), nil
		// 	},
		// 	func(ref goadl.TypeRefBranch_Reference) (string, error) {
		// 		pkg := ""
		// 		if in.moduleName != ref.ModuleName {
		// 			pkg = strings.ReplaceAll(ref.ModuleName, ".", "_") + "."
		// 		}
		// 		return pkg + ref.Name + rightTypeParams, nil
		// 	},
		// )
		// return
	}
	if p == "StringMap" {
		return "map[string]" + elem.String()
		// prefix = "map[string]"
		// _type, _ = goadl.HandleTypeRef(
		// 	params[0].TypeRef.Branch,
		// 	func(primitive goadl.TypeRefBranch_Primitive) (string, error) {
		// 		return string(primitive) + rightTypeParams, nil
		// 	},
		// 	func(typeParam goadl.TypeRefBranch_TypeParam) (string, error) {
		// 		return "/* StringMap<typeParam> not implemented */", nil
		// 	},
		// 	func(ref goadl.TypeRefBranch_Reference) (string, error) {
		// 		pkg := ""
		// 		if in.moduleName != ref.ModuleName {
		// 			pkg = strings.ReplaceAll(ref.ModuleName, ".", "_") + "."
		// 		}
		// 		return pkg + ref.Name + rightTypeParams, nil
		// 	},
		// )
		// return
	}
	if p == "Nullable" {
		return "*" + elem.String()
		// prefix = "*"
		// _type, _ = goadl.HandleTypeRef(
		// 	params[0].TypeRef.Branch,
		// 	func(primitive goadl.TypeRefBranch_Primitive) (string, error) {
		// 		return string(primitive) + rightTypeParams, nil
		// 	},
		// 	func(typeParam goadl.TypeRefBranch_TypeParam) (string, error) {
		// 		return "/* Nullable<typeParam> not implemented */", nil
		// 	},
		// 	func(ref goadl.TypeRefBranch_Reference) (string, error) {
		// 		pkg := ""
		// 		if in.moduleName != ref.ModuleName {
		// 			pkg = strings.ReplaceAll(ref.ModuleName, ".", "_") + "."
		// 		}
		// 		return pkg + ref.Name + rightTypeParams, nil
		// 	},
		// )
		// return
	}
	fmt.Printf("!!'%s'\n", p)
	return ""
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

func GoEscape(n string) string {
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
