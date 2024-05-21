package gen_go_v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/adl-lang/goadlc/internal/fn/slices"
)

func (*generator) JsonEncode(val any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := enc.Encode(val)
	if err != nil {
		panic(err)
	}
	return string(bytes.Trim(buf.Bytes(), "\n"))
}

var declTexpr = goadl.ATypeExpr[goadl.Decl]{
	Value: goadl.TypeExpr{
		TypeRef: goadl.TypeRef{
			Branch: goadl.TypeRefBranch_Reference{
				ModuleName: "sys.adlast",
				Name:       "Decl",
			},
		},
		Parameters: []goadl.TypeExpr{},
	},
}

func (bg *generator) GoDeclValue(val goadl.Decl) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := enc.Encode(val)
	if err != nil {
		panic(err)
	}
	var m any
	dec := json.NewDecoder(&buf)
	err = dec.Decode(&m)
	if err != nil {
		panic(err)
	}
	return bg.GoValue(typeParam{}, declTexpr.Value, m)
}

type ctxPath []string

func (cp ctxPath) String() string {
	return "[" + strings.Join(cp, ",") + "]"
}

type valContext struct {
	path ctxPath
}

func (bg *baseGen) GoValue(decl_tp typeParam, te goadl.TypeExpr, val any) string {
	ctx := valContext{path: []string{"$"}}
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in GoValue %v\n%v", r, string(debug.Stack()))
			panic(r)
		}
	}()
	return bg.goValue(ctx, decl_tp, te, val)
}

func (bg *baseGen) goValue(ctx valContext, decl_tp typeParam, te goadl.TypeExpr, val any) string {
	return goadl.Handle_TypeRef[string](
		te.TypeRef.Branch,
		func(primitive string) string {
			ctx0 := valContext{append(ctx.path, primitive)}
			return bg.goValuePrimitive(ctx0, decl_tp, te, primitive, val)
		},
		func(typeParam string) string {
			// return typeParam
			panic("???GoValue:typeParam " + typeParam)
		},
		func(ref goadl.ScopedName) string {
			ctx0 := valContext{append(ctx.path, ref.Name)}
			decl, ok := bg.resolver(ref)
			if !ok {
				panic(fmt.Errorf("cannot resolve %v", ref))
			}
			tp := typeParamsFromDecl(*decl)

			// decl.
			return bg.goValueScopedName(ctx0, tp, te, ref, val)
		},
	)
}

func (bg *baseGen) goValuePrimitive(
	ctx valContext,
	decl_tp typeParam,
	te goadl.TypeExpr,
	primitive string,
	val any,
) string {
	switch primitive {
	case "Int8", "Int16", "Int32", "Int64",
		"Word8", "Word16", "Word32", "Word64",
		"Bool", "Float", "Double":
		return fmt.Sprintf("%v", val)
	case "String":
		return fmt.Sprintf(`"%s"`, val)
	// case "ByteVector":
	case "Void":
		return "struct{}{}"
	case "Json":
		return fmt.Sprintf("%+#v", val)
		// panic(fmt.Errorf("path %v - todo json %+#v", ctx.path, val))
	case "Vector":
		rv := reflect.ValueOf(val)
		vs := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			v := rv.Index(i)
			ctx0 := valContext{append(ctx.path, fmt.Sprintf("[%d]", i))}
			vs[i] = bg.goValue(ctx0, decl_tp, te.Parameters[0], v.Interface())
		}
		if len(vs) == 0 {
			return fmt.Sprintf("[]%s{}", bg.GoType(te.Parameters[0]))
		}
		vss := strings.Join(vs, ",\n")
		return fmt.Sprintf("[]%s{\n%s,\n}", bg.GoType(te.Parameters[0]), vss)
	case "StringMap":
		m := val.(map[string]any)
		vs := make(kvBy, 0, len(m))
		for k, v := range m {
			ctx0 := valContext{append(ctx.path, fmt.Sprintf("%s:", k))}
			vs = append(vs, kv{k, bg.goValue(ctx0, decl_tp, te.Parameters[0], v)})
		}
		if len(vs) == 0 {
			return fmt.Sprintf("map[string]%s{}", bg.GoType(te.Parameters[0]))
		}
		sort.Sort(vs)
		return fmt.Sprintf("map[string]%s{\n%s,\n}", bg.GoType(te.Parameters[0]), vs)
	case "Nullable":
		if val == nil {
			return "nil"
		}
		return "&" + bg.goValue(ctx, decl_tp, te.Parameters[0], val)
	}
	panic("??? GoValuePrimitive")
}

