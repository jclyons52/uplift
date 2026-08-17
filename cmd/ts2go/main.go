// Command ts2go converts TypeScript source into Go. It uses ts-go-morph
// (the Go port of the TypeScript compiler) to build a type-checked AST and
// then emits idiomatic Go: interfaces → structs, enums → iota, classes →
// structs + methods, function bodies translated statement-by-statement.
//
// The output targets LLM cleanup: anything that cannot be translated is
// emitted as a compiling TODO placeholder, embedded in a post-work manifest
// at the top of the file, and listed in a report (-report / -dry-run).
package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"path"
	"path/filepath"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts2go/internal/transpile"
)

func main() {
	fs := flag.NewFlagSet("ts2go", flag.ExitOnError)
	outFlag := fs.String("o", "", "output file (default: <input>.go next to input)")
	dryRun := fs.Bool("dry-run", false, "assess only: transpile in memory, print the post-work report, write nothing")
	reportFlag := fs.String("report", "", "write the full post-work report to this file (markdown)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go [flags] <file.ts>\n\nconverts a TypeScript file to Go.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(os.Args[1:])

	args := fs.Args()
	if len(args) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	input := args[0]

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
	vpath := filepath.ToSlash(input)
	if !path.IsAbs(vpath) {
		vpath = "/" + vpath
	}
	sf := p.CreateSourceFile(vpath, string(code))

	t := transpile.NewTranspiler(p, sf)
	out, err := t.Transpile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	report := t.Report()

	if *dryRun {
		fmt.Print(report.String())
		if *reportFlag != "" {
			writeReport(*reportFlag, report.String())
		}
		return
	}

	// Format like gofmt so the output is clean Go (field alignment etc.).
	if formatted, err := format.Source([]byte(out)); err == nil {
		out = string(formatted)
	}

	outPath := *outFlag
	if outPath == "" {
		base := filepath.Base(input)
		ext := filepath.Ext(base)
		outPath = strings.TrimSuffix(base, ext) + ".go"
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
	if *reportFlag != "" {
		writeReport(*reportFlag, report.String())
	}
}

func writeReport(path, text string) {
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error writing report:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote report %s\n", path)
}
