// Package uplift is the product-surface command: given a codebase, it computes
// the quality metrics and turns them into a prioritized list of what to do
// next (lift types, simplify, decouple, add tests, port leaves). It emits a
// stable machine-readable report (schema uplift/v1) so both a human and an
// agent can consume the same truth.
package uplift

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jclyons52/uplift/internal/deps"
	"github.com/jclyons52/uplift/internal/measure"
)

// SchemaVersion identifies the report shape.
const SchemaVersion = "uplift/v1"

// Report is the full uplift status for a codebase.
type Report struct {
	SchemaVersion string              `json:"schemaVersion"`
	Root          string              `json:"root"`
	Metrics       measure.Metrics     `json:"metrics"`
	Coupling      deps.CouplingReport `json:"coupling"`
	Gaps          []Gap               `json:"gaps"`
	Actions       []Action            `json:"actions"`
	PortBacklog   []PortItem          `json:"portBacklog,omitempty"`
}

// Gap is a category with a problem and severity.
type Gap struct {
	Category string `json:"category"`
	Severity string `json:"severity"` // high | medium | low
	Detail   string `json:"detail"`
}

// Action is one recommended step.
type Action struct {
	Rank   int    `json:"rank"`
	Stage  string `json:"stage"` // types | structure | decouple | tests | port
	Kind   string `json:"kind"`  // lift | simplify | add-test | hub | port
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
}

// PortItem is a leaf library recommended for porting.
type PortItem struct {
	Name     string `json:"name"`
	GoModule string `json:"goModule"`
	Loc      int    `json:"loc"`
}

// Analyze builds the uplift report for a source root.
func Analyze(root string) (*Report, error) {
	m, err := measure.Analyze(root)
	if err != nil {
		return nil, err
	}
	rep := &Report{SchemaVersion: SchemaVersion, Root: m.Root, Metrics: *m}

	res, err := deps.Analyze(root)
	if err == nil {
		rep.Coupling = deps.CouplingOf(res)
		// port backlog: leaf libs the analysis recommends separating
		for _, name := range res.Order {
			n := res.Nodes[name]
			if n == nil || n.Kind != deps.KindExternal {
				continue
			}
			if wantsOwnRepo(res.Recommend(name)) {
				rep.PortBacklog = append(rep.PortBacklog, PortItem{Name: name, Loc: n.Loc, GoModule: "github.com/jclyons52/" + sanitize(name) + "-go"})
			}
		}
		sort.Slice(rep.PortBacklog, func(i, j int) bool { return rep.PortBacklog[i].Loc > rep.PortBacklog[j].Loc })
	} else {
		rep.Coupling = deps.CouplingReport{}
	}

	rep.computeGaps()
	rep.computeActions()
	return rep, nil
}

func wantsOwnRepo(rec string) bool {
	return strings.Contains(rec, "own repo") || strings.Contains(rec, "port as leaf")
}

