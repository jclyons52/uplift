package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jclyons52/uplift/internal/measure"
)

// runMeasure implements `uplift measure <dir>`: compute a comparable quality
// metric set (type coverage, complexity, coupling, tests) so uplift progress
// is measurable between stages.
func runMeasure(args []string) {
	fs := flag.NewFlagSet("measure", flag.ExitOnError)
	jsonOut := fs.String("json", "", "write the structured report to this file (overrides the human report)")
	compareFlag := fs.String("compare", "", "two JSON snapshot files (before,after) to diff instead of measuring a dir")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: uplift measure <dir> [--json out.json]\n       uplift measure --compare before.json after.json [--json delta.json]\n\ncomputes a codebase quality metric set (schema measure/v1):\n  - type coverage (annotated sites ratio)\n  - cyclomatic complexity (total/avg/max + top files)\n  - coupling (modules, edges, density, fan-out, hubs, cycles)\n  - tests (test files, parity suites)\n\nRun before and after each uplift stage to measure the delta.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(reorderMeasureArgs(args))
	pos := fs.Args()

	if *compareFlag != "" {
		before, after, err := loadTwo(*compareFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		d := measure.Compare(before, after)
		if *jsonOut != "" {
			f, err := os.Create(*jsonOut)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			defer f.Close()
			if err := d.WriteJSON(f); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "wrote %s\n", *jsonOut)
			return
		}
		fmt.Print(d.Render())
		return
	}

	if len(pos) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	m, err := measure.Analyze(pos[0])
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
		if err := m.WriteJSON(f); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *jsonOut)
		return
	}
	fmt.Print(m.Render())
}

// loadTwo reads two JSON metric snapshots.
func loadTwo(flag string) (before, after *measure.Metrics, err error) {
	parts := splitTwo(flag)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("--compare expects two comma/space-separated paths")
	}
	before = &measure.Metrics{}
	after = &measure.Metrics{}
	b, err := os.ReadFile(parts[0])
	if err != nil {
		return nil, nil, err
	}
	a, err := os.ReadFile(parts[1])
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(b, before); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", parts[0], err)
	}
	if err := json.Unmarshal(a, after); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", parts[1], err)
	}
	return before, after, nil
}

func splitTwo(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func reorderMeasureArgs(args []string) []string {
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
