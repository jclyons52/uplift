package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jclyons52/uplift/internal/deps"
)

// runShim implements `uplift shim [name]`: print the collected set of tiny
// "inline, do-not-split" shims from the counterpart registry. With no name,
// lists them; with a name, prints that package's canonical Go snippet to copy
// into a consumer. This is the retrieval side of "assess once": the verdict
// lives in the registry (counterparts.json), the code here.
func runShim(args []string) {
	fs := flag.NewFlagSet("uplift shim", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: uplift shim [name]\n\n"+
			"inline shims for tiny stdlib-adjacent npm leaves (registry verdict 'inline').\n"+
			"With no name, lists all shims; with a name, prints the canonical Go snippet\n"+
			"to copy into the consumer — these packages are assessed once and must NOT be\n"+
			"split into their own repos.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	names := fs.Args()

	if len(names) == 0 {
		reg := deps.LoadCounterparts(nil)
		keys := deps.ListInlineShims()
		fmt.Printf("inline shims (%d) — registry verdict 'inline', do NOT split into repos:\n\n", len(keys))
		for _, k := range keys {
			note := ""
			if c, ok := reg[k]; ok && c.Note != "" {
				note = "  → " + c.Note
			}
			fmt.Printf("  %-24s%s\n", k, note)
		}
		return
	}

	for i, name := range names {
		snippet, ok := deps.InlineShims[name]
		if !ok {
			// Suggest near matches.
			var close []string
			for k := range deps.InlineShims {
				if strings.Contains(k, name) || strings.Contains(name, k) {
					close = append(close, k)
				}
			}
			sort.Strings(close)
			fmt.Fprintf(os.Stderr, "no shim named %q", name)
			if len(close) > 0 {
				fmt.Fprintf(os.Stderr, " (did you mean: %s?)", strings.Join(close, ", "))
			}
			fmt.Fprintln(os.Stderr)
			os.Exit(1)
		}
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s:\n\n%s\n", name, snippet)
	}
}
