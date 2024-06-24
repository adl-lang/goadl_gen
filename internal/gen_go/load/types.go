package load

import (
	"fmt"
	"strings"

	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
)

type ModuleCodeGen struct {
	Directory []string
	File      []DeclCodeGen
}
type DeclCodeGen struct {
	Package     string
	ImportAlias map[string]string
	Imports     []string
	Decl        adlast.Decl
}

type NamedModule struct {
	Name   string
	Module *adlast.Module
}

type NamedDecl struct {
	Name string
	Decl *adlast.Decl
}

type BundleMaps []BundleMap

type BundleMap struct {
	AdlModuleNamePrefix string
	GoModPath           string
	AdlSrc              string
	GoModVersion        *string
	Path                *string
}

func (ims *BundleMaps) Set(text string) error {
	panic("method only here to make opts happy")
}

func (im *BundleMap) Set(text string) error {
	parts := strings.Split(text, `|`)
	lp := len(parts)
	if lp < 2 || lp > 4 {
		return fmt.Errorf("expecting bundle to go map of the form [module_prefix|go_module_path|adl_src]")
	}
	im.AdlModuleNamePrefix = parts[0]
	im.GoModPath = parts[1]
	if lp >= 3 {
		im.GoModVersion = &parts[2]
	}
	return nil
}
