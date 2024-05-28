package gen_go

import (
	"fmt"
	"os"
	"testing"

	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/root"
)

var (
	combinedAst map[string]adlast.Module
	importMap   map[string]importSpec
	modulePath  string
	midPath     string
	modules     []namedModule
	setupErr    error

	cmd *goadlcCmd
)

func init() {
	err := os.Chdir("../../..")
	cwd, _ := os.Getwd()
	fmt.Printf("cwd : %v\n", cwd)
	if err != nil {
		panic(err)
	}
	rt := &root.RootObj{
		Cfg: "adlast.cfg.json",
	}
	cmd = NewGenGoV3(rt).(*goadlcCmd)
	combinedAst, importMap, modulePath, midPath, modules, setupErr = cmd.setup()
	cmd.Debug = false
}

func BenchmarkGen(t *testing.B) {
	t.ResetTimer()
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	for i := 0; i < t.N; i++ {
		cmd.generate(combinedAst, importMap, modulePath, midPath, modules, setupErr)
	}
}
