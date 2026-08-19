package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jclyons52/uplift/internal/deps"
)

// runDeps implements `uplift deps <dir>`: analyze a codebase's module graph and
// print the dependency/size/leaf report that supports the "port each
// dependency as its own library in a separate repo" strategy.
func runDeps(args []string) {
	fs := flag.NewFlagSet("deps", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit the graph as JSON (for downstream tooling) instead of the human report")
	hintsFlag := fs.String("hints", "", "path to a JSON file of { package: recommendation } overrides merged over the built-in map")
	registryFlag := fs.String("registry", "", "path to a JSON file extending/fixing the npm→Go counterpart registry")
	checkUpdates := fs.Bool("check-updates", false, "query the npm registry for the latest version of each external package, flagging stale pinned versions (offline by default)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: uplift deps <dir>\n\nanalyzes a JS/TS codebase's module graph (internal files, node builtins, external npm packages) and reports:\n  - which files/packages require each dependency (need)\n  - which external packages are leaf nodes (own their whole subtree)\n  - size (LOC), exported-symbol count, and a split-vs-absorb recommendation\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(reorderDepsArgs(args))
	pos := fs.Args()
	if len(pos) != 1 {
		fs.Usage()
		os.Exit(2)
	}

	var opts deps.AnalyzeOptions
	opts.CheckUpdates = *checkUpdates
	if *hintsFlag != "" {
		h, err := readHints(*hintsFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		opts.Hints = h
	}
	if *registryFlag != "" {
		b, err := os.ReadFile(*registryFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		opts.CounterpartOverlay = b
	}

	res, err := deps.Analyze(pos[0], opts)
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

// reorderDepsArgs lets flags appear after the positional dir (`uplift deps
// <dir> --json`), which the flag package otherwise stops parsing at. Only
// --hints consumes a value token, so this is unambiguous.
func reorderDepsArgs(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--hints" || a == "-hints" || a == "--registry" || a == "-registry" {
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

// readHints loads a { "package": "recommendation" } JSON map from a file.
func readHints(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read hints: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse hints %s: %w", path, err)
	}
	return m, nil
}
