package gen_go_v2

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

func (in *goadlcV2Cmd) modpath() (modulePath, midPath string, errout error) {
	if in.ModulePath != "" {
		modulePath = in.ModulePath
	} else {
		if in.GoModFile == "" {
			dir := in.Outputdir
			goMod := filepath.Join(dir, "go.mod")
			last := false
			if in.Outputdir == "" {
				last = true
			}
			for {
				if in.Debug {
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
				midPath = filepath.Join(midPath, file)
			}
			in.GoModFile = goMod
			if in.Debug {
				fmt.Fprintf(os.Stderr, "looking for module-path in go.mod file. go.mod:%s\n", in.GoModFile)
			}
		}
		if gms, err := os.Stat(in.GoModFile); err == nil {
			if !gms.IsDir() {
				if modbufm, err := os.ReadFile(in.GoModFile); err == nil {
					modulePath = modfile.ModulePath(modbufm)
					if in.Debug {
						fmt.Fprintf(os.Stderr, "using module-path found in go.mod file. module-path:%s\n", modulePath)
					}
				} else {
					return "", "", fmt.Errorf("module-path needed. Not specified in --module-path and couldn't be found in a go.mod file")
				}
			}
		} else {
			return "", "", fmt.Errorf("module-path required. Not specified in --module-path and no go.mod file found in output directory")
		}
	}
	return
}
