// Command ts2go converts TypeScript source into Go. It uses ts-go-morph
// (the Go port of the TypeScript compiler) to build a type-checked AST and
// then emits idiomatic Go: interfaces → structs, enums → iota, classes →
// structs + methods, function bodies translated statement-by-statement.
//
// Input can be one or more .ts files or directories (directories are walked
// recursively; *.d.ts and node_modules are skipped). Multiple inputs are
// transpiled into a single Go package: imports between the inputs are
// dropped (same package), external imports become work items, and the jsrt
// async shim is emitted once as jsrt.go.
//
// The output targets LLM cleanup: anything that cannot be translated is
// emitted as a compiling TODO placeholder, embedded in a post-work manifest
// at the top of each file, and listed in a report (-report / -dry-run).
package main

import (
	"flag"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts2go/internal/transpile"
)

func main() {
	fs := flag.NewFlagSet("ts2go", flag.ExitOnError)
	outFlag := fs.String("o", "", "output file (single input) or directory (multiple inputs); default: next to the input")
	pkgFlag := fs.String("package", "", "Go package name for multi-file output (default: output directory base name)")
	dryRun := fs.Bool("dry-run", false, "assess only: transpile in memory, print the post-work report, write nothing")
	reportFlag := fs.String("report", "", "write the full post-work report to this file (markdown)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go [flags] <file.ts|dir>...\n\nconverts TypeScript files to Go (one Go package).\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(os.Args[1:])

	inputs := fs.Args()
	if len(inputs) == 0 {
		fs.Usage()
		os.Exit(2)
	}

	// Single file argument: keep the v0 single-file behaviour (-o is a file,
	// shim appended inline). A directory (or several files) goes through the
	// package path.
	if len(inputs) == 1 {
		if st, err := os.Stat(inputs[0]); err == nil && !st.IsDir() {
			runSingle(inputs[0], *outFlag, *dryRun, *reportFlag)
			return
		}
	}
	runPackage(inputs, *outFlag, *pkgFlag, *dryRun, *reportFlag)
}

// runSingle transpiles one file to one .go file (v0 behaviour).
func runSingle(input, outFlag string, dryRun bool, reportFlag string) {
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	code, err := os.ReadFile(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	sf := p.CreateSourceFile(virtualPath(input), string(code))

	t := transpile.NewTranspiler(p, sf)
	out, err := t.Transpile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	report := t.Report()

	if dryRun {
		fmt.Print(report.String())
		if reportFlag != "" {
			writeReport(reportFlag, report.String())
		}
		return
	}

	out = gofmt(out)

	outPath := outFlag
	if outPath == "" {
		outPath = strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + ".go"
	}
	if err := os.WriteFile(outPath, []byte(out), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", outPath)

	// Always surface the assessment summary; the full worklist is in the
	// generated file's manifest and optionally in -report.
	if len(report.Items) > 0 || len(report.FatalErrors) > 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, report.String())
	}
	if reportFlag != "" {
		writeReport(reportFlag, report.String())
	}
}

// runPackage transpiles several files (or a directory tree) into a single
// Go package: one .go file per input, imports between inputs dropped,
// external imports reported as work items, jsrt.go emitted once if any file
// uses async.
func runPackage(inputs []string, outFlag, pkgFlag string, dryRun bool, reportFlag string) {
	files, err := collectInputs(inputs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "error: no .ts files found in the given inputs")
		os.Exit(1)
	}

	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	var sfs []*tsmorph.SourceFile
	for _, f := range files {
		code, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		abs, err := filepath.Abs(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		sfs = append(sfs, p.CreateSourceFile(filepath.ToSlash(abs), string(code)))
	}

	outDir := outFlag
	if outDir == "" {
		outDir = commonDir(files)
	}
	pkgName := pkgFlag
	if pkgName == "" {
		pkgName = sanitizePkg(filepath.Base(outDir))
	}

	pkg := transpile.NewPackage(p, sfs, pkgName)
	res, err := pkg.Transpile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if dryRun {
		fmt.Print(res.Report.String())
		if reportFlag != "" {
			writeReport(reportFlag, res.Report.String())
		}
		return
	}

	inputRoot := commonDir(files)
	for _, f := range res.Files {
		rel := f.RelDir
		if f.SourceFile != nil {
			rel, err = filepath.Rel(inputRoot, f.SourceFile.FilePath())
			if err != nil || strings.HasPrefix(rel, "..") {
				rel = f.RelDir
			}
			rel = filepath.Dir(rel)
		}
		outPath := filepath.Join(outDir, rel, f.Name)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(outPath, []byte(gofmt(f.Code)), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", outPath)
	}

	items := 0
	for _, f := range res.Report.Files {
		items += len(f.Items)
	}
	if items > 0 || len(res.Report.FatalErrors) > 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, res.Report.String())
	}
	if reportFlag != "" {
		writeReport(reportFlag, res.Report.String())
	}
}

// collectInputs expands args into an ordered, de-duplicated list of .ts
// files. Directories are walked recursively, skipping node_modules, hidden
// directories, and *.d.ts declaration files.
func collectInputs(args []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	add := func(f string) {
		if !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	for _, a := range args {
		st, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if !strings.HasSuffix(a, ".ts") {
				return nil, fmt.Errorf("not a .ts file: %s", a)
			}
			add(a)
			continue
		}
		err = filepath.WalkDir(a, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".ts") && !strings.HasSuffix(p, ".d.ts") {
				add(p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

// commonDir returns the deepest directory containing every path.
func commonDir(paths []string) string {
	parts := strings.Split(filepath.Dir(paths[0]), string(filepath.Separator))
	for _, p := range paths[1:] {
		other := strings.Split(filepath.Dir(p), string(filepath.Separator))
		i := 0
		for i < len(parts) && i < len(other) && parts[i] == other[i] {
			i++
		}
		parts = parts[:i]
	}
	d := strings.Join(parts, string(filepath.Separator))
	if d == "" {
		return string(filepath.Separator)
	}
	return d
}

var pkgNameRe = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// sanitizePkg turns a directory name into a valid Go package name.
func sanitizePkg(s string) string {
	s = pkgNameRe.ReplaceAllString(s, "_")
	if s == "" {
		return "main"
	}
	return strings.ToLower(s)
}

// virtualPath makes a relative input path look absolute for the in-memory
// project (single-file mode; import resolution needs an absolute base).
func virtualPath(input string) string {
	v := filepath.ToSlash(input)
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	return v
}

// gofmt formats generated Go; failures are non-fatal (the output still
// compiles, just less tidy).
func gofmt(src string) string {
	if formatted, err := format.Source([]byte(src)); err == nil {
		return string(formatted)
	}
	return src
}

func writeReport(path, text string) {
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error writing report:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote report %s\n", path)
}