func sanitize(name string) string {
	name = strings.TrimPrefix(name, "@")
	var b strings.Builder
	lastDash := true
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// computeGaps derives the problem list from the metrics.
func (r *Report) computeGaps() {
	tc := r.Metrics.TypeCoverage
	if tc.Unannotated > 0 {
		sev := "low"
		if len(tc.ByFile) == 0 || tc.Ratio < 0.5 {
			sev = "high"
		}
		r.Gaps = append(r.Gaps, Gap{
			Category: "types", Severity: sev,
			Detail: fmt.Sprintf("%d/%d declaration sites untyped (%.0f%% coverage)", tc.Unannotated, tc.Annotated+tc.Unannotated, tc.Ratio*100),
		})
	}
	if r.Metrics.Tests.ParitySuites == 0 && r.Metrics.Tests.TestFiles == 0 {
		r.Gaps = append(r.Gaps, Gap{Category: "tests", Severity: "high", Detail: "no test files / parity suites"})
	} else if r.Metrics.Tests.ParitySuites == 0 {
		r.Gaps = append(r.Gaps, Gap{Category: "tests", Severity: "medium", Detail: "no parity suites"})
	}
	if r.Coupling.Cycles > 0 {
		r.Gaps = append(r.Gaps, Gap{Category: "coupling", Severity: "medium",
			Detail: fmt.Sprintf("%d module cycle(s), %d node(s)", r.Coupling.Cycles, r.Coupling.NodesInCycles)})
	}
	if len(r.PortBacklog) == 0 {
		r.Gaps = append(r.Gaps, Gap{Category: "ports", Severity: "low", Detail: "no port backlog (all externals covered by Go)"})
	}
}

// computeActions turns the worst offenders into ranked next steps.
func (r *Report) computeActions() {
	rank := 0
	add := func(stage, kind, target, reason string) {
		rank++
		r.Actions = append(r.Actions, Action{Rank: rank, Stage: stage, Kind: kind, Target: target, Reason: reason})
	}

	// types: basic is types are the safest lever; lift the worst untyped files.
	if len(r.Metrics.TypeCoverage.ByFile) > 0 {
		worst := r.Metrics.TypeCoverage.ByFile[:min(3, len(r.Metrics.TypeCoverage.ByFile))]
		// only lift files that actually have untyped sites
		n := 0
		for _, f := range worst {
			if f.Unannotated == 0 {
				continue
			}
			add("types", "lift", f.File, fmt.Sprintf("%d untyped declaration sites", f.Unannotated))
			if n++; n >= 3 {
				break
			}
		}
	}

	// structure: simplify the highest-complexity files.
	for i, f := range r.Metrics.Complexity.Top {
		if i >= 2 {
			break
		}
		if f.Cyclomatic < 30 {
			break
		}
		add("structure", "simplify", f.File, fmt.Sprintf("cyclomatic complexity %d", f.Cyclomatic))
	}

	// decouple: the top hubs (cap at a few so tests/port still get shown).
	hubCount := 0
	for _, h := range r.Coupling.Hubs {
		if h.FanOut < 5 || hubCount >= 3 {
			break
		}
		add("decouple", "hub", h.Module, fmt.Sprintf("fan-out %d", h.FanOut))
		hubCount++
	}

	// tests: parity suites for ported libs.
	if r.Metrics.Tests.ParitySuites == 0 && len(r.PortBacklog) > 0 {
		add("tests", "add-test", "parity", "no parity suites; add one per ported library")
	}

	// port: top backlog leaves by LOC (only when there is real work).
	for _, p := range r.PortBacklog {
		if p.Loc < 50 {
			continue
		}
		add("port", "port", p.Name, fmt.Sprintf("%d LOC leaf; no Go counterpart", p.Loc))
	}
	if len(r.PortBacklog) == 0 && len(r.Actions) > 0 {
		// nothing to port but other work remains
	}

	// cap actions at a readable number
	if len(r.Actions) > 12 {
		r.Actions = r.Actions[:12]
	}
}

// Render prints a human-friendly status + next actions.
func (r *Report) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "uplift status (schema %s) — %s\n\n", r.SchemaVersion, r.Root)
	m := r.Metrics
	fmt.Fprintf(&b, "  %-14s %d files, %d LOC\n", "size", m.Totals.Files, m.Totals.Loc)
	fmt.Fprintf(&b, "  %-14s %d/%d sites (%.0f%%)\n", "type coverage", m.TypeCoverage.Annotated, m.TypeCoverage.Annotated+m.TypeCoverage.Unannotated, m.TypeCoverage.Ratio*100)
	fmt.Fprintf(&b, "  %-14s total %d, avg %.1f, max %d\n", "complexity", m.Complexity.Total, m.Complexity.Avg, m.Complexity.Max)
	c := r.Coupling
	fmt.Fprintf(&b, "  %-14s %d modules, %d edges, %d cycle(s)\n", "coupling", c.Modules, c.Edges, c.Cycles)
	fmt.Fprintf(&b, "  %-14s %d test files, %d parity suites, %d port backlog\n", "tests+ports", m.Tests.TestFiles, m.Tests.ParitySuites, len(r.PortBacklog))

	if len(r.Gaps) > 0 {
		b.WriteString("\ngaps:\n")
		for _, g := range r.Gaps {
			fmt.Fprintf(&b, "  [%s] %s: %s\n", g.Severity, g.Category, g.Detail)
		}
	}
	if len(r.Actions) > 0 {
		b.WriteString("\nnext actions (highest value first):\n")
		for _, a := range r.Actions {
			fmt.Fprintf(&b, "  %d. [%s] %s %s — %s\n", a.Rank, a.Stage, a.Kind, orDash(a.Target), a.Reason)
		}
	} else {
		b.WriteString("\nno uplift actions — codebase looks clean.\n")
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "(whole repo)"
	}
	return s
}

// WriteJSON writes the structured report.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
