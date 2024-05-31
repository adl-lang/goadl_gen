package gen_go

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadl_rt/v3/sys/types"
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

func (bg *generator) GoDeclValue(val adlast.Decl) string {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in GoDeclValue %v\n%v", r, string(debug.Stack()))
			panic(r)
		}
	}()
	var buf bytes.Buffer
	enc := goadl.CreateJsonEncodeBinding(goadl.Texpr_Decl(), goadl.RESOLVER)
	err := enc.Encode(&buf, val)
	if err != nil {
		fmt.Fprintf(os.Stderr, "!!!! encode error %v\n", err)
		panic(err)
	}
	var m any
	dec := json.NewDecoder(&buf)
	// dec.UseNumber()
	err = dec.Decode(&m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "!!!! decode error %v\n", err)
		panic(err)
	}
	bg.genAdlAst = true
	// TODO make it so we GoValue can take both an any and a decl
	// or make it so the encoder can encode to an any
	return bg.GoValue(val.Annotations, goadl.Texpr_Decl().Value, m)
}

func (bg *generator) GoValue(
	anns adlast.Annotations,
	te adlast.TypeExpr,
	val any,
) string {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in GoValue %v\n%v", r, string(debug.Stack()))
			panic(r)
		}
	}()
	return bg.goValue(anns, te, val)
}

func (bg *generator) goValue(
	anns adlast.Annotations,
	te adlast.TypeExpr,
	val any,
) string {
	return adlast.Handle_TypeRef[string](
		te.TypeRef,
		func(primitive string) string {
			return bg.goValuePrimitive(anns, te, primitive, val)
		},
		func(typeParam string) string {
			panic("unbound typeParam " + typeParam)
		},
		func(ref adlast.ScopedName) string {
			gt := bg.GoType(te)
			decl, ok := bg.resolver(ref)
			if !ok {
				panic(fmt.Errorf("cannot resolve %v", ref))
			}
			tbind := goadl.CreateDecBoundTypeParams(goadl.TypeParamsFromDecl(*decl), te.Parameters)
			if goadl.HasAnnotation(decl.Annotations, goCustomTypeSN) {
				monoTe := goadl.SubstituteTypeBindings(tbind, te)
				return bg.goCustomType(decl, monoTe, gt, val)
			}
			return adlast.Handle_DeclType(
				decl.Type_,
				func(struct_ adlast.Struct) string {
					return bg.goStruct(struct_, tbind, gt, val)
				},
				func(union_ adlast.Union) string {
					return bg.goUnion(union_, decl.Name, tbind, gt, val)
				},
				func(type_ adlast.TypeDef) string {
					monoTe := goadl.SubstituteTypeBindings(tbind, type_.TypeExpr)
					return bg.goValue(decl.Annotations, monoTe, val)
				},
				func(newtype_ adlast.NewType) string {
					monoTe := goadl.SubstituteTypeBindings(tbind, newtype_.TypeExpr)
					return bg.goValue(decl.Annotations, monoTe, val)
				},
				nil,
			)
		},
		nil,
	)
}

