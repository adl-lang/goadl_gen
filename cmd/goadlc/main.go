package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"text/template"

	goadl "github.com/adl-lang/goadl_rt/v3"

	"github.com/adl-lang/goadlc/internal/cli/gengo"
	"github.com/adl-lang/goadlc/internal/cli/goapi"
	"github.com/adl-lang/goadlc/internal/cli/gomod"
	"github.com/adl-lang/goadlc/internal/cli/gotypes"
	"github.com/adl-lang/goadlc/internal/cli/loader"
	"github.com/adl-lang/goadlc/internal/cli/root"
)

func main() {
	rt := goadl.Addr(root.Make_Root())
	flag.BoolVar(&rt.Debug, "debug", false, "Print extra diagnostic information, especially about files being read/written")
	flag.BoolVar(&rt.DumpConfig, "dump-config", false, "Dump the config to stdout and exits")
	flag.StringVar(&rt.Cfg, "cfg", "", "Config file in json format")
	flag.Parse()
	if rt.Cfg == "" {
		flag.Usage()
		os.Exit(1)
	}
	ld := &loader.Loader{}
	gm := &gomod.GoModule{}
	gg := &gengo.GenGo{}
	gt := &gotypes.GoTypes{}

	ld.Root = rt

	gt.Root = rt

	gg.Loader = ld
	gg.GoTypes = gt
	gg.Root = rt
	gg.Mod = gm

	if err := root.ReadConfig(*rt, gengo.Texpr_GenGo(), gg); err != nil {
		fmt.Fprintf(os.Stderr, "Error read cfg : %v\n", err)
		os.Exit(1)
	}
	// gg.Loader.Root = &rt
	// gg.GoTypes.Loader = gg.Loader
	// gg.GoTypes.Root = &rt
	// gg.GoTypes.GoMod = gg.Mod

	tmplVar(&gg.Loader.WorkingDir)
	tmplVar(&gg.Loader.UserCacheDir)
	if rt.DumpConfig {
		err := root.DumpConfig(*rt, gengo.Texpr_GenGo(), *gg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error dummping cfg : %v\n", err)
			os.Exit(1)
		}
		return
	}
	err := gg.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}
}

func tmplVar(val *string) {
	if tmpl, err := template.New("").Parse(*val); err != nil {
		fmt.Fprintf(os.Stderr, "tmpl parse err : %s\n %v\n", *val, err)
		os.Exit(1)
	} else {
		buf := bytes.Buffer{}
		err = tmpl.Execute(&buf, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tmpl exec err : %v\n", err)
			os.Exit(1)
		}
		*val = buf.String()
	}
}

type Data struct {
	OS *OS
}

type OS struct {
}

var data Data = Data{}

func (*OS) MkdirTemp(dir string, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

func (*OS) UserCacheDir() (string, error) {
	return os.UserCacheDir()
}

func dump_exmaple() {
	// rt := cli.Make_Root()
	// wk, err := os.MkdirTemp("", "goadlc-")
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, `WARNING: os.MkdirTemp("", "goadlc-") %v\n`, err)
	// }
	// cacheDir, err := os.UserCacheDir()
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, `WARNING: error getting UserCacheDir %v\n`, err)
	// }

	ld := loader.Make_Loader(
		[]string{"abc"},
	)
	gen := gengo.Make_GenGo(
		&ld,
		goadl.Addr(gomod.Make_GoModule_Outputdir(".")),
		goadl.Addr(gotypes.Make_GoTypes(
			// &rt,
			// &ld,
			".",
		)),
	)
	gen.GoApis = &[]goapi.GoApi{{}}

	je := goadl.CreateJsonEncodeBinding(gengo.Texpr_GenGo(), goadl.RESOLVER)
	buf := bytes.Buffer{}
	err := je.Encode(&buf, gen)
	if err != nil {
		log.Fatalf("%v", err)
	}
	out := bytes.Buffer{}
	json.Indent(&out, buf.Bytes(), "", "  ")
	fmt.Printf("%s\n", out.String())
}
