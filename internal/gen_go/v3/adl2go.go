package gen_go

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/gen_go/fn/slices"
	"github.com/samber/lo"
)

type goTypeExpr struct {
	Pkg             string
	Type            string
	TypeParams      typeParam
	UnionTypeParams typeParam
	IsTypeParam     bool
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
	anns adlast.Annotations,
) goTypeExpr {
	unionTypeParams := typeParam{}
	gotype := in.goType(typeExpr, &unionTypeParams, anns)
	unionTypeParams.type_constraints = get_type_constraints(anns)
	gotype.UnionTypeParams = unionTypeParams
	return gotype
}

func (in *baseGen) goType(
	typeExpr adlast.TypeExpr,
	unionTypeParams *typeParam,
	anns adlast.Annotations,
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
			_type := in.PrimitiveMap(primitive, typeExpr.Parameters, unionTypeParams, anns)
			return _type
		},
		func(tp string) goTypeExpr {
			if !lo.ContainsBy(unionTypeParams.params, func(it param) bool {
				return it.name == tp
			}) {
				unionTypeParams.params = append(unionTypeParams.params, param{name: tp})
			}
			return goTypeExpr{
				"",
				tp,
				typeParam{
					params: []param{{name: tp, concrete: false}},
					// type_constraints: []string{},
					type_constraints: get_type_constraints(anns),
				},
				typeParam{},
				true,
			}
		},
		func(ref adlast.ScopedName) goTypeExpr {
			decl, ok := in.resolver(ref)
			if !ok {
				panic(fmt.Errorf("cannot find decl '%v", ref))
			}
			if goadl.HasAnnotation(decl.Annotations, goCustomTypeSN) {
				return in.gotype_ref_customtype(decl, typeExpr, unionTypeParams, anns)
			}
			// go can't have typeParam on lhs in type alias, so replace with concrete type
			if typ, ok := decl.Type_.Cast_type_(); ok {
				if len(typ.TypeParams) != 0 {
					tbind := goadl.CreateDecBoundTypeParams(typ.TypeParams, typeExpr.Parameters)
					monoTe, _ := goadl.SubstituteTypeBindings(tbind, typ.TypeExpr)
					return in.goType(monoTe, unionTypeParams, anns)
				}
			}

			packageName := ""
			if in.moduleName != ref.ModuleName {
				packageName = in.imports.addModule(ref.ModuleName, in.modulePath, in.midPath)
			}
			goTypeParams := slices.Map(typeExpr.Parameters, func(a adlast.TypeExpr) goTypeExpr {
				return in.goType(a, unionTypeParams, anns)
			})

			return goTypeExpr{
				Pkg:  packageName,
				Type: ref.Name,
				TypeParams: typeParam{
					params:           slices.Map(goTypeParams, func(a goTypeExpr) param { return param{name: a.String(), concrete: !a.IsTypeParam} }),
					type_constraints: get_type_constraints(anns),
				},
				IsTypeParam: false,
			}
		},
		nil,
	)
	return _type
}

func get_type_constraints(anns adlast.Annotations) []string {
	jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_TypeParamConstraintList(), goadl.RESOLVER)
	lst, err := goadl.GetAnnotation(anns, TypeParamConstraintListSN, jb)
	if err != nil {
		panic(err)
	}
	if lst != nil {
		return *lst
	}
	return []string{}
}

func (in *baseGen) gotype_ref_customtype(
	decl *adlast.Decl,
	typeExpr adlast.TypeExpr,
	unionTypeParams *typeParam,
	anns adlast.Annotations,
) goTypeExpr {
	jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_GoCustomType(), goadl.RESOLVER)
	gct, err := goadl.GetAnnotation(decl.Annotations, goCustomTypeSN, jb)
	if err != nil {
		panic(fmt.Errorf("error getting go_custom_type annotation for %v. err : %w", decl.Name, err))
	}
	pkg := gct.Gotype.Import_path[strings.LastIndex(gct.Gotype.Import_path, "/")+1:]
	spec := importSpec{
		Path:    gct.Gotype.Import_path,
		Name:    gct.Gotype.Pkg,
		Aliased: gct.Gotype.Pkg != pkg,
	}
	in.imports.addSpec(spec)
	got := goTypeExpr{
		Pkg:  gct.Gotype.Pkg,
		Type: gct.Gotype.Name,
		TypeParams: typeParam{
			params: slices.Map(typeExpr.Parameters, func(a adlast.TypeExpr) param {
				pt := in.goType(a, unionTypeParams, anns)
				return param{pt.String(), !pt.IsTypeParam}
			}),
			// type_constraints: get_type_constraints(anns),
			type_constraints: []string{},
		},
		IsTypeParam: false,
	}
	return got
}

func (in *baseGen) PrimitiveMap(
	p string,
	params []adlast.TypeExpr,
	unionTypeParams *typeParam,
	anns adlast.Annotations,
) goTypeExpr {
	r, has := primitiveMap[p]
	if has {
		return goTypeExpr{"", r, typeParam{}, typeParam{}, false}
	}
	elem := in.goType(params[0], unionTypeParams, anns)
	switch p {
	case "TypeToken":
		pkg, err := in.GoImport("adlast")
		if err != nil {
			panic(err)
		}
		tp := typeParam{
			params: []param{{name: elem.String(), concrete: !elem.IsTypeParam}},
		}
		return goTypeExpr{"", pkg + "ATypeExpr", tp, tp, false}
	case "Vector":
		return goTypeExpr{"", "[]" + elem.sansTypeParam(), elem.TypeParams, typeParam{}, elem.IsTypeParam}
	case "StringMap":
		return goTypeExpr{"", "map[string]" + elem.sansTypeParam(), elem.TypeParams, typeParam{}, elem.IsTypeParam}
	case "Nullable":
		return goTypeExpr{"", "*" + elem.sansTypeParam(), elem.TypeParams, typeParam{}, elem.IsTypeParam}
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
