package load

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	goadl "github.com/adl-lang/goadl_rt/v3"
	"github.com/adl-lang/goadl_rt/v3/sys/adlast"
	"github.com/adl-lang/goadlc/internal/root"
	"github.com/mattn/go-zglob"
)

func NewLoadCmd(rt *root.RootObj) *LoadTask {
	wk, err := os.MkdirTemp("", "goadlc-")
	if err != nil {
		fmt.Fprintf(os.Stderr, `WARNING: os.MkdirTemp("", "goadlc-") %v\n`, err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, `WARNING: error getting UserCacheDir %v\n`, err)
	}
	return &LoadTask{
		rt:           rt,
		BundleMaps:   []BundleMap{},
		WorkingDir:   wk,
		MergeAdlext:  "adl-go",
		UserCacheDir: filepath.Join(cacheDir, "adl-bundles"),
	}
}

type LoadTask struct {
	rt *root.RootObj `json:"-"`
	//load
	BundleMaps BundleMaps `opts:"group=load" help:"Mapping from ADL bundle to go module. Of the form [module_prefix:go_module_path]"`
	Searchdir  []string   `opts:"group=load,short=I" help:"Add the specifed directory to the ADL searchpath"`
	Files      []string   `opts:"group=load" help:"File or pattern"`
	files      []string

	//makeAdlcAstArgs
	WorkingDir  string `opts:"group=load" help:"The temp directory used to place intermediate files."`
	MergeAdlext string `opts:"group=load" help:"Add the specifed adl file extension to merged on loading"`

	//zippedBundle
	UserCacheDir string `opts:"group=load" help:"The directory used to place cached files (e.g. download adl source)."`

	// //modpath
	// ModulePath string `help:"The path of the Go module for the generated code. Overrides the module-path from the '--go-mod-file' flag."`
	// GoModFile  string `help:"Path of a go.mod file. If the file exists, the module-path is used for generated imports."`
	// Outputdir  string `opts:"short=O" help:"Set the directory where generated code is written "`
}

type LoadResult struct {
	CombinedAst map[string]adlast.Module
	// ModulePath  string
	// MidPath     string
	Modules []NamedModule
}

func (in *LoadTask) Load() (*LoadResult, error) {
	for _, bm := range in.BundleMaps {
		if strings.HasPrefix(bm.AdlSrc, "file://") {
			in.Searchdir = append(in.Searchdir, bm.AdlSrc[len("file://"):])
		}
		if strings.HasPrefix(bm.AdlSrc, "https://") && strings.HasSuffix(bm.AdlSrc, ".zip") {
			path, err := in.zippedBundle(bm)
			if err != nil {
				return nil, err
			}
			if bm.Path != nil {
				in.Searchdir = append(in.Searchdir, filepath.Join(path, *bm.Path))
			} else {
				in.Searchdir = append(in.Searchdir, path)
			}
		}
	}

	if len(in.Files) == 0 {
		return nil, fmt.Errorf("no file or pattern specified")
	}
	for _, p := range in.Files {
		matchs, err := zglob.Glob(p)
		sort.Strings(matchs)
		if err != nil {
			return nil, fmt.Errorf("error globbing file : %v", err)
		}
		in.files = append(in.files, matchs...)
	}
	if len(in.files) == 0 {
		return nil, fmt.Errorf("no files found")
	}
	if in.rt.Debug {
		fmt.Fprintf(os.Stderr, "found files:\n")
		for _, f := range in.files {
			fmt.Fprintf(os.Stderr, "  %v\n", f)
		}
	}

	results := &LoadResult{}

	results.Modules = []NamedModule{}
	var err error
	results.CombinedAst, results.Modules, err = loadAdl(in)
	if err != nil {
		return nil, err
	}

	// results.ModulePath, results.MidPath, err = in.modpath()
	// if err != nil {
	// 	return nil, err
	// }
	return results, nil
}

