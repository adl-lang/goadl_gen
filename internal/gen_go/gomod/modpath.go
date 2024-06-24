package gomod

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adl-lang/goadlc/internal/root"
	"golang.org/x/mod/modfile"
)

func NewGoCmd(rt *root.RootObj) *GoCmd {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, `WARNING: error getting current working directory %v\n`, err)
	}
	return &GoCmd{
		rt:        rt,
		Outputdir: cwd,
	}
}

type GoCmd struct {
	rt         *root.RootObj `json:"-"`
	ModulePath string        `opts:"group=go" help:"The path of the Go module for the generated code. Overrides the module-path from the '--go-mod-file' flag."`
	GoModFile  string        `opts:"group=go" help:"Path of a go.mod file. If the file exists, the module-path is used for generated imports."`
	Outputdir  string        `opts:"group=go,short=O" help:"Set the directory where generated code is written "`
}

type GoModResult struct {
	ModulePath string
	MidPath    string
}

func (in *GoCmd) Modpath() (*GoModResult, error) {
	if in.ModulePath != "" {
		res := &GoModResult{}
		res.ModulePath = in.ModulePath
		return res, nil
	}
	res := &GoModResult{}
	if in.GoModFile == "" {
		dir := in.Outputdir
		goMod := filepath.Join(dir, "go.mod")
		last := false
		if in.Outputdir == "" {
			last = true
		}
		for {
			if in.rt.Debug {
				fmt.Fprintf(os.Stderr, "searching for module-path in go.mod file. go.mod:%s\n", goMod)
			}
			if gms, err := os.Stat(goMod); err == nil && !gms.IsDir() {
				in.GoModFile = goMod
				break
			}
			dir0, file := filepath.Split(dir)
			dir = dir0
			if last {
				break
			}
			if dir == "" {
				last = true
			}
			goMod = filepath.Join(dir, "go.mod")
			res.MidPath = filepath.Join(res.MidPath, file)
		}
		in.GoModFile = goMod
		if in.rt.Debug {
			fmt.Fprintf(os.Stderr, "looking for module-path in go.mod file. go.mod:%s\n", in.GoModFile)
		}
	}
	if gms, err := os.Stat(in.GoModFile); err == nil {
		if !gms.IsDir() {
			if modbufm, err := os.ReadFile(in.GoModFile); err == nil {
				res.ModulePath = modfile.ModulePath(modbufm)
				if in.rt.Debug {
					fmt.Fprintf(os.Stderr, "using module-path found in go.mod file. module-path:%s\n", res.ModulePath)
				}
			} else {
				return nil, fmt.Errorf("module-path needed. Not specified in --module-path and couldn't be found in a go.mod file")
			}
		}
	} else {
		return nil, fmt.Errorf("module-path required. Not specified in --module-path and no go.mod file found in output directory")
	}
	return res, nil
}
