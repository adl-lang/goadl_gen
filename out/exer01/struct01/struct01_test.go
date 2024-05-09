package struct01

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/adl-lang/goadl_rt/v2/json"
)

func TestXxx(t *testing.T) {
	a := "a"
	x := Struct01{
		A: struct{}{},
		B: 41,
		C: "",
		D: map[string]any{
			"a": 1234567890,
		},
		E: []string{"a", "b", "c"},
		F: map[string][]string{"a": {"z"}, "b": {"x"}, "c": {"y"}},
		I: &a,
		J: B{
			A: "sfd",
		},
	}

	v := reflect.ValueOf(x)
	f0 := v.Field(0)
	fmt.Printf("%v\n", f0.IsZero())
	f2 := v.Field(2)
	fmt.Printf("%v\n", f2.IsZero())

	out := &bytes.Buffer{}
	enc := json.NewEncoder[Struct01](out, Texpr_Struct01(), goadl.RESOLVER)
	enc.Encode(x)
	fmt.Printf("%s\n", string(out.Bytes()))
}
