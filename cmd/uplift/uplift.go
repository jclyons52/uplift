package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jclyons52/uplift/internal/deps"
	"github.com/jclyons52/uplift/internal/uplift"
)

// runUplift implements `uplift uplift <dir>`: compute a codebase's quality
// status and the prioritized next uplift actions (types → structure →
// decouple → tests → ports). Emits a stable JSON report (schema uplift/v1)
// for agents, and a readable status for humans — the primary interface.
func runUplift(args []string) {
	fs := flag.NewFlagSet("uplift", flag.ExitOnError)
	jsonOut := fs.String("json", "", "write the structured report (schema uplift/v1) to this file")
	checkUpdates := fs.Bool("check-updates", false, "query the npm registry for the latest version of each leaf dep; flag stale pinned versions before you port (requires network)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: uplift uplift <dir>\n\ncomputes a codebase's uplift status and the prioritized next actions:\n  types (lift) → structure (simplify) → decouple (hubs/cycles) → tests (parity) → ports (leaves).\nThe default output is human-readable; --json emits a stable machine report\n(schema uplift/v1) for agents. Re-run after each stage to see progress.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(reorderUpliftArgs(args))
	pos := fs.Args()
	if len(pos) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	var opts []deps.AnalyzeOptions
	if *checkUpdates {
		opts = append(opts, deps.AnalyzeOptions{CheckUpdates: true})
	}
	rep, err := uplift.Analyze(pos[0], opts...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if *jsonOut != "" {
		f, err := os.Create(*jsonOut)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := rep.WriteJSON(f); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *jsonOut)
		return
	}
	fmt.Print(rep.Render())
}

// reorderUpliftArgs lives in args.go (shared flag-reorder helper).
