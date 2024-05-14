package gen_go_v2

import (
	"os"
	"text/template"

	"github.com/jpillora/opts"
)

func NewGenTypeExprV2() opts.Opts {
	return opts.New(&texprV2Cmd{})
}

type texprV2Cmd struct {
}

var tmpl = `
func Texpr_{{.ADL}}() ATypeExpr[{{.Go}}] {
	return ATypeExpr[{{.Go}}]{
		Value: TypeExpr{
			TypeRef: TypeRef{
				Branch: TypeRefBranch_Primitive("{{.ADL}}"),
			},
			Parameters: []TypeExpr{},
		},
	}
}
`

var texprData = []struct {
	ADL string
	Go  string
}{
	{"Int8", "int8"},
	{"Int16", "int16"},
	{"Int32", "int32"},
	{"Int64", "int64"},
	{"Word8", "uint8"},
	{"Word16", "uint16"},
	{"Word32", "uint32"},
	{"Word64", "uint64"},
	{"Bool", "bool"},
	{"Float", "float64"},
	{"Double", "float64"},
	{"String", "string"},
	// {"ByteVector", "[]byte"},
	{"Void", "struct{}"},
	{"Json", "any"},
}

func (in *texprV2Cmd) Run() error {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return err
	}
	for _, te := range texprData {
		t.Execute(os.Stdout, te)
	}
	return nil
}
