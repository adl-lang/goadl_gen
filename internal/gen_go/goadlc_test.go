package gen_go

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/adl-lang/goadlc/internal/diff"
	"golang.org/x/tools/txtar"
)

func Test(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.txt")
	if len(files) == 0 {
		t.Fatalf("no testdata")
	}

	wk, err := os.MkdirTemp("", "goadlc-")
	if err != nil {
		t.Fatalf(`os.MkdirTemp("", "goadlc-") %v`, err)
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			ar, err := txtar.ParseFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if len(ar.Files)%2 != 0 {
				t.Fatalf("%s: want pairs of ADL and Go files", file)
			}
			srcFiles := []string{}
			expectedFiles := []txtar.File{}
			for _, f := range ar.Files {
				if filepath.Ext(f.Name) != ".adl" {
					expectedFiles = append(expectedFiles, f)
					continue
				}
				dir := filepath.Dir(f.Name)
				err = os.MkdirAll(filepath.Join(wk, "testdata", dir), os.ModePerm)
				if err != nil {
					t.Fatalf("mkdir -p %s, error: %v", filepath.Join(wk, "testdata", dir), err)
				}
				fd, err := os.OpenFile(filepath.Join(wk, "testdata", f.Name), os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
				if err != nil {
					t.Fatalf("open %s, error: %v", filepath.Join(wk, "testdata", f.Name), err)
				}
				_, err = fd.Write(f.Data)
				if err != nil {
					t.Fatalf("write %s, error: %v", filepath.Join(wk, "testdata", f.Name), err)
				}
				fd.Close()
				srcFiles = append(srcFiles, filepath.Join(wk, "testdata", f.Name))
			}
			cmd := gengoCmd{
				WorkingDir: wk,
				// Searchdir:  []string{filepath.Join(wk, "testdata")},
				Outputdir: filepath.Join(wk, "output"),
				Debug:     true,
				Files:     srcFiles,
			}
			err = cmd.Run()
			if err != nil {
				t.Fatalf("gengoCmd error: %v\n%+v", err, cmd)
			}
			for _, expect := range expectedFiles {
				gened, err := os.ReadFile(filepath.Join(wk, "output", expect.Name))
				if err != nil {
					t.Fatalf("read %s error: %v\n%+v", filepath.Join(wk, "output", expect.Name), err, cmd)
				}
				have := clean(gened)
				want := clean(expect.Data)
				if !bytes.Equal(have, want) {
					t.Fatalf("%s:%s "+
						"\nhave:\n%s\nwant:\n%s\n"+
						"%s\n%+v", file, expect.Name,
						have, want,
						diff.Diff("have", have, "want", want),
						cmd)
				}
			}
		})
	}
}

func clean(text []byte) []byte {
	text = bytes.Trim(text, "\n")
	// text = bytes.TrimSuffix(text, []byte("\n"))
	// text = bytes.TrimSuffix(text, []byte("^D\n"))
	return text
}
