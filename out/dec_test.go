package out_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"adl_testing/decode/test01"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadl_rt/v3/types"
)

func TestNewTypePrim(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString("42")
	dec := goadl.NewDecoder(out, test01.Texpr_A(), goadl.RESOLVER)
	var y test01.A
	err := dec.Decode(&y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if y != 42 {
		t.Fail()
	}
}

func TestStructOfPrim(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"a":42}`)
	dec := goadl.NewDecoder(out, test01.Texpr_B(), goadl.RESOLVER)
	var y test01.B
	err := dec.Decode(&y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.B{A: 42}) {
		t.Fail()
	}
}

func TestStructOfStruct(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"b": {"a":42}, "c": {"a": 3}}`)
	dec := goadl.NewDecoder(out, test01.Texpr_C(), goadl.RESOLVER)
	var y test01.C
	err := dec.Decode(&y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.C{B: test01.B{A: 42}, C: test01.B{A: 3}}) {
		t.Fail()
	}
}

// These aren't test, rather snipped of who to use refection
func TestReflectConstruction(t *testing.T) {
	_ = t
	dT := reflect.TypeFor[test01.D]()
	dv := reflect.New(dT).Elem()

	b := test01.D_A{
		V: 1,
	}
	s0 := reflect.ValueOf(b)
	f0 := dv.Field(0)
	f0.Set(s0)

	// fmt.Printf("1 %+#v\n", dv.Interface())
}

// These aren't test, rather snipped of who to use refection
func TestReflectConstruction02(t *testing.T) {
	_ = t

	x := struct {
		D test01.D
	}{}

	dT := reflect.TypeFor[test01.D_A]()
	dv := reflect.New(dT).Elem()
	dv.Field(0).SetInt(123)

	f0 := reflect.ValueOf(&x).Elem().Field(0).Field(0)
	f0.Set(dv)

	// fmt.Printf("1 %+#v\n", dv.Interface())
}

func TestTopLevelUnion01(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"a": 42}`)
	dec := goadl.NewDecoder(out, test01.Texpr_D(), goadl.RESOLVER)
	var y test01.D
	err := dec.Decode(&y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.Make_D_a(42)) {
		t.Fail()
	}
}

func TestTypeCast(t *testing.T) {
	d := &test01.D{}
	if _, ok := any(d).(goadl.BranchFactory); ok {
		// fmt.Printf("D implements BranchFactory")
	} else {
		t.Errorf("D doesn't implements BranchFactory")
	}
}

func TestTopLevelUnion02(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"b": {"a":42}}`)
	dec := goadl.NewDecoder(out, test01.Texpr_D(), goadl.RESOLVER)
	var y test01.D
	err := dec.Decode(&y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.Make_D_b(test01.B{A: 42})) {
		t.Fail()
	}
}

func TestUnionInStruct(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"d":{"b": {"a":42}}}`)
	dec := goadl.NewDecoder(out, test01.Texpr_E(), goadl.RESOLVER)
	var y test01.E
	err := dec.Decode(&y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.E{D: test01.Make_D_b(test01.B{A: 42})}) {
		t.Fail()
	}
}

func TestPrims(t *testing.T) {
	_ = t
	x := int64(99)
	_ = x
	p := test01.F{
		A: 1,
		B: 2,
		C: 3,
		D: 4,
		E: 5,
		F: 6,
		G: 7,
		H: 8,
		I: true,
		J: 1.1,
		K: 1.2,
		L: "abcd",
		// NOTE the default encoding of a number is a float64
		N: []any{nil, false, float64(1), map[string]any{"a": "a", "b": "sadf"}},
		O: []int64{1, 2, 3},
		P: map[string]int64{"a": 1, "b": 2},
		Q: &x,
		R: goadl.Void{},
	}
	buf := bytes.Buffer{}
	enc := goadl.NewEncoder(&buf, test01.Texpr_F(), goadl.RESOLVER)
	err := enc.Encode(p)
	if err != nil {
		t.Errorf("%v", err)
	}
	// fmt.Printf("%v\n", buf.String())
	dec := goadl.NewDecoder(&buf, test01.Texpr_F(), goadl.RESOLVER)
	pIn := test01.F{}
	err = dec.Decode(&pIn)
	if err != nil {
		t.Errorf("%v", err)
	}
	// fmt.Printf("%+v\n", pIn)
	if !reflect.DeepEqual(p, pIn) {
		t.Errorf(`out != in
