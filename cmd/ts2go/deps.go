package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jclyons52/ts2go/internal/deps"
)

// runDeps implements `ts2go deps <dir>`: analyze a codebase's module graph and
// print the dependency/size/leaf report that supports the "port each
// dependency as its own library in a separate repo" strategy.
func runDeps(args []string) {
	fs := flag.NewFlagSet("deps", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit the graph as JSON (for downstream tooling) instead of the human report")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go deps <dir>\n\nanalyzes a JS/TS codebase's module graph (internal files, node builtins, external npm packages) and reports:\n  - which files/packages require each dependency (need)\n  - which external packages are leaf nodes (own their whole subtree)\n  - size (LOC) and a split-vs-absorb recommendation\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	pos := fs.Args()
	if len(pos) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	res, err := deps.Analyze(pos[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if *jsonOut {
		if err := res.WriteJSON(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Print(res.Render())
}
