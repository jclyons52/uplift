package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/core"
	"github.com/jclyons52/ts2go/internal/lift"
)

// runLift implements `ts2go lift`: it reads JavaScript (optionally
// JSDoc-typed) and writes annotated TypeScript, adding `: type` to every
// parameter and return the checker resolves — a safe, reviewable commit
// that tightens poorly-typed codebases and feeds the JS→TS→Go pipeline.
func runLift(args []string) {
	fs := flag.NewFlagSet("ts2go lift", flag.ExitOnError)
	outFlag := fs.String("o", "", "output .ts file (single input) or directory (multiple inputs); default: same path with .ts")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go lift [flags] <file.js|dir>...\n\nlifts JavaScript (JSDoc optional) to annotated TypeScript.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	inputs := fs.Args()
	if len(inputs) == 0 {
		fs.Usage()
		os.Exit(2)
	}

	files, err := collectJsInputs(inputs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "error: no .js files found in the given inputs")
		os.Exit(1)
	}

	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{
		UseInMemoryFileSystem: true,
		CompilerOptions:       &core.CompilerOptions{AllowJs: core.TSTrue, CheckJs: core.TSTrue},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	inputRoot := commonDir(files)
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
		sf := p.CreateSourceFile(filepath.ToSlash(abs), string(code))
		if sf == nil {
			fmt.Fprintf(os.Stderr, "error: could not parse %s\n", f)
			os.Exit(1)
		}
		lifted, err := lift.Lift(p, sf)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		// Output path: for a single input, -o names the .ts file directly;
		// otherwise mirror the input tree under the output directory.
		var outPath string
		if len(files) == 1 && *outFlag != "" {
			outPath = *outFlag
		} else {
			outDir := *outFlag
			if outDir == "" {
				outDir = inputRoot
			}
			if len(files) == 1 {
				outPath = filepath.Join(outDir, strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))+".ts")
			} else {
				rel, err := filepath.Rel(inputRoot, abs)
				if err != nil || strings.HasPrefix(rel, "..") {
					rel = filepath.Base(f)
				}
				outPath = filepath.Join(outDir, strings.TrimSuffix(rel, filepath.Ext(rel))+".ts")
			}
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(outPath, []byte(lifted), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		// Rough count of annotations added.
		added := strings.Count(lifted, ": ") - strings.Count(string(code), ": ")
		if added < 0 {
			added = 0
		}
		fmt.Fprintf(os.Stderr, "lifted %s -> %s (+%d annotations)\n", f, outPath, added)
	}
}

// collectJsInputs expands args into .js files (recursing directories,
// skipping node_modules, hidden dirs, and .jsx).
func collectJsInputs(args []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, a := range args {
		st, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if !strings.HasSuffix(a, ".js") {
				return nil, fmt.Errorf("not a .js file: %s", a)
			}
			if !seen[a] {
				seen[a] = true
				files = append(files, a)
			}
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
			if strings.HasSuffix(p, ".js") && !seen[p] {
				seen[p] = true
				files = append(files, p)
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
