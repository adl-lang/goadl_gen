package gen_go_v2

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v2"
	"github.com/golang/glog"
)

type ModuleCodeGen struct {
	Directory []string
	File      []DeclCodeGen
}
type DeclCodeGen struct {
	Package     string
	ImportAlias map[string]string
	Imports     []string
	Decl        goadl.Decl
}

func makeAdlcAstArgs(in *goadlcV2Cmd, pathSuffix string) []string {
	args := []string{
		"ast", "--outputdir", in.WorkingDir + pathSuffix,
		"--manifest", in.WorkingDir + pathSuffix + "/.manifest",
	}
	if in.Debug {
		args = append(args, "--verbose")
	}
	if in.MergeAdlext != "" {
		args = append(args, "--merge-adlext", in.MergeAdlext)
	}
	for _, dir := range in.Searchdir {
		args = append(args, "--searchdir", dir)
	}
	args = append(args, in.files...)
	if in.Debug {
		fmt.Fprintf(os.Stderr, "cmd '%s'\n", strings.Join(args, " "))
	}
	return args
}

// / A JsonBinding is a de/serializer for a give ADL type
type JsonBinding[T any] interface {
	// Returns JSON encoding of ADL value t.
	Marshal(t T) ([]byte, error)
	// Parses the JSON-encoded data to an ADL type t, returning the result.
	Unmarshal([]byte) (T, error)
}

type moduleTuple[M any] struct {
	name   string
	module *M
}

type moduleMap[M any] map[string]M

func loadAdl[M any, D any](
	in *goadlcV2Cmd,
	modules *[]moduleTuple[M],
	jb func(r io.Reader) (moduleMap[M], moduleMap[D], error),
) (moduleMap[M], moduleMap[D], error) {
	if in.WorkingDir != "" {
		os.Remove(in.WorkingDir)
		if _, errO := os.Open(in.WorkingDir); errO != nil {
			err := os.Mkdir(in.WorkingDir, os.ModePerm)
			if err != nil {
				log.Fatalf(`os.Mkdir %v`, err)
			}
		}
	}
	args1 := makeAdlcAstArgs(in, "/individual")
	cmd1 := exec.Command("adlc", args1...)
	out1, err := cmd1.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error calling adlc to generate individual ast files. err : %v\n", err)
		fmt.Fprintf(os.Stderr, "  output '%s'\n", string(out1))
		return nil, nil, err
	}
	moduleSet := make(map[string]int)
	idx := 0
	filepath.WalkDir(in.WorkingDir+"/individual", func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("error in walkfunc %v", err)
			return err
		}
		if !de.IsDir() && strings.HasSuffix(path, ".json") {
			name := path[strings.LastIndex(path, "/")+1 : strings.LastIndex(path, ".")]
			moduleSet[name] = idx
			*modules = append(*modules, moduleTuple[M]{name, nil})
			idx++
		}
		return nil
	})
	if in.Debug {
		for _, m := range *modules {
			fmt.Fprintf(os.Stderr, "'%s'\n", m.name)
		}
	}
	args2 := makeAdlcAstArgs(in, "/combined")
	args2 = append(args2, "--combined-output=combined.json")
	cmd2 := exec.Command("adlc", args2...)
	out2, err := cmd2.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error calling adlc to generate combined ast file. err : %v\n", err)
		fmt.Fprintf(os.Stderr, "  output '%s'\n", string(out2))
		return nil, nil, err
	}
	fd, err := os.Open(in.WorkingDir + "/combined/combined.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error open combined ast file '%v'\n", err)
		return nil, nil, err
	}
	combinedAst, declMap, err := jb(fd)
	// dec := json.NewDecoder(fd)
	// err = dec.Decode(&combinedAst)
	if err != nil {
		glog.Errorf("decoding ast error %v\n", err)
		return nil, nil, err
	}
	for k := range combinedAst {
		if in.Debug {
			fmt.Fprintf(os.Stderr, "combined %s", k)
		}
		if idx, has := moduleSet[k]; has {
			if in.Debug {
				fmt.Fprintf(os.Stderr, " ✅")
			}
			m := combinedAst[k]
			(*modules)[idx].module = &m
		}
		if in.Debug {
			fmt.Fprintf(os.Stderr, "\n")
		}
	}
	return combinedAst, declMap, nil
}
