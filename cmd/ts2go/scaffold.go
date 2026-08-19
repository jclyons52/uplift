package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jclyons52/ts2go/internal/deps"
	"github.com/jclyons52/ts2go/internal/scaffold"
)

// runScaffold implements `ts2go scaffold <dir>`: for every external package
// the deps analysis recommends separating ("port as its own repo"), create a
// standalone library-repo skeleton (go.mod, package stub, parity test,
// README) under --out.
func runScaffold(args []string) {
	fs := flag.NewFlagSet("scaffold", flag.ExitOnError)
	outFlag := fs.String("out", "", "directory to create repos under (required)")
	prefix := fs.String("module-prefix", "github.com/jclyons52", "Go module prefix for the new repos")
	vendor := fs.Bool("vendor", false, "copy the original node_modules source into <repo>/original/")
	dryRun := fs.Bool("dry-run", false, "print the plan (repos to create) without writing")
	queueFlag := fs.String("queue", "", "write the machine-readable port work queue (JSON, schema port-queue/v1) to this file")
	onlyFlag := fs.String("only", "", "comma-separated package names to scaffold (default: all that are 'own repo')")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ts2go scaffold <dir> --out <reposdir>\n\ncreates a standalone library repo for each external dep the analysis recommends\nseparating (a leaf library 'port as own repo'). Lay down: go.mod, a compiling\npackage stub, a parity-test stub, README, and optionally the original source.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(reorderScaffoldArgs(args))
	pos := fs.Args()
	if len(pos) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	if *outFlag == "" && !*dryRun {
		fmt.Fprintln(os.Stderr, "error: --out is required (or pass --dry-run to preview)")
		os.Exit(2)
	}

	var only map[string]bool
	if *onlyFlag != "" {
		only = map[string]bool{}
		for _, name := range strings.Split(*onlyFlag, ",") {
			if name = strings.TrimSpace(name); name != "" {
				only[name] = true
			}
		}
	}

	res, err := deps.Analyze(pos[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	opts := scaffold.Options{
		Out:          *outFlag,
		ModulePrefix: *prefix,
		SourceRoot:   deps.FindNodeModules(pos[0]),
		Vendor:       *vendor,
		Only:         only,
	}
	repos := scaffold.Plan(res, opts)

	if *queueFlag != "" {
		pq := scaffold.PortQueueOf(repos, opts)
		f, err := os.Create(*queueFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if err := scaffold.WriteQueueJSON(f, pq); err != nil {
			f.Close()
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		f.Close()
		fmt.Fprintf(os.Stderr, "wrote port queue %s (%d repos)\n", *queueFlag, len(pq.Repos))
	}

	if *dryRun {
		fmt.Printf("would scaffold %d repo(s) under %s:\n\n", len(repos), *outFlag)
		for _, r := range repos {
			fmt.Printf("  %-46s %-34s %d LOC  api=%s\n", r.Name, r.Module, r.Loc, apiShort(r.Exports))
		}
		return
	}

	created, err := scaffold.Write(repos, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("scaffolded %d repo(s) under %s:\n", len(created), *outFlag)
	for _, c := range created {
		rel, _ := filepath.Rel(*outFlag, c)
		fmt.Printf("  %s\n", rel)
	}
	fmt.Printf("\nSee %s for the backlog index.\n", filepath.Join(*outFlag, "PORTS.md"))
}

func apiShort(exports []string) string {
	if len(exports) == 0 {
		return "(see original/)"
	}
	if len(exports) > 4 {
		return strings.Join(exports[:4], ",") + ",…"
	}
	return strings.Join(exports, ",")
}

// reorderScaffoldArgs lets flags appear after the positional dir. Only
// --out, --module-prefix and --only consume a following value token.
func reorderScaffoldArgs(args []string) []string {
	valueFlags := map[string]bool{"--out": true, "-out": true, "--module-prefix": true,
		"-module-prefix": true, "--only": true, "-only": true, "--queue": true, "-queue": true}
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
