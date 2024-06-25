package goapi

import (
	"fmt"
)

func (in *GoApi) Run() error {
	mod, ok := in.Loader.CombinedAst[in.ApiStruct.ModuleName]
	if !ok {
		return fmt.Errorf("Module not found '%s", in.ApiStruct.ModuleName)
	}
	decl, ok := mod.Decls[in.ApiStruct.Name]
	if !ok {
		return fmt.Errorf("Decl not found '%s", in.ApiStruct.Name)
	}
	st, ok := decl.Type_.Cast_struct_()
	if !ok || len(st.TypeParams) != 0 {
		return fmt.Errorf("Unexpected - apiRequests is not a monomorphic struct")
	}

	// adlast.HandleWithErr_DeclType[*adlast.Struct](
	// 	decl,
	// 	func(struct_ adlast.Struct) (*adlast.Struct, error) {
	// 		return &struct_, nil
	// 	},
	// 	func(union_ adlast.Union) (*adlast.Struct, error) {
	// 		return nil, fmt.Errorf("must be a struct")
	// 	},
	// 	func(type_ adlast.TypeDef) (*adlast.Struct, error) {

	// 	},
	// 	func(newtype_ adlast.NewType) (*adlast.Struct, error) {

	// 	},
	// 	func() (adlast.Struct, error) {
	// 		panic("shouldn't get here")
	// 	},
	// )
	return nil
}
