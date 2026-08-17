package transpile

import (
	"fmt"
	"sort"
	"strings"
)

// Report summarizes the LLM post-work a transpiled file needs: every gap
// recorded during transpilation, a complexity score, and a human-readable
// rendering.
type Report struct {
	Input       string     // input file path
	TSLines     int        // lines of TS source
	DeclCount   int        // top-level declarations in the TS source
	Items       []WorkItem // gaps, in source order
	EmittedOK   bool       // whether transpilation completed
	FatalErrors []string   // internal errors that aborted emission
}

// Score sums the LLM-effort weights of all work items.
func (r *Report) Score() int {
	s := 0
	for _, it := range r.Items {
		s += it.weight()
	}
	return s
}

// Complexity labels the overall difficulty of the LLM post-work.
func (r *Report) Complexity() string {
	switch s := r.Score(); {
	case s == 0 && len(r.FatalErrors) == 0:
		return "TRIVIAL"
	case s <= 5 && len(r.FatalErrors) == 0:
		return "LOW"
	case s <= 15:
		return "MEDIUM"
	case s <= 40:
		return "HIGH"
	default:
		return "VERY HIGH"
	}
}

// BySeverity counts work items per severity.
func (r *Report) BySeverity() map[Severity]int {
	m := map[Severity]int{}
	for _, it := range r.Items {
		m[it.Severity]++
	}
	return m
}

// Manifest renders the work items as a Go comment block suitable for
// embedding at the top of the generated file, so the LLM gets its worklist
// next to the code.
func (r *Report) Manifest() string {
	if len(r.Items) == 0 && len(r.FatalErrors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("// ts2go post-work manifest (LLM TODO list):\n")
	for _, it := range r.Items {
		fmt.Fprintf(&b, "//   [%s] %s:%d %s", it.Severity, baseName(r.Input), it.Line, it.Message)
		if it.Snippet != "" {
			b.WriteString("  (" + it.Snippet + ")")
		}
		b.WriteString("\n")
	}
	for _, e := range r.FatalErrors {
		fmt.Fprintf(&b, "//   [error] %s\n", e)
	}
	return b.String()
}

// String renders the full human-readable report (used by -dry-run and
// -report output).
func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ts2go assessment: %s\n", r.Input)
	fmt.Fprintf(&b, "  TS source:      %d lines, %d top-level declarations\n", r.TSLines, r.DeclCount)
	if !r.EmittedOK {
		fmt.Fprintf(&b, "  EMISSION FAILED: %s\n", strings.Join(r.FatalErrors, "; "))
	}
	sev := r.BySeverity()
	fmt.Fprintf(&b, "  Work items:     %d (%d banned, %d todo, %d approx)\n",
		len(r.Items), sev[SevBanned], sev[SevTodo], sev[SevApprox])
	fmt.Fprintf(&b, "  Complexity:     %s (score %d)\n", r.Complexity(), r.Score())

	if len(r.Items) > 0 {
		b.WriteString("\n  Work items (grouped by line):\n")
		items := append([]WorkItem(nil), r.Items...)
		sort.SliceStable(items, func(i, j int) bool { return items[i].Line < items[j].Line })
		for _, it := range items {
			line := "-"
			if it.Line > 0 {
				line = fmt.Sprint(it.Line)
			}
			fmt.Fprintf(&b, "    %5s  [%-6s] %-12s %s", line, it.Severity, it.Category, it.Message)
			if it.Snippet != "" {
				fmt.Fprintf(&b, "  | %s", it.Snippet)
			}
			b.WriteString("\n")
		}
	}
	if len(r.FatalErrors) > 0 {
		b.WriteString("\n  Fatal errors:\n")
		for _, e := range r.FatalErrors {
			fmt.Fprintf(&b, "    %s\n", e)
		}
	}
	return b.String()
}

// baseName strips directories from a path for compact manifest lines.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}