pOut:%+#v
pIn :%+#v
`, p, pIn)
	}
	// fmt.Printf("out == in\npOut:%+v\npIn :%+v\n", p, pIn)

	buf2 := bytes.Buffer{}
	enc2 := goadl.NewEncoder(&buf2, test01.Texpr_F(), goadl.RESOLVER)
	err = enc2.Encode(p)
	if err != nil {
		t.Errorf("%v", err)
	}
	// fmt.Printf("%v\n", buf2.String())
}

func TestStructRecurse(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"a":[{"a":[]}]}`)
	dec := goadl.NewDecoder(out, test01.Texpr_G(), goadl.RESOLVER)
	var y test01.G
	err := dec.Decode(&y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	expect := test01.G{A: []test01.G{{A: []test01.G{}}}}
	if !reflect.DeepEqual(y, expect) {
		t.Errorf("expect != received\nexpect  :%+#v\nreceived:%+#v\n", expect, y)
	}

	buf := bytes.Buffer{}
	enc := goadl.NewEncoder(&buf, test01.Texpr_G(), goadl.RESOLVER)
	err = enc.Encode(y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if buf.String() != `{"a":[{"a":[]}]}` {
		t.Errorf("expect != received\nreceived%v", buf.String())
	}
}

func TestAdlAst(t *testing.T) {
	_ = t
	cj, err := os.Open("combined.json")
	if err != nil {
		t.Fatalf("can't read combined.json err:%v", err)
	}
	dec := goadl.NewDecoder(cj, goadl.Texpr_StringMap(goadl.Texpr_Module()), goadl.RESOLVER)
	var ast map[string]adlast.Module
	err = dec.Decode(&ast)
	if err != nil {
		t.Errorf("%v", err)
	}
	// fmt.Printf("%+v\n", ast)
	buf := bytes.Buffer{}
	enc := goadl.NewEncoder(&buf, goadl.Texpr_StringMap(goadl.Texpr_Module()), goadl.RESOLVER)
	err = enc.Encode(ast)
	if err != nil {
		t.Fatalf("err:%v", err)
	}

	cj.Seek(0, 0)
	d0 := json.NewDecoder(cj)
	d1 := json.NewDecoder(&buf)
	var a0, a1 any
	d0.Decode(&a0)
	d1.Decode(&a1)
	if reflect.DeepEqual(a0, a1) {
		t.Errorf("decode -> encode doesn't produce the same json")
	}

	// ibuf := bytes.Buffer{}
	// err = json.Indent(&ibuf, buf.Bytes(), ``, `    `)
	// if err != nil {
	// 	t.Fatalf("indent err:%v", err)
	// }
	// cj0, err := os.Create("combined_out.json")
	// if err != nil {
	// 	t.Fatalf("can't create combined_out.json err:%v", err)
	// }
	// cj0.Write(ibuf.Bytes())
	// cj0.Close()
}

func TestUnchecked(t *testing.T) {
	t.Run("struct-struct", func(t *testing.T) {
		var x = test01.New_Int(5)
		var y = test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := goadl.NewEncoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err := enc.Encode(x)
		if err != nil {
			t.Fatal(err)
		}

		dec := goadl.NewDecoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err = dec.Decode(&y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("&struct-struct", func(t *testing.T) {
		var x = test01.New_Int(5)
		var y = test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := goadl.NewEncoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err := enc.Encode(&x)
		if err != nil {
			t.Fatal(err)
		}

		dec := goadl.NewDecoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err = dec.Decode(&y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("struct-any", func(t *testing.T) {
		var x = test01.New_Int(5)
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := goadl.NewEncoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err := enc.Encode(x)
		if err != nil {
			t.Fatal(err)
		}

		dec := goadl.NewDecoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err = dec.Decode(y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("&struct-any", func(t *testing.T) {
		var x = test01.New_Int(5)
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := goadl.NewEncoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err := enc.Encode(&x)
		if err != nil {
			t.Fatal(err)
		}

		dec := goadl.NewDecoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err = dec.Decode(y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("any&-any", func(t *testing.T) {
		var z = test01.New_Int(5)
		var x any = &z
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := goadl.NewEncoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err := enc.Encode(x)
		if err != nil {
			t.Fatal(err)
		}

		dec := goadl.NewDecoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err = dec.Decode(y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("any-any", func(t *testing.T) {
		var z = test01.New_Int(5)
		var x any = z
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := goadl.NewEncoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err := enc.Encode(x)
		if err != nil {
			t.Fatal(err)
		}

		dec := goadl.NewDecoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err = dec.Decode(y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, reflect.ValueOf(y).Elem().Interface()) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("&any-any", func(t *testing.T) {
		var z = test01.New_Int(5)
		var x any = z
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := goadl.NewEncoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err := enc.Encode(&x)
		if err != nil {
			t.Fatal(err)
		}

		dec := goadl.NewDecoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err = dec.Decode(y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, reflect.ValueOf(y).Elem().Interface()) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("&any-&any", func(t *testing.T) {
		var z = test01.New_Int(5)
		var x any = &z
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := goadl.NewEncoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err := enc.Encode(&x)
		if err != nil {
			t.Fatal(err)
		}

		dec := goadl.NewDecoderUnchecked(&buf, texpr, goadl.RESOLVER)
		err = dec.Decode(&y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
}

func TestEncDec(t *testing.T) {
	str := "abc"
	testCases := []struct {
		desc  string
		texpr adlast.TypeExpr
		x     any
		y     any
	}{
		{
			desc:  "Int",
			texpr: test01.Texpr_Int().Value,
			x:     test01.New_Int(5),
			y:     &test01.Int{},
		},
		{
			desc:  "UInt",
			texpr: test01.Texpr_Uint().Value,
			x:     test01.New_Uint(5),
			y:     &test01.Uint{},
		},
		{
			desc:  "Bool",
			texpr: test01.Texpr_Bool().Value,
			x:     test01.New_Bool(true),
			y:     &test01.Bool{},
		},
		{
			desc:  "Unit",
			texpr: test01.Texpr_Unit().Value,
			x:     test01.New_Unit(),
			y:     &test01.Unit{},
		},
		{
			desc:  "NullableString",
			texpr: test01.Texpr_NullableString().Value,
			x:     test01.New_NullableString(&str),
			y:     &test01.NullableString{},
		},
		{
			desc:  "StringMapString",
			texpr: test01.Texpr_StringMapString().Value,
			x:     test01.New_StringMapString(map[string]string{"a": "sdf"}),
			y:     &test01.StringMapString{},
		},
		{
			desc:  "VectorString",
			texpr: test01.Texpr_VectorString().Value,
			x:     test01.New_VectorString([]string{"a", "sdf"}),
			y:     &test01.VectorString{},
		},
		{
			desc:  "Json01",
			texpr: test01.Texpr_Json().Value,
			x:     test01.New_Json(map[string]any{"a": "sdf"}),
			y:     &test01.Json{},
		},
		{
			desc:  "Json02",
			texpr: test01.Texpr_Json().Value,
			x:     test01.New_Json([]any{true, float64(1), float64(1.1), nil}),
			y:     &test01.Json{},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			buf := bytes.Buffer{}
			enc := goadl.NewEncoderUnchecked(&buf, tC.texpr, goadl.RESOLVER)
			err := enc.Encode(tC.x)
			if err != nil {
				t.Fatal(err)
			}

			dec := goadl.NewDecoderUnchecked(&buf, tC.texpr, goadl.RESOLVER)
			err = dec.Decode(tC.y)
			if err != nil {
				t.Fatal(err)
			}
			x := tC.x
			y := reflect.ValueOf(tC.y).Elem().Interface()
			if !reflect.DeepEqual(x, y) {
				t.Errorf("not equal\n%#+v\n%#+v", x, y)
			}
		})
	}
}

func TestSetTest(t *testing.T) {
	st := test01.New_SetTest(
		types.MapSet[string]{
			"a": {},
			"b": {},
			"c": {},
		},
	)
	for i := 0; i < 8; i++ {
		buf := bytes.Buffer{}
		enc := goadl.NewEncoder(&buf, test01.Texpr_SetTest(), goadl.RESOLVER)
		// enc := goadl.NewEncoder(&buf, goadl.Texpr_Set(goadl.Texpr_String()), goadl.RESOLVER)
		enc.Encode(st)
		fmt.Printf("%s\n", buf.String())
	}
	buf := bytes.Buffer{}
	enc := goadl.NewEncoder(&buf, test01.Texpr_SetTest(), goadl.RESOLVER)
	// enc := goadl.NewEncoder(&buf, goadl.Texpr_Set(goadl.Texpr_String()), goadl.RESOLVER)
	enc.Encode(st)
	dec := goadl.NewDecoder(&buf, test01.Texpr_SetTest(), goadl.RESOLVER)
	st2 := test01.SetTest{}
	err := dec.Decode(&st2)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("%+#v\n", st2)
}
