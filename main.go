package main

import (
	"fmt"
	"os"

	"github.com/adl-lang/goadlc/internal/gen_go"
	gen_go_v2 "github.com/adl-lang/goadlc/internal/gen_go/v2"
	"github.com/adl-lang/goadlc/internal/root"
	"github.com/jpillora/opts"
)

func main() {
	rflg := &root.RootObj{}
	op := opts.New(rflg).
		Name("goadlc").
		EmbedGlobalFlagSet().
		AddCommand(opts.New(&struct{}{}).Name("go").
			AddCommand(gen_go.NewGenGo().Name("v1")).
			AddCommand(gen_go_v2.NewGenGoV2().Name("v2")).
			AddCommand(gen_go_v2.NewGenTypeExprV2().Name("v2_gen_texpr"))).
		Complete().
		AddCommand(opts.New(&versionCmd{}).Name("version")).
		Parse()
	if !op.IsRunnable() {
		fmt.Fprintf(os.Stderr, "%s", op.Help())
		os.Exit(1)
	}
	err := op.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		fmt.Fprintf(os.Stderr, "%s", op.Selected().Help())
		os.Exit(2)
	}
}

// Set by build tool chain by
// go build --ldflags '-X main.Version=xxx -X main.Date=xxx -X main.Commit=xxx'
var (
	Version string = "dev"
	Date    string = "na"
	Commit  string = "na"
)

type versionCmd struct{}

func (r *versionCmd) Run() {
	fmt.Printf("version: %s\ndate: %s\ncommit: %s\n", Version, Date, Commit)
}
