package exer01

import (
	"bytes"
	"fmt"
	"testing"

	"adl_testing/exer01/simple_union"
	"adl_testing/exer01/struct01"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/adl-lang/goadl_rt/v2/adljson"
)

func TestEnum(_ *testing.T) {
	x := simple_union.Make_UnionOfVoids_A(struct{}{})
	out := &bytes.Buffer{}
	enc := adljson.NewEncoder(out, simple_union.Texpr_UnionOfVoids(), goadl.RESOLVER)
	enc.Encode(x)
	fmt.Printf("%s\n", out.String())
}

func TestUnion(_ *testing.T) {
	x := simple_union.Make_UnionOfPrimitives_A(42)
	out := &bytes.Buffer{}
	enc := adljson.NewEncoder(out, simple_union.Texpr_UnionOfPrimitives(), goadl.RESOLVER)
	enc.Encode(x)
	fmt.Printf("%s\n", out.String())
}

func TestUnions(_ *testing.T) {
	xs := []simple_union.UnionOfPrimitives{
		simple_union.Make_UnionOfPrimitives_A(42),
		simple_union.Make_UnionOfPrimitives_B(41),
		simple_union.Make_UnionOfPrimitives_c(true),
		simple_union.Make_UnionOfPrimitives_d(41.01),
		simple_union.Make_UnionOfPrimitives_e("sdfadf"),
		simple_union.Make_UnionOfPrimitives_f([]string{"a", "b", "v"}),
		simple_union.Make_UnionOfPrimitives_g(struct{}{}),
	}
	out := &bytes.Buffer{}
	te := goadl.ATypeExpr[[]simple_union.UnionOfPrimitives]{
		Value: goadl.TypeExpr{
			TypeRef: goadl.TypeRef{
				Branch: goadl.TypeRefBranch_Primitive("Vector"),
			},
			Parameters: []goadl.TypeExpr{
				{
					TypeRef: goadl.TypeRef{
						Branch: goadl.TypeRefBranch_Reference{
							ModuleName: "exer01.simple_union",
							Name:       "UnionOfPrimitives",
						},
					},
					Parameters: []goadl.TypeExpr{},
				},
			},
		},
	}
	enc := adljson.NewEncoder(out, te, goadl.RESOLVER)
	enc.Encode(xs)
	fmt.Printf("%s\n", out.String())
}

func TestStruct01(t *testing.T) {
	a := "a"
	x := struct01.Struct01{
		A: struct{}{},
		B: 41,
		C: "",
		D: map[string]any{
			"a": 1234567890,
		},
		E: []string{"a", "b", "c"},
		F: map[string][]string{"a": {"z"}, "b": {"x"}, "c": {"y"}},
		I: &a,
		J: struct01.B{
			A: "sfd",
		},
	}

	out := &bytes.Buffer{}
	enc := adljson.NewEncoder[struct01.Struct01](out, struct01.Texpr_Struct01(), goadl.RESOLVER)
	enc.Encode(x)
	fmt.Printf("%s\n", out.String())
}
