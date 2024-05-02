package gen_go

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

func makeAdlcAstArgs(in GoAdlcCmd, pathSuffix string) []string {
	args := []string{
		"ast", "--outputdir", in.workingDir() + pathSuffix,
		"--manifest", in.workingDir() + pathSuffix + "/.manifest",
	}
	if in.debug() {
		args = append(args, "--verbose")
	}
	if in.mergeAdlext() != "" {
		args = append(args, "--merge-adlext", in.mergeAdlext())
	}
	for _, dir := range in.searchdir() {
		args = append(args, "--searchdir", dir)
	}
	args = append(args, in.files()...)
	if in.debug() {
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

type GoAdlcCmd interface {
	workingDir() string
	mergeAdlext() string
	debug() bool
	searchdir() []string
	files() []string
}

func loadAdl[M any, D any](
	in GoAdlcCmd,
	modules *[]moduleTuple[M],
	jb func(r io.Reader) (moduleMap[M], moduleMap[D], error),
) (moduleMap[M], moduleMap[D], error) {
	if in.workingDir() != "" {
		os.Remove(in.workingDir())
		if _, errO := os.Open(in.workingDir()); errO != nil {
			err := os.Mkdir(in.workingDir(), os.ModePerm)
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
	filepath.WalkDir(in.workingDir()+"/individual", func(path string, de fs.DirEntry, err error) error {
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
	if in.debug() {
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
	fd, err := os.Open(in.workingDir() + "/combined/combined.json")
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
		if in.debug() {
			fmt.Fprintf(os.Stderr, "combined %s", k)
		}
		if idx, has := moduleSet[k]; has {
			if in.debug() {
				fmt.Fprintf(os.Stderr, " ✅")
			}
			m := combinedAst[k]
			(*modules)[idx].module = &m
		}
		if in.debug() {
			fmt.Fprintf(os.Stderr, "\n")
		}
	}
	return combinedAst, declMap, nil
}
