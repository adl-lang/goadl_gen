package out_test

import (
	"bytes"
	"reflect"
	"testing"

	"adl_testing/exer03/generics"

	goadl "github.com/adl-lang/goadl_rt/v3"
)

func TestGenericEncode(t *testing.T) {
	x := generics.Make_Abc[int64, string](
		1, []string{"a"}, generics.Make_Def_a[int64, string](3),
	)

	x.Kids = []generics.Abc[int64, string]{x}

	// x := generics.Abc[int64, string]{
	// 	A: 1,
	// 	B: []string{"a"},
	// 	C: 2,
	// 	D: generics.Make_Def_a[int64, string](3),
	// }
	out := &bytes.Buffer{}
	enc := goadl.NewEncoder(out, generics.Texpr_Abc(goadl.Texpr_Int64(), goadl.Texpr_String()), goadl.RESOLVER)
	err := enc.Encode(x)
	if err != nil {
		t.Errorf("%v", err)
	}
	dec := goadl.NewDecoder(out, generics.Texpr_Abc(goadl.Texpr_Int64(), goadl.Texpr_String()), goadl.RESOLVER)
	var y generics.Abc[int64, string]
	err = dec.Decode(&y)
	if err != nil {
		t.Errorf("%v", err)
	}
	if !reflect.DeepEqual(x, y) {
		t.Errorf("!=\n%v\n%v\n", x, y)
	}
	// out2 := bytes.Buffer{}
	// json.Indent(&out2, out.Bytes(), "", "  ")
	// fmt.Printf("%s\n", out2.String())
	// o2, _ := json.Marshal(x)
	// fmt.Printf("%s\n", string(o2))

}
