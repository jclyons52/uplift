package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jclyons52/ts2go/internal/uplift"
)

// runUplift implements `ts2go uplift <dir>`: compute a codebase's quality
// status and the prioritized next uplift actions (types → structure →
// decouple → tests → ports). Emits a stable JSON report (schema uplift/v1)
// for agents, and a readable status for humans — the primary interface.
func runUplift(args []string) {
	fs := flag.NewFlagSet("uplift", flag.ExitOnError)
	jsonOut := fs.String("json", "", "write the structured report (schema uplift/v1) to this file")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go uplift <dir>\n\ncomputes a codebase's uplift status and the prioritized next actions:\n  types (lift) → structure (simplify) → decouple (hubs/cycles) → tests (parity) → ports (leaves).\nThe default output is human-readable; --json emits a stable machine report\n(schema uplift/v1) for agents. Re-run after each stage to see progress.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(reorderUpliftArgs(args))
	pos := fs.Args()
	if len(pos) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	rep, err := uplift.Analyze(pos[0])
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

func reorderUpliftArgs(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--json" || a == "-json" {
			flags = append(flags, a)
			if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] != '-' {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		if len(a) > 0 && a[0] == '-' {
			flags = append(flags, a)
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
}