func (bg *generator) goCustomType(
	decl *adlast.Decl,
	monoTe adlast.TypeExpr,
	gt goTypeExpr,
	val any,
) string {
	jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_GoCustomType(), goadl.RESOLVER)
	gct, err := goadl.GetAnnotation(decl.Annotations, goCustomTypeSN, jb)
	if err != nil {
		panic(err)
	}
	{
		pkg := gct.Gotype.Import_path[strings.LastIndex(gct.Gotype.Import_path, "/")+1:]
		spec := importSpec{
			Path:    gct.Gotype.Import_path,
			Name:    gct.Gotype.Pkg,
			Aliased: gct.Gotype.Pkg != pkg,
		}
		bg.imports.addSpec(spec)
	}

	gen := &generator{
		baseGen: bg.baseGen,
		rr:      templateRenderer{t: templates},
	}

	typeExprStrs := slices.Map[adlast.TypeExpr, string](monoTe.Parameters, func(a adlast.TypeExpr) string {
		return bg.strRep(a)
	})

	{
		pkg := gct.Helpers.Import_path[strings.LastIndex(gct.Helpers.Import_path, "/")+1:]
		spec := importSpec{
			Path:    gct.Helpers.Import_path,
			Name:    gct.Helpers.Pkg,
			Aliased: gct.Helpers.Pkg != pkg,
		}
		bg.imports.addSpec(spec)
	}
	gen.rr.Render(custTypeConstructionParams{
		G:                gen,
		Name:             decl.Name,
		ModuleName:       bg.moduleName,
		TypeParams:       gt.TypeParams,
		AnyValue:         fmt.Sprintf("%+#v", val),
		CustomType:       gct.Gotype.Pkg + "." + gct.Gotype.Name,
		CustomTypeHelper: gct.Helpers.Pkg + "." + gct.Helpers.Name,
		TypeExprStrs:     typeExprStrs,
	})
	return gen.rr.buf.String()
}

func (bg *generator) strRep(te adlast.TypeExpr) string {
	br := adlast.Handle_TypeRef[string](
		te.TypeRef,
		func(primitive string) string {
			return fmt.Sprintf(`adlast.TypeRef_Primitive{V: "%s"}`, primitive)
		},
		func(typeParam string) string {
			panic("typeParm not valid in mono te")
		},
		func(reference adlast.ScopedName) string {
			return fmt.Sprintf(`adlast.TypeRef_Reference{V: adlast.ScopedName{ModuleName: "%s",Name: "%s"}}`, reference.ModuleName, reference.Name)
		},
		nil,
	)
	bg.GoImport("adlast")
	params := slices.Map[adlast.TypeExpr, string](te.Parameters, func(a adlast.TypeExpr) string {
		return bg.strRep(a)
	})
	return fmt.Sprintf(`adlast.TypeExpr{TypeRef: adlast.TypeRef{Branch: %s},Parameters: []adlast.TypeExpr{%s}}`, br, strings.Join(params, ","))
}

type custTypeConstructionParams struct {
	G                *generator
	ModuleName       string
	Name             string
	TypeParams       typeParam
	AnyValue         string
	CustomType       string
	CustomTypeHelper string
	TypeExprStrs     []string
}

func (bg *generator) goStruct(
	struct_ adlast.Struct,
	tbind []goadl.TypeBinding,
	gt goTypeExpr,
	val any,
) string {
	m := val.(map[string]any)
	ret := slices.FlatMap[adlast.Field, string](struct_.Fields, func(fld adlast.Field) []string {
		ret := []string{}
		if bg.genAdlAst && fld.Name == "annotations" {
			bg.GoImport("customtypes")
			anns := m[fld.SerializedName].([]any)
			annvs := []string{}
			for _, mapEntry := range anns {
				ann := mapEntry.(map[string]any)
				k := ann["k"].(map[string]any)
				v := ann["v"]
				mn := k["moduleName"]
				na := k["name"]
				annvs = append(annvs, fmt.Sprintf(`adlast.Make_ScopedName("%s", "%s"): %+#v`, mn, na, v))
			}
			ret = append(ret, fmt.Sprintf(`customtypes.MapMap[adlast.ScopedName, any]{%s}`, strings.Join(annvs, ",")))
			// ret = append(ret, fmt.Sprintf(`%s: customtypes.MapMap[adlast.ScopedName, any]{%s}`, public(fld.Name), strings.Join(annvs, ",")))
			return ret
		}
		if v, ok := m[fld.SerializedName]; ok {
			monoTe := goadl.SubstituteTypeBindings(tbind, fld.TypeExpr)
			fgv := bg.goValue(fld.Annotations, monoTe, v)
			ret = append(ret, fgv)
			// ret = append(ret, fmt.Sprintf(`%s: %s`, public(fld.Name), fgv))
		}
		if _, ok := m[fld.SerializedName]; !ok {
			types.Handle_Maybe[any, any](
				fld.Default,
				func(nothing struct{}) any {
					return nil
				},
				func(just any) any {
					val := reflect.ValueOf(just).Interface()
					monoTe := goadl.SubstituteTypeBindings(tbind, fld.TypeExpr)
					fgv := bg.goValue(fld.Annotations, monoTe, val)
					ret = append(ret, fgv)
					// ret = append(ret, fmt.Sprintf(`%s: %s`, public(fld.Name), fgv))
					return nil
				},
				nil,
			)
		}
		return ret
	})
	pkg := ""
	if gt.Pkg != "" {
		pkg = gt.Pkg + "."
	}
	return fmt.Sprintf("%sMakeAll_%s%s(\n%s,\n)", pkg, gt.Type, gt.TypeParams.RSide(), strings.Join(ret, ",\n"))
}

