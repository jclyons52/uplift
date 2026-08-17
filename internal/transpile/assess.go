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

	// Audit lists type-tightening sites: emitted as any/dynamic where the
	// checker resolved a concrete type (see TightenSite).
	Audit []TightenSite

	// Files, when set, holds the per-file reports of a multi-file package;
	// the fields above then describe the whole package (Items stays empty,
	// Score/Complexity/BySeverity aggregate across the files).
	Files []*Report
}

// GroupedWorkItem collapses the many identical work items a large file
// produces (types.ts emits ~50k items; checker.ts ~5k+ distinct gaps) into
// one entry with a count and a line range, so the manifest stays a worklist.
type GroupedWorkItem struct {
	Severity  Severity
	Category  string
	Message   string
	Count     int    // number of identical items collapsed into this one
	FirstLine int    // earliest TS line (0 if none)
	LastLine  int    // latest TS line
	Snippet   string // sample snippet for context
}

// CategoryCount is one category's tally for the report rollup.
type CategoryCount struct {
	Category string
	Count    int
	Severity Severity // most common severity in the category
}

// GroupedItems collapses identical (Severity, Category, Message) items,
// preserving first-seen order.
func (r *Report) GroupedItems() []GroupedWorkItem {
	var out []GroupedWorkItem
	idx := map[string]int{}
	for _, it := range r.Items {
		key := string(it.Severity) + "\x00" + it.Category + "\x00" + it.Message
		if i, ok := idx[key]; ok {
			out[i].Count++
			if it.Line > 0 && (it.Line < out[i].FirstLine || out[i].FirstLine == 0) {
				out[i].FirstLine = it.Line
			}
			if it.Line > out[i].LastLine {
				out[i].LastLine = it.Line
			}
			continue
		}
		idx[key] = len(out)
		out = append(out, GroupedWorkItem{
			Severity:  it.Severity,
			Category:  it.Category,
			Message:   it.Message,
			Count:     1,
			FirstLine: it.Line,
			LastLine:  it.Line,
			Snippet:   it.Snippet,
		})
	}
	return out
}

