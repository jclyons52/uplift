package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/jclyons52/ts2go/internal/deps"
)

// runRegistry implements `ts2go registry [dir]`: dump the npm→Go counterpart
// registry. With a dir, marks entries referenced by that codebase. This is
// the seed of a DefinitelyTyped-style shared npm→Go registry.
func runRegistry(args []string) {
	fs := flag.NewFlagSet("registry", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit the raw registry as JSON")
	overlayFlag := fs.String("overlay", "", "optional JSON file extending/fixing the embedded registry")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go registry [dir]\n\nprints the npm→Go counterpart registry (which existing Go modules / stdlib\ncover a TS/npm package, vs which must be ported). With a dir, marks the\npackages that codebase actually references.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(reorderRegistryArgs(args))
	pos := fs.Args()

	overlay, err := os.ReadFile(*overlayFlag)
	if err != nil && *overlayFlag != "" {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	reg := deps.LoadCounterparts(overlay)

	if *jsonOut {
		b, _ := json.MarshalIndent(reg, "", "  ")
		fmt.Println(string(b))
		return
	}

	if len(pos) == 1 {
		res, err := deps.Analyze(pos[0], deps.AnalyzeOptions{CounterpartOverlay: overlay})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		referenced := map[string]bool{}
		for name, n := range res.Nodes {
			if n.Kind == deps.KindExternal {
				referenced[name] = true
			}
		}
		fmt.Print(deps.RenderCounterparts(reg, referenced))
		return
	}
	fmt.Print(deps.RenderCounterparts(reg, nil))
}

func reorderRegistryArgs(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--overlay" || a == "-overlay" {
			flags = append(flags, a)
			if i+1 < len(args) {
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
