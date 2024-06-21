package gen_go

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/samber/lo"
)

func typeParamsFromDecl(decl adlast.Decl) typeParam {
	tp := adlast.Handle_DeclType[typeParam](
		decl.Type_,
		func(struct_ adlast.Struct) typeParam {
			return typeParam{
				params:           lo.Map(struct_.TypeParams, func(name string, _ int) param { return param{name: name, concrete: false} }),
				type_constraints: []string{},
				added:            false,
			}
		},
		func(union_ adlast.Union) typeParam {
			return typeParam{
				params:           lo.Map(union_.TypeParams, func(name string, _ int) param { return param{name: name, concrete: false} }),
				type_constraints: []string{},
				added:            false,
			}
		},
		func(type_ adlast.TypeDef) typeParam {
			return typeParam{
				params:           lo.Map(type_.TypeParams, func(name string, _ int) param { return param{name: name, concrete: false} }),
				type_constraints: []string{},
				added:            false,
			}
		},
		func(newtype_ adlast.NewType) typeParam {
			return typeParam{
				params:           lo.Map(newtype_.TypeParams, func(name string, _ int) param { return param{name: name, concrete: false} }),
				type_constraints: []string{},
				added:            false,
			}
		},
		nil,
	)
	jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_TypeParamConstraintList(), goadl.RESOLVER)
	lst, err := goadl.GetAnnotation(decl.Annotations, TypeParamConstraintListSN, jb)
	if err != nil {
		panic(err)
	}
	if lst != nil {
		tp.type_constraints = *lst
	}
	return tp
}

var TypeParamConstraintListSN = adlast.Make_ScopedName("adlc.config.go_", "TypeParamConstraintList")

type param struct {
	name     string
	concrete bool
}

type typeParam struct {
	params           []param
	type_constraints []string
	added            bool
}

func (tp typeParam) MarshalJSON() ([]byte, error) {
	return json.Marshal(tp.params)
}

func (tp typeParam) AddParam(newp string) typeParam {
	psMap := make(map[string]bool)
	tp0 := make([]param, len(tp.params)+1)
	for i, p := range tp.params {
		tp0[i] = p
		psMap[p.name] = true
	}

	tp0[len(tp.params)] = param{name: newp, concrete: false}
	if psMap[tp0[len(tp.params)].name] {
		n := uint64(1)
		for {
			n++
			tp0[len(tp.params)] = param{name: newp + strconv.FormatUint(n, 10), concrete: false}
			if !psMap[tp0[len(tp.params)].name] {
				break
			}
		}
	}
	return typeParam{
		params:           tp0,
		type_constraints: tp.type_constraints,
		// tp.isTypeParam,
		added: true,
	}
}
func (tp typeParam) Has() bool {
	return (!tp.added && len(tp.params) != 0) || len(tp.params) != 1
}
func (tp typeParam) Last() string {
	if len(tp.params) == 0 {
		return ""
	}
	return tp.params[len(tp.params)-1].name
}

func (tp typeParam) LSide() string {
	if len(tp.params) == 0 {
		return ""
	}
	names := lo.Map[param, string](tp.params, func(a param, i int) string { return a.name })
	return "[" + strings.Join(lo.Map(names, func(e string, i int) string {
		if i+1 <= len(tp.type_constraints) {
			return e + " " + tp.type_constraints[i]
		}
		return e + " any"
	}), ", ") + "]"
}
func (tp typeParam) RSide() string {
	if len(tp.params) == 0 {
		return ""
	}
	names := lo.Map[param, string](tp.params, func(a param, i int) string { return a.name })
	return "[" + strings.Join(names, ",") + "]"
}
func (tp typeParam) TexprArgs() string {
	if len(tp.params) == 0 {
		return ""
	}
	names := lo.Map[param, string](tp.params, func(a param, i int) string { return a.name })
	return strings.Join(lo.Map(names, func(e string, _ int) string { return fmt.Sprintf("%s adlast.ATypeExpr[%s]", strings.ToLower(e), e) }), ", ")
}
func (tp typeParam) TexprValues() string {
	if len(tp.params) == 0 {
		return ""
	}
	names := lo.Map[param, string](tp.params, func(a param, i int) string { return a.name })
	return strings.Join(lo.Map(names, func(e string, _ int) string { return fmt.Sprintf("%s.Value", strings.ToLower(e)) }), ", ")
}

func (tp typeParam) TpArgs() string {
	if len(tp.params) == 0 {
		return ""
	}
	return "[any" + strings.Repeat(",any", len(tp.params)-1) + "]"
}
