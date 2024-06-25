// Code generated by goadlc v3 - DO NOT EDIT.
package goapi

import (
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/cli/gomod"
	"github.com/adl-lang/goadlc/internal/cli/loader"
	"github.com/adl-lang/goadlc/internal/cli/root"
)

type GoApi struct {
	_GoApi
}

type _GoApi struct {
	Root      *root.Root         `json:"-"`
	Loader    *loader.LoadResult `json:"-"`
	GoMod     *gomod.GoModResult `json:"-"`
	ApiStruct adlast.ScopedName  `json:"ApiStruct"`
}

func MakeAll_GoApi(
	root *root.Root,
	loader *loader.LoadResult,
	gomod *gomod.GoModResult,
	apistruct adlast.ScopedName,
) GoApi {
	return GoApi{
		_GoApi{
			Root:      root,
			Loader:    loader,
			GoMod:     gomod,
			ApiStruct: apistruct,
		},
	}
}

func Make_GoApi(
	apistruct adlast.ScopedName,
) GoApi {
	ret := GoApi{
		_GoApi{
			Root:      ((*GoApi)(nil)).Default_root(),
			Loader:    ((*GoApi)(nil)).Default_loader(),
			GoMod:     ((*GoApi)(nil)).Default_goMod(),
			ApiStruct: apistruct,
		},
	}
	return ret
}

func (*GoApi) Default_root() *root.Root {
	return nil
}
func (*GoApi) Default_loader() *loader.LoadResult {
	return nil
}
func (*GoApi) Default_goMod() *gomod.GoModResult {
	return nil
}