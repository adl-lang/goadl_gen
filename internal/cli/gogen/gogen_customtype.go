package gogen

import (
	"fmt"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/cli/goimports"
)

var GoCustomTypeSN = adlast.Make_ScopedName(
	"adlc.config.go_",
	"GoCustomType",
)

func (in *Generator) GoRegisterHelper(moduleName string, decl adlast.Decl) (string, error) {
	jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_GoCustomType(), goadl.RESOLVER)
	gct, err := goadl.GetAnnotation(decl.Annotations, GoCustomTypeSN, jb)
	if err != nil {
		return "", err
	}
	if gct == nil {
		return "", nil
	}
	if in.Cli.IsStdLibGen() && gct.Helpers.Import_path == in.Cli.GoAdlImportPath() {
		return fmt.Sprintf(`	RESOLVER.RegisterHelper(
			adlast.Make_ScopedName("%s", "%s"),
			(*%s)(nil),
		)
`, moduleName, decl.Name, gct.Helpers.Name), nil
	}
	pkg := gct.Helpers.Import_path[strings.LastIndex(gct.Helpers.Import_path, "/")+1:]
	spec := goimports.ImportSpec{
		Path:    gct.Helpers.Import_path,
		Name:    gct.Helpers.Pkg,
		Aliased: gct.Helpers.Pkg != pkg,
	}
	in.Imports.AddSpec(spec)
	return fmt.Sprintf(`	RESOLVER.RegisterHelper(
			adlast.Make_ScopedName("%s", "%s"),
			(*%s%s)(nil),
		)
`, moduleName, decl.Name, gct.Helpers.Pkg+".", gct.Helpers.Name), nil
}
