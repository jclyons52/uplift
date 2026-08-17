package transpile

import (
	"fmt"
	"sort"
	"strings"
)

// CompileError is one `go build` failure in a generated file, fed back into
// the report by the driver loop so the LLM's worklist includes what the Go
// compiler rejected.
type CompileError struct {
	File    string // generated Go file, relative to the output root
	Line    int    // line in the generated Go file (0 when unknown)
	Col     int    // column (0 when unknown)
	Message string
}

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

	// CompileErrors, when set, holds `go build` failures in the generated
	// file (fed back by the driver loop).
	CompileErrors []CompileError

	// Files, when set, holds the per-file reports of a multi-file package;
	// the fields above then describe the whole package (Items stays empty,
	// Score/Complexity/BySeverity aggregate across the files).
	Files []*Report
}

// Score sums the LLM-effort weights of all work items.
func (r *Report) Score() int {
	s := 0
	for _, it := range r.Items {
		s += it.weight()
	}
	for _, f := range r.Files {
		s += f.Score()
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
	for _, f := range r.Files {
		for sev, n := range f.BySeverity() {
			m[sev] += n
		}
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
// -report output). For a multi-file package it renders the aggregate header
// plus one section per file.
func (r *Report) String() string {
	if len(r.Files) > 0 {
		return r.stringPackage()
	}
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
		renderItems(&b, r.Items, "    ")
	}
	renderCompileErrors(&b, r.CompileErrors, "    ")
	if len(r.FatalErrors) > 0 {
		b.WriteString("\n  Fatal errors:\n")
		for _, e := range r.FatalErrors {
			fmt.Fprintf(&b, "    %s\n", e)
		}
	}
	return b.String()
}

// stringPackage renders the aggregate package assessment with per-file
// sections.
func (r *Report) stringPackage() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ts2go package assessment: %s\n", r.Input)
	fmt.Fprintf(&b, "  TS source:      %d lines, %d top-level declarations across %d files\n",
		r.TSLines, r.DeclCount, len(r.Files))
	if !r.EmittedOK {
		fmt.Fprintf(&b, "  EMISSION FAILED: %s\n", strings.Join(r.FatalErrors, "; "))
	}
	sev := r.BySeverity()
	items := 0
	for _, f := range r.Files {
		items += len(f.Items)
	}
	fmt.Fprintf(&b, "  Work items:     %d (%d banned, %d todo, %d approx)\n",
		items, sev[SevBanned], sev[SevTodo], sev[SevApprox])
	fmt.Fprintf(&b, "  Complexity:     %s (score %d)\n", r.Complexity(), r.Score())
	for _, f := range r.Files {
		b.WriteString("\n  --- " + f.Input + " ---\n")
		fmt.Fprintf(&b, "    %d lines, %d declarations, %d work items\n",
			f.TSLines, f.DeclCount, len(f.Items))
		if len(f.Items) > 0 {
			renderItems(&b, f.Items, "      ")
		}
		renderCompileErrors(&b, f.CompileErrors, "      ")
		if len(f.FatalErrors) > 0 {
			for _, e := range f.FatalErrors {
				fmt.Fprintf(&b, "      [error] %s\n", e)
			}
		}
	}
	renderCompileErrors(&b, r.CompileErrors, "  ")
	return b.String()
}

// renderCompileErrors writes go build findings, when any.
func renderCompileErrors(b *strings.Builder, errs []CompileError, indent string) {
	if len(errs) == 0 {
		return
	}
	b.WriteString("\n  go build findings:\n")
	for _, e := range errs {
		loc := e.File
		if e.Line > 0 {
			loc = fmt.Sprintf("%s:%d", e.File, e.Line)
			if e.Col > 0 {
				loc = fmt.Sprintf("%s:%d", loc, e.Col)
			}
		}
		fmt.Fprintf(b, "%s  %s: %s\n", indent, loc, e.Message)
	}
}

// renderItems writes work items sorted by TS line.
func renderItems(b *strings.Builder, items []WorkItem, indent string) {
	sorted := append([]WorkItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Line < sorted[j].Line })
	for _, it := range sorted {
		line := "-"
		if it.Line > 0 {
			line = fmt.Sprint(it.Line)
		}
		fmt.Fprintf(b, "%s%5s  [%-6s] %-12s %s", indent, line, it.Severity, it.Category, it.Message)
		if it.Snippet != "" {
			fmt.Fprintf(b, "  | %s", it.Snippet)
		}
		b.WriteString("\n")
	}
}

// baseName strips directories from a path for compact manifest lines.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}
