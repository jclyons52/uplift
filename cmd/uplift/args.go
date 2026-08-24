package main

import "strings"

// reorderArgs moves flags ahead of positionals so Go's flag package (which
// stops parsing at the first positional) handles `uplift <cmd> <file> -flag
// value` — the natural order — for every subcommand. valueFlags names flags
// that consume the NEXT arg as their value; boolFlags the ones that don't.
// Both -x and --x forms are accepted (normalized on lookup).
func reorderArgs(args []string, valueFlags, boolFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		norm := a
		if strings.HasPrefix(norm, "--") {
			norm = norm[1:]
		}
		if valueFlags[norm] {
			flags = append(flags, a)
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		if boolFlags[norm] {
			flags = append(flags, a)
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

// ts2goValueFlags / ts2goBoolFlags back `uplift ts2go` and the bare
// `uplift <file.ts|dir>` entry point.
var (
	ts2goValueFlags = map[string]bool{"-o": true, "-package": true, "-report": true}
	ts2goBoolFlags  = map[string]bool{"-dry-run": true, "-verify": true, "-type-audit": true}
)

// reorderTranspileArgs is kept for backward compatibility with call sites
// and tests; behaviour is identical to reorderArgs for ts2go's flags.
func reorderTranspileArgs(args []string) []string {
	return reorderArgs(args, ts2goValueFlags, ts2goBoolFlags)
}

var upliftValueFlags = map[string]bool{"-json": true}
var upliftBoolFlags = map[string]bool{"-check-updates": true}

// reorderUpliftArgs backs `uplift uplift <dir>` (status report).
func reorderUpliftArgs(args []string) []string {
	return reorderArgs(args, upliftValueFlags, upliftBoolFlags)
}
