package gotypes

import "github.com/adl-lang/goadl_rt/v3/sys/adlast"

type snResolver func(sn adlast.ScopedName) (*adlast.Decl, bool)

type baseGen struct {
	cli        *GoTypes
	resolver   snResolver
	modulePath string
	midPath    string
	moduleName string
	imports    imports
}
