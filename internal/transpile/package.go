package transpile

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
)

// importClass classifies a module specifier imported from a file.
type importClass int

const (
	// importSamePackage: the specifier resolves to another file of the same
	// Go package; the import is dropped (identifiers resolve directly).
	importSamePackage importClass = iota
	// importSiblingPackage: resolves to a different Go package of the same
	// project; the LLM must wire up the Go import.
	importSiblingPackage
	// importExternal: node_modules / stdlib / absolute; port or stub.
	importExternal
)

// FileResult is one generated Go file of a transpiled package set.
type FileResult struct {
	Name       string // output file name, e.g. "util.go" (or "jsrt.go")
	RelDir     string // output directory relative to the output root ("" = root)
	Code       string // full Go source (preamble + body)
	Report     *Report
	UsesShim   bool                // code references the jsrt async shim
	SourceFile *tsmorph.SourceFile // nil for the synthesized shim file
}

// PackageResult is the outcome of transpiling a set of TS files.
type PackageResult struct {
	Files  []*FileResult
	Report *Report // aggregate report; per-file reports in Report.Files
}

// Package transpiles a set of TS source files into Go packages.
//
// v1 mapping follows Go's one-directory-one-package rule: files that share
// an output directory become one Go package (named after that directory).
// Imports between files of the same package are dropped (identifiers
// resolve directly); imports into a sibling package of the same project
// become "wire up the Go import" work items; external imports become "port
// or stub" work items. The jsrt async shim, when a package needs it, is
// emitted once as jsrt.go in that package's directory.
type Package struct {
	p        *tsmorph.Project
	files    []*tsmorph.SourceFile
	rootName string // Go package name for files at the output root
}

// NewPackage returns a package transpiler for the given source files. All
// files must belong to p, created with absolute slash virtual paths
// (e.g. /src/a.ts) so relative module imports resolve consistently.
// rootName names the Go package for files at the output root; subdirectories
// are named after their directory.
func NewPackage(p *tsmorph.Project, files []*tsmorph.SourceFile, rootName string) *Package {
	return &Package{p: p, files: files, rootName: rootName}
}

// Transpile converts every file, then assembles the packages: per-file Go
// sources, one shared shim file per package that needs it, and the
// aggregate report.
func (pk *Package) Transpile() (*PackageResult, error) {
	inputRoot := commonDirOf(filePaths(pk.files))
	groupOf := func(sf *tsmorph.SourceFile) string {
		rel, err := filepath.Rel(inputRoot, filepath.ToSlash(sf.FilePath()))
		if err != nil || strings.HasPrefix(rel, "..") {
			return "."
		}
		return filepath.Dir(rel)
	}
	pkgNameOf := func(group string) string {
		if group == "" || group == "." {
			return pk.rootName
		}
		return sanitizePkgName(filepath.Base(group))
	}

	// Group files by output directory; each group is one Go package.
	groups := map[string][]*tsmorph.SourceFile{}
	for _, sf := range pk.files {
		g := groupOf(sf)
		groups[g] = append(groups[g], sf)
	}
	// Package-wide declared names, per group.
	declared := map[string]map[string]bool{}
	for g, sfs := range groups {
		declared[g] = collectDeclaredNames(sfs)
	}
	// Canonical module path → group, for import classification.
	canonical := map[string]string{}
	for _, sf := range pk.files {
		p := strings.TrimSuffix(filepath.ToSlash(sf.FilePath()), filepath.Ext(sf.FilePath()))
		canonical[p] = groupOf(sf)
	}

	classify := func(fromFile, spec string) (importClass, string) {
		target, ok := resolveModule(canonical, fromFile, spec)
		if !ok {
			return importExternal, ""
		}
		fromGroup := canonical[strings.TrimSuffix(filepath.ToSlash(fromFile), filepath.Ext(fromFile))]
		if canonical[target] == fromGroup {
			return importSamePackage, ""
		}
		return importSiblingPackage, pkgNameOf(canonical[target])
	}

	res := &PackageResult{}
	shimPerGroup := map[string]bool{}
	for _, sf := range pk.files {
		g := groupOf(sf)
		t := NewTranspiler(pk.p, sf)
		t.SetPackageName(pkgNameOf(g))
		t.SetPackageContext(declared[g], classify)
		t.SetEmitShim(false)
		out, err := t.Transpile()
		if err != nil {
			return nil, err
		}
		fr := &FileResult{
			Name:       strings.TrimSuffix(filepath.Base(sf.FilePath()), filepath.Ext(sf.FilePath())) + ".go",
			RelDir:     g,
			Code:       out,
			Report:     t.Report(),
			UsesShim:   t.UsedShim(),
			SourceFile: sf,
		}
		res.Files = append(res.Files, fr)
		if fr.UsesShim {
			shimPerGroup[g] = true
		}
	}
	for g := range shimPerGroup {
		res.Files = append(res.Files, &FileResult{
			Name:   "jsrt.go",
			RelDir: g,
			Code:   pk.shimFile(pkgNameOf(g)),
		})
	}
	res.Report = pk.buildReport(res.Files)
	return res, nil
}

