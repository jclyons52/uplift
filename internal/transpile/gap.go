package transpile

import (
	"fmt"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
)

// Severity classifies how much LLM post-work a gap requires.
type Severity string

const (
	// SevBanned: construct with no idiomatic Go equivalent (prototype tricks,
	// eval, Proxy). Needs redesign, not just translation.
	SevBanned Severity = "banned"
	// SevTodo: could not be translated; a compiling placeholder was emitted
	// in its place. Must be filled in.
	SevTodo Severity = "todo"
	// SevApprox: translated, but semantics are approximate (e.g. try/catch
	// via recover, `??` widened to any). Verify by hand.
	SevApprox Severity = "approx"
)

// WorkItem is one piece of LLM post-work: what, where in the TS source, and
// how severe.
type WorkItem struct {
	Severity Severity
	Category string // coarse bucket: async, union, dynamic, expression, ...
	Line     int    // 1-based line in the TS source
	Message  string
	Snippet  string // short TS excerpt for context
}

// weight is the LLM-effort weight used to score complexity.
func (w WorkItem) weight() int {
	switch w.Severity {
	case SevBanned:
		return 10
	case SevTodo:
		return 3
	default:
		return 1
	}
}

// recordGap appends a work item to the report. Never fails — gap collection
// must not abort transpilation.
func (tr *transpiler) recordGap(sev Severity, category string, n tsmorph.Node, format string, args ...any) {
	item := WorkItem{
		Severity: sev,
		Category: category,
		Line:     tr.lineOf(n),
		Message:  fmt.Sprintf(format, args...),
		Snippet:  oneLine(snippetLong(n)),
	}
	tr.items = append(tr.items, item)
}

// lineOf returns the 1-based TS source line of a node.
func (tr *transpiler) lineOf(n tsmorph.Node) int {
	if n.IsZero() {
		return 0
	}
	return lineAt(tr.sf.Text(), n.Pos())
}

// snippetLong is snippet() with a longer budget for report context.
func snippetLong(n tsmorph.Node) string {
	s := strings.ReplaceAll(n.Text(), "\n", " ")
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

// oneLine collapses whitespace so a snippet fits on one comment line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// placeholderExpr returns a compiling Go expression standing in for an
// untranslated TS expression, with the TODO inline. Inner comment markers
// are stripped so placeholders can nest.
func placeholderExpr(msg string) string {
	msg = strings.ReplaceAll(msg, "/*", "(")
	msg = strings.ReplaceAll(msg, "*/", ")")
	return "any(nil) /* TODO(ts2go): " + msg + " */"
}

// condExpr coerces a placeholder that landed in boolean-condition position
// (if/while/for): `any(nil)` is not a bool in Go.
func condExpr(s string) string {
	if strings.HasPrefix(s, "any(nil)") {
		return "false" + strings.TrimPrefix(s, "any(nil)")
	}
	return s
}

// placeholderComment extracts "// TODO(ts2go): ..." from a placeholder
// expression, for use on a declaration the placeholder cannot initialize
// (e.g. `var p string = any(nil)` is not assignable in Go). Returns "" for
// non-placeholder expressions.
func placeholderComment(expr string) string {
	if !strings.HasPrefix(expr, "any(nil)") {
		return ""
	}
	if i := strings.Index(expr, "TODO(ts2go):"); i >= 0 {
		msg := strings.TrimSuffix(strings.TrimSpace(expr[i+len("TODO(ts2go):"):]), "*/")
		return "// TODO(ts2go): " + strings.TrimSpace(msg)
	}
	return "// TODO(ts2go): untranslated expression"
}

// markUsedIfUnused appends `_ = name` when a local variable appears unused
// in the rest of the file — TS allows unused locals, Go does not. Emitting
// it unconditionally would be safe but noisy, so use a cheap occurrence
// count over the remaining source text.
func (tr *transpiler) markUsedIfUnused(name string, n tsmorph.Node) {
	if len(tr.retStack) == 0 || name == "" || name == "_" {
		return // top-level (package scope): unused is legal in Go
	}
	rest := tr.sf.Text()[min(n.End(), len(tr.sf.Text())):]
	if strings.Count(rest, name) == 0 {
		tr.out.line("_ = " + name)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// emitTodoStatement writes a compiling statement-level placeholder plus the
// original TS snippet as context for the LLM. It is panic-safe: if even the
// placeholder machinery trips over the node, a bare `_ = 0` line still
// lands so the file keeps compiling.
func (tr *transpiler) emitTodoStatement(n tsmorph.Node, format string, args ...any) {
	defer func() {
		if recover() != nil {
			tr.out.line("_ = 0 // TODO(ts2go): untranslated statement")
		}
	}()
	msg := fmt.Sprintf(format, args...)
	tr.recordGap(SevTodo, "statement", n, "%s", msg)
	tr.out.line("// TODO(ts2go): " + msg)
	if s := oneLine(snippetLong(n)); s != "" {
		tr.out.line("//   TS: " + s)
	}
	tr.out.line("_ = 0")
}

// todoExpr returns a placeholder expression and records the gap.
func (tr *transpiler) todoExpr(n tsmorph.Node, format string, args ...any) string {
	tr.recordGap(SevTodo, "expression", n, "%s", fmt.Sprintf(format, args...))
	return placeholderExpr(fmt.Sprintf(format, args...))
}

// approxExpr returns the translated expression but records that its
// semantics are approximate.
func (tr *transpiler) approxExpr(n tsmorph.Node, category, format string, args ...any) string {
	tr.recordGap(SevApprox, category, n, format, args...)
	return ""
}
