package exer02_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"adl_testing/exer03/generics"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/adl-lang/goadl_rt/v2/adljson"
)

func TestXxx(t *testing.T) {
	x := generics.Make_Abc[int64, string](
		1, []string{"a"}, generics.Make_Def_a[int64, string](3),
	)

	// x := generics.Abc[int64, string]{
	// 	A: 1,
	// 	B: []string{"a"},
	// 	C: 2,
	// 	D: generics.Make_Def_a[int64, string](3),
	// }
	out := &bytes.Buffer{}
	enc := adljson.NewEncoder(out, generics.Texpr_Abc(goadl.Texpr_Int64(), goadl.Texpr_String()), goadl.RESOLVER)
	err := enc.Encode(x)
	if err != nil {
		t.Errorf("%v", err)
	}
	fmt.Printf("%s\n", out.String())
	o2, _ := json.Marshal(x)
	fmt.Printf("%s\n", string(o2))

}