// TopCategories returns the n most frequent work-item categories by count,
// for the report's rollup.
func (r *Report) TopCategories(n int) []CategoryCount {
	m := map[string]*CategoryCount{}
	for _, it := range r.Items {
		cc := m[it.Category]
		if cc == nil {
			cc = &CategoryCount{Category: it.Category, Severity: it.Severity}
			m[it.Category] = cc
		}
		cc.Count++
		// Keep the highest-severity severity seen (banned > todo > approx).
		if weight(it.Severity) > weight(cc.Severity) {
			cc.Severity = it.Severity
		}
	}
	all := make([]CategoryCount, 0, len(m))
	for _, cc := range m {
		all = append(all, *cc)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Count > all[j].Count })
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// weight returns the gap weight for a severity (todo 3, banned 10, else 1).
func weight(s Severity) int {
	switch s {
	case SevBanned:
		return 10
	case SevTodo:
		return 3
	default:
		return 1
	}
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
// next to the code. Identical items are collapsed with a count and line
// range, so a 50k-item file stays readable.
func (r *Report) Manifest() string {
	grouped := r.GroupedItems()
	if len(grouped) == 0 && len(r.FatalErrors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("// ts2go post-work manifest (LLM TODO list):\n")
	for _, it := range grouped {
		loc := fmt.Sprintf("%s:%d", baseName(r.Input), it.FirstLine)
		if it.FirstLine != it.LastLine {
			loc = fmt.Sprintf("%s:%d-%d", baseName(r.Input), it.FirstLine, it.LastLine)
		}
		fmt.Fprintf(&b, "//   [%s] %s x%d %s", it.Severity, loc, it.Count, it.Message)
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
		renderCategoryRollup(&b, r.TopCategories(5))
		b.WriteString("\n  Work items (grouped; line ranges, x=count):\n")
		renderItems(&b, r.GroupedItems(), "    ")
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
		fmt.Fprintf(&b, "    %d lines, %d declarations, %d work items (%d distinct)\n",
			f.TSLines, f.DeclCount, len(f.Items), len(f.GroupedItems()))
		if len(f.Items) > 0 {
			renderCategoryRollup(&b, f.TopCategories(5))
			renderItems(&b, f.GroupedItems(), "      ")
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

// AuditString renders the type-tightening worklist: the sites where the
// checker resolved a concrete type but ts2go emitted any/dynamic, grouped by
// category+checker-type. This is what `-type-audit` prints — annotate these
// source sites to tighten the output.
func (r *Report) AuditString() string {
	count := 0
	for _, f := range r.Files {
		count += len(f.Audit)
	}
	count += len(r.Audit)
	var b strings.Builder
	fmt.Fprintf(&b, "ts2go type audit: %d tightenable sites (checker knows more than emitted)\n", count)
	if count == 0 {
		b.WriteString("  none — the checker already resolves to any at every degraded site\n")
		return b.String()
	}
	// Aggregate per file (single-file mode: the report itself).
	var pass []*Report
	if len(r.Files) == 0 {
		pass = []*Report{r}
	} else {
		pass = r.Files
	}
	for _, f := range pass {
		if len(f.Audit) == 0 {
			continue
		}
		b.WriteString("\n  --- " + f.Input + " (" + fmt.Sprint(len(f.Audit)) + " sites) ---\n")
		renderAudit(&b, f.Audit)
	}
	renderAudit(&b, r.Audit)
	return b.String()
}

// renderAudit writes tighten sites grouped by category + checker type.
func renderAudit(b *strings.Builder, sites []TightenSite) {
	type gkey struct{ cat, t string }
	m := map[gkey]*GroupedWorkItem{}
	order := []gkey{}
	for _, s := range sites {
		k := gkey{s.Category, s.CheckerType}
		g, ok := m[k]
		if !ok {
			g = &GroupedWorkItem{Category: s.Category, Severity: SevTodo, FirstLine: s.Line, LastLine: s.Line, Count: 1, Message: "checker resolves to " + s.CheckerType, Snippet: s.Snippet}
			m[k] = g
			order = append(order, k)
		} else {
			g.Count++
			if s.Line > 0 && (s.Line < g.FirstLine || g.FirstLine == 0) {
				g.FirstLine = s.Line
			}
			if s.Line > g.LastLine {
				g.LastLine = s.Line
			}
		}
	}
	// Sort groups by descending count.
	sort.Slice(order, func(i, j int) bool { return m[order[i]].Count > m[order[j]].Count })
	var gi []GroupedWorkItem
	for _, k := range order {
		gi = append(gi, *m[k])
	}
	renderItems(b, gi, "    ")
}

// renderCategoryRollup prints the top categories by count, e.g.
// "  Top gaps: module (3,024) · dynamic (2,111) · import (950)".
func renderCategoryRollup(b *strings.Builder, cats []CategoryCount) {
	if len(cats) == 0 {
		return
	}
	var parts []string
	for _, c := range cats {
		parts = append(parts, fmt.Sprintf("%s (%d)", c.Category, c.Count))
	}
	b.WriteString("  Top gaps: " + strings.Join(parts, " · ") + "\n")
}

// renderItems writes grouped work items sorted by first TS line.
func renderItems(b *strings.Builder, items []GroupedWorkItem, indent string) {
	sorted := append([]GroupedWorkItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].FirstLine < sorted[j].FirstLine })
	for _, it := range sorted {
		line := "-"
		if it.FirstLine > 0 {
			line = fmt.Sprint(it.FirstLine)
			if it.FirstLine != it.LastLine {
				line = fmt.Sprintf("%d-%d", it.FirstLine, it.LastLine)
			}
		}
		fmt.Fprintf(b, "%s%5s  [%-6s] x%-4d %-12s %s", indent, line, it.Severity, it.Count, it.Category, it.Message)
		if it.Snippet != "" {
			fmt.Fprintf(b, "  | %s", it.Snippet)
		}
		b.WriteString("\n")
	}
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

// baseName strips directories from a path for compact manifest lines.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}
