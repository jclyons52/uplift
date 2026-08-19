package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jclyons52/ts2go/internal/bench"
)

// runBench implements `ts2go bench`: benchmark a pure function in both JS
// (node) and a Go port across the same inputs, measuring wall time, memory,
// and ops/sec — the performance/resource uplift metric.
func runBench(args []string) {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	name := fs.String("name", "bench", "benchmark name")
	iters := fs.Int("iters", 200000, "outer iterations")
	inputsFlag := fs.String("inputs", "", "comma-separated input values (or a file path containing them, one per line)")
	argType := fs.String("arg-type", "string", "argument kind: string | int")
	jsModule := fs.String("js-module", "", "absolute path to the JS module exporting js-func")
	jsFunc := fs.String("js-func", "", "exported JS function name to benchmark")
	goImport := fs.String("go-import", "", "Go import path of the ported repo (e.g. github.com/jclyons52/esutils-go)")
	goDir := fs.String("go-dir", "", "absolute directory of the Go repo")
	goFunc := fs.String("go-func", "", "exported Go function name to benchmark")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go bench --js-module <js file> --js-func <fn> --go-import <imp> --go-dir <dir> --go-func <fn> [--inputs ...] [--iters N]\n\nbenchmarks a single-argument pure function (string|int) in node vs the Go\nport across the same inputs — wall time, memory, ops/sec.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(reorderBenchArgs(args))

	inputs := loadInputs(*inputsFlag)
	if len(inputs) == 0 {
		inputs = []string{"foo", "someIdentifier", "class", "1bad", "日本語abc", "_x1"}
	}
	o := bench.Options{
		Name:       *name,
		Iterations: *iters,
		Inputs:     inputs,
		ArgType:    *argType,
		JSModule:   *jsModule,
		JSFunc:     *jsFunc,
		GoImport:   *goImport,
		GoDir:      *goDir,
		GoFunc:     *goFunc,
	}
	if o.JSModule == "" || o.JSFunc == "" || o.GoImport == "" || o.GoDir == "" || o.GoFunc == "" {
		fs.Usage()
		os.Exit(2)
	}
	js, gores, err := bench.Run(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Print(bench.Render(o, js, gores))
}

func loadInputs(spec string) []string {
	if spec == "" {
		return nil
	}
	if b, err := os.ReadFile(spec); err == nil {
		var out []string
		for _, line := range strings.Split(string(b), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				out = append(out, line)
			}
		}
		return out
	}
	var out []string
	for _, p := range strings.Split(spec, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func reorderBenchArgs(args []string) []string {
	valueFlags := map[string]bool{"--inputs": true, "-inputs": true, "--js-module": true,
		"-js-module": true, "--js-func": true, "-js-func": true, "--go-import": true,
		"-go-import": true, "--go-dir": true, "-go-dir": true, "--go-func": true,
		"-go-func": true, "--name": true, "-name": true, "--arg-type": true,
		"-arg-type": true, "--iters": true, "-iters": true}
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if valueFlags[a] {
			flags = append(flags, a)
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
}
