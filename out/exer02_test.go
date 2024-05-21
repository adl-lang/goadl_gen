package out_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"adl_testing/exer02/a"
	"adl_testing/exer02/b"

	goadl "github.com/adl-lang/goadl_rt/v3"
)

func TestExec02Encode(t *testing.T) {
	x := a.A{
		A: b.B{},
	}
	out := &bytes.Buffer{}
	enc := goadl.NewEncoder[a.A](out, a.Texpr_A(), goadl.RESOLVER)
	enc.Encode(x)
	fmt.Printf("%s\n", string(out.Bytes()))
	o2, _ := json.Marshal(x)
	fmt.Printf("%s\n", string(o2))

}