func (bg *generator) goUnion(
	union_ adlast.Union,
	decl_name string,
	tbind []goadl.TypeBinding,
	gt goTypeExpr,
	val any,
) string {
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
			panic(fmt.Sprintf("expect an object with one and only element received %v - %v", len(t), t))
		}
		for k0, v0 := range t {
			k = k0
			v = v0
		}
	default:
		panic(fmt.Errorf("union: expect an object received %v '%v'", reflect.TypeOf(val), val))
	}
	var fld *adlast.Field
	for _, f0 := range union_.Fields {
		if f0.SerializedName == k {
			fld = &f0
			break
		}
	}
	if fld == nil {
		panic(fmt.Errorf("unexpected branch - no type registered '%v'", k))
	}
	monoTe := goadl.SubstituteTypeBindings(tbind, fld.TypeExpr)
	// f_tp := typeParam{
	// 	ps: slices.Map[adlast.TypeExpr, string](monoTe.Parameters, func(a adlast.TypeExpr) string {
	// 		return bg.GoType(a).Type
	// 	}),
	// }

	// if f_tp0, ok := fld.TypeExpr.TypeRef.Cast_typeParam(); ok {
	// 	// if f_tp0, ok := fld.TypeExpr.TypeRef.Branch.(adlast.TypeRef_TypeParam); ok {
	// 	// 	f_tp0 := f_tp0.V
	// 	ok := false
	// 	for _, x := range tbind {
	// 		if x.Name == f_tp0 {
	// 			ok = true
	// 			monoGt := bg.GoType(x.Value)
	// 			f_tp = typeParam{
	// 				ps: []string{monoGt.Type},
	// 			}
	// 			break
	// 		}
	// 	}
	// 	if !ok {
	// 		panic(fmt.Errorf("type param not found"))
	// 	}
	// }

	pkg := ""
	if gt.Pkg != "" {
		pkg = gt.Pkg + "."
	}

	return fmt.Sprintf("%sMake_%s_%s%s(\n%s,\n)", pkg, gt.Type, fld.Name, gt.TypeParams.RSide(), bg.goValue(fld.Annotations, monoTe, v))

	// ret := []string{
	// 	fmt.Sprintf("%s%s_%s%s{\nV: %v}",
	// 		pkg,
	// 		decl_name,
	// 		public(fld.Name),
	// 		f_tp.RSide(),
	// 		bg.goValue(fld.Annotations, monoTe, v),
	// 	),
	// }
	// return fmt.Sprintf("%s{\nBranch: %s,\n}", gt.String(), strings.Join(ret, ",\n"))
}

func (bg *generator) goValuePrimitive(
	anns adlast.Annotations,
	te adlast.TypeExpr,
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
			vs[i] = bg.goValue(anns, te.Parameters[0], v.Interface())
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
			vs = append(vs, kv{k, bg.goValue(anns, te.Parameters[0], v)})
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
		gl, _ := bg.GoImport("goadl")
		return gl + "Addr(" + bg.goValue(anns, te.Parameters[0], val) + ")"
	}
	panic("Unknown GoValuePrimitive")
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
func (a kvBy) Less(i, j int) bool { return a[i].k < a[j].k }
