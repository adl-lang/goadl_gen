package gen_go_v2

import (
	"strconv"
	"strings"
	"unicode"
)

type imports struct {
	specs []importSpec
	used  map[string]bool // keyed on import path
}

type importSpec struct {
	path    string
	aliased bool
	name    string
}

func newImports() imports {
	im := imports{used: make(map[string]bool)}
	// for _, spec := range importList {
	// 	i.reserve(spec)
	// }
	return im
}

func (spec importSpec) String() string {
	if !spec.aliased {
		return strconv.Quote(spec.path)
	}
	return spec.name + " " + strconv.Quote(spec.path)
}

func (i *imports) byPath(path string) (spec importSpec, ok bool) {
	for _, spec = range i.specs {
		if spec.path == path {
			return spec, true
		}
	}
	return importSpec{}, false
}

func (i *imports) byName(name string) (spec importSpec, ok bool) {
	for _, spec = range i.specs {
		if spec.name == name {
			return spec, true
		}
	}
	return importSpec{}, false
}

func (i *imports) add(path string) (name string) {
	spec := i.reserve(path)
	i.used[spec.path] = true
	return name
}

// reserve adds an import spec without marking it as used.
func (i *imports) reserve(path string) importSpec {
	if ispec, ok := i.byPath(path); ok {
		return ispec
	}
	spec := importSpec{
		path:    path,
		name:    pkgFromImport(path),
		aliased: false,
	}
	if _, found := i.byName(spec.name); found {
		spec.aliased = true
		for base, n := spec.name, uint64(2); ; n++ {
			spec.name = base + strconv.FormatUint(n, 10)
			if _, found = i.byName(spec.name); !found {
				break
			}
		}
	}
	i.specs = append(i.specs, spec)
	return spec
}

func pkgFromImport(path string) string {
	if i := strings.LastIndex(path, "/"); i != -1 {
		path = path[i+1:]
	}
	p := []rune(path)
	n := 0
	for _, r := range p {
		if isIdent(r) {
			p[n] = r
			n++
		}
	}
	if n == 0 || !isLower(p[0]) {
		return "pkg" + string(p[:n])
	}
	return string(p[:n])
}

func isLower(r rune) bool {
	return 'a' <= r && r <= 'z' || r == '_'
}

func isIdent(r rune) bool {
	return isLower(r) || 'A' <= r && r <= 'Z' || r >= 0x80 && unicode.IsLetter(r)
}
