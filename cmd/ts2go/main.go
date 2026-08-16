// Command ts2go converts TypeScript source into Go. It uses ts-go-morph
// (the Go port of the TypeScript compiler) to build a type-checked AST and
// then emits idiomatic Go: interfaces → structs, enums → iota, classes →
// structs + methods, function bodies translated statement-by-statement.
package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts2go/internal/transpile"
)

func main() {
	fs := flag.NewFlagSet("ts2go", flag.ExitOnError)
	outFlag := fs.String("o", "", "output file (default: <input>.go next to input)")
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
	sf := p.CreateSourceFile("/"+filepath.ToSlash(input), string(code))

	t := transpile.NewTranspiler(p, sf)
	out, err := t.Transpile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
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
}