// collectDeclaredNames unions every top-level name across the files, so a
// call to a symbol declared in a sibling file of the same package is not
// flagged as an unknown module import.
func collectDeclaredNames(sfs []*tsmorph.SourceFile) map[string]bool {
	declared := map[string]bool{}
	for _, sf := range sfs {
		for _, stmt := range sf.Statements() {
			if n := stmt.Name(); n != "" {
				declared[n] = true
			}
			if vs, ok := stmt.AsVariableStatement(); ok {
				for _, d := range vs.Declarations() {
					if d.Name() != "" {
						declared[d.Name()] = true
					}
				}
			}
		}
	}
	return declared
}

// resolveModule resolves spec, imported from fromFile, against the package's
// canonical module paths; it returns the canonical target path. Handles
// relative specifiers with or without extension and index-file modules.
// Bare specifiers (node_modules, stdlib) never resolve.
func resolveModule(canonical map[string]string, fromFile, spec string) (string, bool) {
	if !strings.HasPrefix(spec, "./") && !strings.HasPrefix(spec, "../") {
		return "", false
	}
	base := path.Join(path.Dir(filepath.ToSlash(fromFile)), spec)
	for _, cand := range []string{base, base + ".ts", base + ".tsx", base + "/index.ts", base + "/index.tsx"} {
		cand = strings.TrimSuffix(cand, path.Ext(cand))
		if _, ok := canonical[cand]; ok {
			return cand, true
		}
	}
	return "", false
}

// shimFile renders jsrt.go for one package: the package clause, the imports
// the shim needs, and the shim source — once per package instead of once
// per file.
func (pk *Package) shimFile(pkgName string) string {
	var b strings.Builder
	b.WriteString("// Code generated by ts2go from TypeScript. DO NOT EDIT.\n")
	b.WriteString("// jsrt: minimal JS-style async runtime, emitted once per package.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", pkgName)
	b.WriteString("import (\n	\"fmt\"\n	\"reflect\"\n	\"regexp\"\n	\"strings\"\n	\"sync\"\n	\"time\"\n)\n\n")
	b.WriteString(strings.TrimPrefix(jsrtShim, "\n"))
	return b.String()
}

// buildReport assembles the aggregate report; per-file reports are attached
// in Files so Score/Complexity/String aggregate across them.
func (pk *Package) buildReport(files []*FileResult) *Report {
	r := &Report{
		Input:     fmt.Sprintf("%d-file project (one Go package per directory)", len(pk.files)),
		EmittedOK: true,
	}
	for _, f := range files {
		if f.Report == nil {
			continue // synthesized shim file has no TS source
		}
		r.Files = append(r.Files, f.Report)
		r.TSLines += f.Report.TSLines
		r.DeclCount += f.Report.DeclCount
		if !f.Report.EmittedOK {
			r.EmittedOK = false
		}
		r.FatalErrors = append(r.FatalErrors, f.Report.FatalErrors...)
	}
	return r
}

// filePaths extracts the virtual paths of the source files.
func filePaths(sfs []*tsmorph.SourceFile) []string {
	ps := make([]string, 0, len(sfs))
	for _, sf := range sfs {
		ps = append(ps, filepath.ToSlash(sf.FilePath()))
	}
	return ps
}

// commonDirOf returns the deepest directory containing every path.
func commonDirOf(paths []string) string {
	if len(paths) == 0 {
		return "."
	}
	parts := strings.Split(path.Dir(paths[0]), "/")
	for _, p := range paths[1:] {
		other := strings.Split(path.Dir(p), "/")
		i := 0
		for i < len(parts) && i < len(other) && parts[i] == other[i] {
			i++
		}
		parts = parts[:i]
	}
	d := strings.Join(parts, "/")
	if d == "" {
		return "/"
	}
	return d
}

// sanitizePkgName turns a directory name into a valid Go package name.
func sanitizePkgName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "main"
	}
	return b.String()
}
