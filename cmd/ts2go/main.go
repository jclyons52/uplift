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
	// Subcommand: `ts2go lift <file.js|dir>` — JS→TS type lifting.
	if len(os.Args) > 1 && os.Args[1] == "lift" {
		runLift(os.Args[2:])
		return
	}
	// Subcommand: `ts2go deps <dir>` — module-graph / leaf-node analysis.
	if len(os.Args) > 1 && os.Args[1] == "deps" {
		runDeps(os.Args[2:])
		return
	}
	// Subcommand: `ts2go scaffold <dir>` — per-leaf library repo skeletons.
	if len(os.Args) > 1 && os.Args[1] == "scaffold" {
		runScaffold(os.Args[2:])
		return
	}
	// Subcommand: `ts2go registry [dir]` — npm→Go counterpart registry.
	if len(os.Args) > 1 && os.Args[1] == "registry" {
		runRegistry(os.Args[2:])
		return
	}
	// Subcommand: `ts2go measure <dir>` — codebase quality metric report.
	if len(os.Args) > 1 && os.Args[1] == "measure" {
		runMeasure(os.Args[2:])
		return
	}
	// Subcommand: `ts2go bench` — node vs Go port benchmark.
	if len(os.Args) > 1 && os.Args[1] == "bench" {
		runBench(os.Args[2:])
		return
	}
	// Subcommand: `ts2go uplift <dir>` — status + prioritized next actions.
	if len(os.Args) > 1 && os.Args[1] == "uplift" {
		runUplift(os.Args[2:])
		return
	}
	fs := flag.NewFlagSet("ts2go", flag.ExitOnError)
	outFlag := fs.String("o", "", "output file (single input) or directory (multiple inputs); default: next to the input")
	pkgFlag := fs.String("package", "", "Go package name for multi-file output (default: output directory base name)")
	dryRun := fs.Bool("dry-run", false, "assess only: transpile in memory, print the post-work report, write nothing")
	verify := fs.Bool("verify", true, "build the generated Go in a temp module and report compile errors as work items")
	typeAudit := fs.Bool("type-audit", false, "also print the type-tightening worklist: sites where the checker resolves a concrete type but ts2go emitted any/dynamic")
	reportFlag := fs.String("report", "", "write the full post-work report to this file (markdown)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go [flags] <file.ts|dir>...\n\nconverts TypeScript files to Go (one Go package per directory).\n\nflags:\n")
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
			runSingle(inputs[0], *outFlag, *dryRun, *verify, *typeAudit, *reportFlag)
			return
		}
	}
	runPackage(inputs, *outFlag, *pkgFlag, *dryRun, *verify, *typeAudit, *reportFlag)
}

// runSingle transpiles one file to one .go file (v0 behaviour).
func runSingle(input, outFlag string, dryRun, verify, typeAudit bool, reportFlag string) {
	if err := validateInput(input); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
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
	if sf == nil {
		fmt.Fprintf(os.Stderr, "error: could not parse %s as TypeScript\n", input)
		os.Exit(1)
	}

	t := transpile.NewTranspiler(p, sf)
	out, err := t.Transpile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	file := &transpile.FileResult{
		Name:       strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + ".go",
		Code:       out,
		Report:     t.Report(),
		UsesShim:   t.UsedShim(),
		SourceFile: sf,
	}
	if verify {
		attachCompileErrors([]*transpile.FileResult{file}, verifyBuild([]*transpile.FileResult{file}))
	}

	if dryRun {
		fmt.Print(file.Report.String())
		if typeAudit {
			fmt.Println()
			fmt.Print(file.Report.AuditString())
		}
		if reportFlag != "" {
			writeReport(reportFlag, file.Report.String())
		}
		return
	}

	outPath := outFlag
	if outPath == "" {
		outPath = file.Name
	}
	if err := os.WriteFile(outPath, []byte(gofmt(file.Code)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", outPath)

	// Always surface the assessment summary; the full worklist is in the
	// generated file's manifest and optionally in -report.
	if len(file.Report.Items) > 0 || len(file.Report.CompileErrors) > 0 || len(file.Report.FatalErrors) > 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, file.Report.String())
	}
	if typeAudit {
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, file.Report.AuditString())
	}
	if reportFlag != "" {
		writeReport(reportFlag, file.Report.String())
	}
}

// runPackage transpiles several files (or a directory tree) into a single
// Go package: one .go file per input, imports between inputs dropped,
// external imports reported as work items, jsrt.go emitted once if any file
// uses async.
func runPackage(inputs []string, outFlag, pkgFlag string, dryRun, verify, typeAudit bool, reportFlag string) {
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
		if sfs[len(sfs)-1] == nil {
			fmt.Fprintf(os.Stderr, "error: could not parse %s as TypeScript\n", f)
			os.Exit(1)
		}
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

	// Driver loop: build the generated Go in a temp module and feed every
	// compiler failure back into the per-file reports and the generated
	// files themselves.
	if verify {
		unmatched := attachCompileErrors(res.Files, verifyBuild(res.Files))
		res.Report.CompileErrors = append(res.Report.CompileErrors, unmatched...)
	}

	if dryRun {
		fmt.Print(res.Report.String())
		if typeAudit {
			fmt.Println()
			fmt.Print(res.Report.AuditString())
		}
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
	if typeAudit {
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, res.Report.AuditString())
	}
	if reportFlag != "" {
		writeReport(reportFlag, res.Report.String())
	}
}

// validateInput rejects files ts2go cannot transpile with a clear message
// instead of a panic (a .js source file produces a nil SourceFile).
func validateInput(path string) error {
	if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
		return fmt.Errorf("unsupported input %q: ts2go transpiles TypeScript. JavaScript/JSDoc support (ESLint-style codebases) is a planned future phase", path)
	}
	return nil
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
			if err := validateInput(a); err != nil {
				return nil, err
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
