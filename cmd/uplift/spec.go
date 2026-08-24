package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/uplift/internal/spec"
)

// runSpec implements `uplift spec <file.d.ts|dir>`: ingest TypeScript
// declaration files and emit the exported API surface as a stable contract
// (schema spec/v1). This is the "port-to-spec" path — ESLint v8's types live
// in @types/eslint, so `uplift spec @types/eslint/index.d.ts` yields the
// contract the Go port must reproduce, instead of reverse-engineering a 0%
// typed JS source.
func runSpec(args []string) {
	fs := flag.NewFlagSet("uplift spec", flag.ExitOnError)
	jsonOut := fs.String("json", "", "write the spec/v1 report to this file; default: print a human summary")
	pkgFlag := fs.String("package", "", "npm/package name to record in the report header")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: uplift spec [flags] <file.d.ts|dir>...\n\n"+
			"ingests TypeScript declaration files and emits the exported API contract (spec/v1).\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(reorderArgs(args, map[string]bool{"-json": true, "-package": true}, nil))
	inputs := fs.Args()
	if len(inputs) == 0 {
		fs.Usage()
		os.Exit(2)
	}

	files, err := collectDTs(inputs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "error: no .d.ts files found in the given inputs")
		os.Exit(1)
	}

	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	report := &spec.Spec{Schema: spec.Schema, Package: *pkgFlag}
	seen := map[*tsmorph.SourceFile]bool{}
	for _, f := range files {
		code, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		abs, err := filepath.Abs(f)
		if err != nil {
			abs = f
		}
		sf := p.CreateSourceFile(filepath.ToSlash(abs), string(code))
		if sf == nil || seen[sf] {
			continue
		}
		seen[sf] = true
		stem := sourceStem(f)
		one := spec.Extract(p, sf, stem)
		report.Entries = append(report.Entries, one.Entries...)
		if report.File == "" {
			report.File = stem
		}
	}

	if *jsonOut != "" {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*jsonOut, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote spec %s (%d entries)\n", *jsonOut, len(report.Entries))
		return
	}

	printSpecSummary(report, files)
}

// collectDTs expands args into .d.ts files (recursing directories, skipping
// node_modules and hidden dirs).
func collectDTs(args []string) ([]string, error) {
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
			if strings.HasSuffix(a, ".d.ts") {
				add(a)
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
			if strings.HasSuffix(p, ".d.ts") {
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

// sourceStem returns the file path relative-ish stem used for attribution,
// e.g. "index" for "index.d.ts" and "rules/index" for nested files.
func sourceStem(f string) string {
	rel := filepath.Base(f)
	rel = strings.TrimSuffix(rel, ".d.ts")
	return rel
}

// printSpecSummary renders a readable API-contract summary grouped by kind.
func printSpecSummary(r *spec.Spec, files []string) {
	byKind := map[string]int{}
	for _, e := range r.Entries {
		byKind[e.Kind]++
	}
	fmt.Printf("spec/v1 — exported API surface (%d files, %d top-level entries)\n", len(files), len(r.Entries))
	if r.Package != "" {
		fmt.Printf("  package: %s\n", r.Package)
	}
	var kinds []string
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	fmt.Printf("  by kind: ")
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%s=%d", k, byKind[k]))
	}
	fmt.Println(strings.Join(parts, ", "))

	fmt.Println()
	for _, e := range r.Entries {
		line := e.Text
		if line == "" {
			line = e.Name
		}
		if e.Kind == "class" || e.Kind == "interface" {
			line = fmt.Sprintf("%s [%d members]", line, len(e.Members))
		}
		fmt.Printf("  %-10s %s\n", e.Kind, line)
	}
}
