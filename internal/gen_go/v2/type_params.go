package gen_go_v2

import (
	"fmt"
	"strconv"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/adl-lang/goadlc/internal/gen_go/fn/slices"
)

func new_typeParams(ps []string) typeParam {
	return typeParam{
		ps:    ps,
		added: false,
	}
}

func getTypeParams(decl goadl.Decl) typeParam {
	return goadl.HandleP_DeclType[typeParam](
		decl.Type.Branch,
		func(struct_ goadl.Struct) typeParam {
			return typeParam{
				ps:    struct_.TypeParams,
				added: false,
			}
		},
		func(union_ goadl.Union) typeParam {
			return typeParam{
				ps:    union_.TypeParams,
				added: false,
			}
		},
		func(type_ goadl.TypeDef) typeParam {
			return typeParam{
				ps:    type_.TypeParams,
				added: false,
			}
		},
		func(newtype_ goadl.NewType) typeParam {
			return typeParam{
				ps:    newtype_.TypeParams,
				added: false,
			}
		},
	)
}

// func usedTypeParams(te goadl.TypeExpr) typeParam {
// 	lside := slices.FlatMap[goadl.TypeExpr, string](te.Parameters, func(a goadl.TypeExpr) []string {
// 		goadl.Handle_P_TypeRef[[]string](
// 			a.TypeRef.Branch,
// 			func(primitive string) []string {
// 				return []string{}
// 			},
// 			func(typeParam string) []string {
// 				return []string{typeParam}
// 			},
// 			func(reference goadl.ScopedName) []string {

// 			},
// 		)
// 	})
// }

type typeParam struct {
	ps []string
	// isTypeParam bool
	added bool
}

func (tp typeParam) AddParam(newp string) typeParam {
	psMap := make(map[string]bool)
	tp0 := make([]string, len(tp.ps)+1)
	for i, p := range tp.ps {
		tp0[i] = p
		psMap[p] = true
	}

	tp0[len(tp.ps)] = newp
	if psMap[tp0[len(tp.ps)]] {
		n := uint64(1)
		for {
			n++
			tp0[len(tp.ps)] = newp + strconv.FormatUint(n, 10)
			if !psMap[tp0[len(tp.ps)]] {
				break
			}
		}
	}
	return typeParam{
		ps: tp0,
		// tp.isTypeParam,
		added: true,
	}
}
func (tp typeParam) Has() bool {
	return (!tp.added && len(tp.ps) != 0) || len(tp.ps) != 1
}
func (tp typeParam) Last() string {
	if len(tp.ps) == 0 {
		return ""
	}
	return tp.ps[len(tp.ps)-1]
}
func (tp typeParam) LSide() string {
	// if tp.isTypeParam {
	// 	return ""
	// }
	if len(tp.ps) == 0 {
		return ""
	}
	return "[" + strings.Join(slices.Map(tp.ps, func(e string) string { return e + " any" }), ", ") + "]"
}
func (tp typeParam) RSide() string {
	// if tp.isTypeParam {
	// 	return ""
	// }
	if len(tp.ps) == 0 {
		return ""
	}
	return "[" + strings.Join(tp.ps, ",") + "]"
}
func (tp typeParam) TexprArgs() string {
	// if tp.isTypeParam {
	// 	return ""
	// }
	if len(tp.ps) == 0 {
		return ""
	}
	return strings.Join(slices.Map(tp.ps, func(e string) string { return fmt.Sprintf("%s goadl.ATypeExpr[%s]", strings.ToLower(e), e) }), ", ")
}
func (tp typeParam) TexprValues() string {
	// if tp.isTypeParam {
	// 	return ""
	// }
	if len(tp.ps) == 0 {
		return ""
	}
	return strings.Join(slices.Map(tp.ps, func(e string) string { return fmt.Sprintf("%s.Value", strings.ToLower(e)) }), ", ")
}

func (tp typeParam) TpArgs() string {
	if len(tp.ps) == 0 {
		return ""
	}
	return "[any" + strings.Repeat(",any", len(tp.ps)-1) + "]"
}