func loadAdl(
	in *LoadTask,
) (map[string]adlast.Module, []NamedModule, error) {
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
	modules := []NamedModule{}

	idx := 0
	filepath.WalkDir(in.WorkingDir+"/individual", func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("error in walkfunc %v", err)
			return err
		}
		if !de.IsDir() && strings.HasSuffix(path, ".json") {
			name := path[strings.LastIndex(path, "/")+1 : strings.LastIndex(path, ".")]
			moduleSet[name] = idx
			modules = append(modules, NamedModule{name, nil})
			idx++
		}
		return nil
	})
	if in.rt.Debug {
		for _, m := range modules {
			fmt.Fprintf(os.Stderr, "'%s'\n", m.Name)
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

	combinedAst := make(map[string]adlast.Module)
	dec := goadl.CreateJsonDecodeBinding(adlast.Texpr_StringMap[adlast.Module](goadl.Texpr_Module()), goadl.RESOLVER)
	err = dec.Decode(fd, &combinedAst)
	if err != nil {
		return nil, nil, err
	}
	for k := range combinedAst {
		if in.rt.Debug {
			fmt.Fprintf(os.Stderr, "combined %s", k)
		}
		if idx, has := moduleSet[k]; has {
			if in.rt.Debug {
				fmt.Fprintf(os.Stderr, " ✅")
			}
			m := combinedAst[k]
			modules[idx].Module = &m
		}
		if in.rt.Debug {
			fmt.Fprintf(os.Stderr, "\n")
		}
	}
	return combinedAst, modules, nil
}

func makeAdlcAstArgs(in *LoadTask, pathSuffix string) []string {
	args := []string{
		"ast", "--outputdir", in.WorkingDir + pathSuffix,
		"--manifest", in.WorkingDir + pathSuffix + "/.manifest",
	}
	if in.rt.Debug {
		args = append(args, "--verbose")
	}
	if in.MergeAdlext != "" {
		args = append(args, "--merge-adlext", in.MergeAdlext)
	}
	for _, dir := range in.Searchdir {
		args = append(args, "--searchdir", dir)
	}
	args = append(args, in.files...)
	if in.rt.Debug {
		fmt.Fprintf(os.Stderr, "cmd '%s'\n", strings.Join(args, " "))
	}
	return args
}

func (in *LoadTask) zippedBundle(bm BundleMap) (string, error) {
	path := bm.AdlSrc[len("https://"):strings.LastIndex(bm.AdlSrc, "/")]
	file := bm.AdlSrc[strings.LastIndex(bm.AdlSrc, "/"):]
	zipdir := filepath.Join(in.UserCacheDir, "download", path)
	zipfile := filepath.Join(zipdir, file)
	if _, err := os.Stat(zipfile); err != nil {
		if err := os.MkdirAll(zipdir, 0777); err != nil {
			return "", fmt.Errorf("error creating dir for zip adlsrc '%s' err: %w", zipdir, err)
		}
		if in.rt.Debug {
			fmt.Fprintf(os.Stderr, "created zip download dir '%s'\n", zipdir)
		}
		file, err := os.Create(zipfile)
		if err != nil {
			return "", err
		}
		defer file.Close()
		resp, err := http.Get(bm.AdlSrc)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("bad status: %s", resp.Status)
		}
		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return "", err
		}
		zarch, err := zip.OpenReader(zipfile)
		if err != nil {
			return "", err
		}
		if in.rt.Debug {
			fmt.Printf("Unzipping %v\n", zipfile)
		}

		cachePath := bm.AdlSrc[len("https://") : len(bm.AdlSrc)-len(".zip")]
		cacheDir := filepath.Join(in.UserCacheDir, "cache", cachePath)
		if err := os.MkdirAll(cacheDir, 0777); err != nil {
			return "", fmt.Errorf("error creating dir for cache adlsrc '%s' err: %w", cacheDir, err)
		}
		for _, zf := range zarch.File {
			if zf.FileInfo().IsDir() {
				continue
			}
			name := zf.Name[strings.Index(zf.Name, "/")+1:]
			if in.rt.Debug {
				fmt.Printf("  %v\n", name)
			}
			dst := filepath.Join(cacheDir, name)
			if err := os.MkdirAll(filepath.Dir(dst), 0777); err != nil {
				return "", err
			}
			w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0444)
			if err != nil {
				return "", err
			}
			r, err := zf.Open()
			if err != nil {
				w.Close()
				return "", err
			}
			lr := &io.LimitedReader{R: r, N: int64(zf.UncompressedSize64) + 1}
			_, err = io.Copy(w, lr)
			r.Close()
			if err != nil {
				w.Close()
				return "", err
			}
			if err := w.Close(); err != nil {
				return "", err
			}
			if lr.N <= 0 {
				return "", fmt.Errorf("uncompressed size of file %s is larger than declared size (%d bytes)", zf.Name, zf.UncompressedSize64)
			}
		}
		return cacheDir, nil
	}

	if in.rt.Debug {
		fmt.Fprintf(os.Stderr, "cached zip  '%s'\n", zipfile)
	}
	cachePath := bm.AdlSrc[len("https://") : len(bm.AdlSrc)-len(".zip")]
	cacheDir := filepath.Join(in.UserCacheDir, "cache", cachePath)
	return cacheDir, nil
}
