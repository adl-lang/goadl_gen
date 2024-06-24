package main

import (
	"fmt"
	"os"

	"github.com/adl-lang/goadlc/internal/gen_go/gomod"
	"github.com/adl-lang/goadlc/internal/gen_go/load"
	gen_go_v3 "github.com/adl-lang/goadlc/internal/gen_go/v3"
	"github.com/adl-lang/goadlc/internal/root"
	"github.com/jpillora/opts"
)

func main() {
	rflg := &root.RootObj{}
	loadFlg := load.NewLoadCmd(rflg)
	goFlg := gomod.NewGoCmd(rflg)
	gorepFlg := gen_go_v3.NewGenGoV3(rflg, goFlg, loadFlg)

	loadCmd := opts.New(loadFlg).Name("load")
	goCmd := opts.New(goFlg).Name("go")
	typeCmd := opts.New(gorepFlg).Name("types")

	cliBldr := opts.New(rflg).
		Name("goadlc").
		SetLineWidth(200).
		EmbedGlobalFlagSet().
		Complete()

	cliBldr.AddCommand(loadCmd)
	loadCmd.AddCommand(goCmd)
	goCmd.AddCommand(typeCmd)

	cliBldr.AddCommand(opts.New(gen_go_v3.NewGenTypeExprV3()).Name("v3_gen_texpr"))
	cliBldr.AddCommand(opts.New(&versionCmd{}).Name("version"))

	op, err := cliBldr.ParseArgsError(os.Args)
	if err != nil {
		// fmt.Fprintf(os.Stderr, "%s", op.Selected().Help())
		// fmt.Fprintf(os.Stderr, "  Error:\n    %s\n", err)
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
	if !op.IsRunnable() {
		fmt.Fprintf(os.Stderr, "Sub/Command not runnable\n%s", op.Selected().Help())
		os.Exit(2)
	}
	err = op.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", op.Selected().Help())
		fmt.Fprintf(os.Stderr, "  Error:\n    %s\n", err)
		os.Exit(3)
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