// func (bg *baseGen) goValuePrimitiveJson(
// 	ctx valContext,
// 	decl_tp typeParam,
// 	te goadl.TypeExpr,
// 	primitive string,
// 	val any,
// ) string {
// 	switch t := val.(type) {
// 	case []any:
// 	case map[string]any:
// 	}
// }

func (bg *baseGen) goValueScopedName(
	ctx valContext,
	decl_tp typeParam,
	te goadl.TypeExpr,
	ref goadl.ScopedName,
	val any,
) string {
	gt := bg.GoType(te)

	if len(decl_tp.ps) != len(te.Parameters) {
		panic(fmt.Errorf("path : %s - len(decl typeparams) != len(gt.TypeParams.ps)\n%+v\n%+v",
			ctx.path, decl_tp.ps, te.Parameters))
	}
	tpMap := map[string]goadl.TypeExpr{}
	for i, tp := range decl_tp.ps {
		tpMap[tp] = te.Parameters[i]
	}

	decl, ok := bg.declMap[ref.ModuleName+"::"+ref.Name]
	if !ok {
		panic(fmt.Errorf("path %v - decl not in map : %v", ctx.path, ref))
	}
	return goadl.Handle_DeclType[string](
		decl.Type.Branch,
		func(struct_ goadl.Struct) string {
			m := val.(map[string]any)
			ret := slices.FlatMap[goadl.Field, string](struct_.Fields, func(f goadl.Field) []string {
				ret := []string{}
				if v, ok := m[f.SerializedName]; ok {
					monoTe := defunctionalizeTe(tpMap, f.TypeExpr)
					ctx0 := valContext{append(ctx.path, f.Name)}
					fgv := bg.goValue(ctx0, decl_tp, monoTe, v)
					// fgv := bg.goValue(ctx0, decl_tp, monoTe, v)
					ret = append(ret, fmt.Sprintf(`%s: %s`, public(f.Name), fgv))
				}
				if _, ok := m[f.SerializedName]; !ok && f.Default.Just != nil {
					monoTe := defunctionalizeTe(tpMap, f.TypeExpr)
					ctx0 := valContext{append(ctx.path, f.Name)}
					fgv := bg.goValue(ctx0, decl_tp, monoTe, *f.Default.Just)
					ret = append(ret, fmt.Sprintf(`%s: %s`, public(f.Name), fgv))
				}
				return ret
			})
			return fmt.Sprintf("%s{\n%s,\n}", gt.String(), strings.Join(ret, ",\n"))
		},
		func(union_ goadl.Union) string {
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
					panic(fmt.Sprintf("path %v - expect an object with one and only element received %v - %v", ctx.path, len(t), t))
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
			monoTe := defunctionalizeTe(tpMap, fld.TypeExpr)
			// monoTe.Parameters
			f_tp := typeParam{
				ps: slices.Map[goadl.TypeExpr, string](monoTe.Parameters, func(a goadl.TypeExpr) string {
					return bg.GoType(a).Type
				}),
			}

			if f_tp0, ok := fld.TypeExpr.TypeRef.Branch.(goadl.TypeRefBranch_TypeParam); ok {
				monoFtp, ok := tpMap[string(f_tp0)]
				if !ok {
					panic(fmt.Errorf("type param not found"))
				}
				monoGt := bg.GoType(monoFtp)
				f_tp = typeParam{
					ps: []string{monoGt.Type},
				}
			}
			ctx0 := valContext{append(ctx.path, fld.Name)}
			pkg := ""
			if gt.Pkg != "" {
				pkg = gt.Pkg + "."
			}
			ret := []string{
				fmt.Sprintf("%s%s_%s%s{\nV: %v}",
					pkg,
					decl.Name,
					public(fld.Name),
					f_tp.RSide(),
					bg.goValue(ctx0, decl_tp, monoTe, v),
				),
			}
			return fmt.Sprintf("%s{\nBranch: %s,\n}", gt.String(), strings.Join(ret, ",\n"))

		},
		func(type_ goadl.TypeDef) string {
			monoTe := defunctionalizeTe(tpMap, type_.TypeExpr)
			return bg.goValue(ctx, typeParam{ps: type_.TypeParams}, monoTe, val)
			// panic(fmt.Errorf("path %v - todo typede", ctx.path))
		},
		func(newtype_ goadl.NewType) string {
			monoTe := defunctionalizeTe(tpMap, newtype_.TypeExpr)
			return fmt.Sprintf("%s(%s)", gt.String(), bg.goValue(ctx, typeParam{ps: newtype_.TypeParams}, monoTe, val))
		},
		nil,
	)
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
