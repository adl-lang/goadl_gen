package main

import (
	"fmt"
	"os"

	"github.com/adl-lang/goadlc/internal/gen_go"
	"github.com/adl-lang/goadlc/internal/root"
	"github.com/jpillora/opts"
)

func main() {
	rflg := &root.RootObj{}
	op := opts.New(rflg).
		Name("goadlc").
		EmbedGlobalFlagSet().
		AddCommand(gen_go.NewGoadlc().Name("go")).
		Complete().
		AddCommand(opts.New(&versionCmd{}).Name("version")).
		Parse()
	if !op.IsRunnable() {
		fmt.Fprintf(os.Stderr, "%s", op.Help())
		os.Exit(1)
	}
	op.RunFatal()
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
