package measure

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Render writes a human-friendly summary of the metrics.
func (m *Metrics) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Measure report (schema %s)\n", m.SchemaVersion)
	fmt.Fprintf(&b, "for %s\n\n", m.Root)

	fmt.Fprintf(&b, "Totals:      %d files, %d LOC\n", m.Totals.Files, m.Totals.Loc)
	fmt.Fprintf(&b, "TypeCover:   %d/%d sites annotated (%.1f%%)\n",
		m.TypeCoverage.Annotated, m.TypeCoverage.Annotated+m.TypeCoverage.Unannotated,
		m.TypeCoverage.Ratio*100)
	fmt.Fprintf(&b, "Complexity:  %d total, avg %.1f, max %d\n",
		m.Complexity.Total, m.Complexity.Avg, m.Complexity.Max)
	if len(m.Complexity.Top) > 0 {
		b.WriteString("  most complex:\n")
		for _, f := range m.Complexity.Top[:min(5, len(m.Complexity.Top))] {
			fmt.Fprintf(&b, "    %-46s cyc=%d loc=%d\n", f.File, f.Cyclomatic, f.Loc)
		}
	}
	c := m.Coupling
	fmt.Fprintf(&b, "Coupling:    %d internal modules, %d edges (density %.3f)\n",
		c.Modules, c.Edges, c.Density)
	fmt.Fprintf(&b, "             avg fan-out %.2f, max fan-out %d, %d cycle(s), %d node(s) in cycles\n",
		c.AvgFanOut, c.MaxFanOut, c.Cycles, c.NodesInCycles)
	if len(c.Hubs) > 0 {
		b.WriteString("  top hubs:\n")
		for _, h := range c.Hubs[:min(5, len(c.Hubs))] {
			fmt.Fprintf(&b, "    %-46s out=%d in=%d\n", h.Module, h.FanOut, h.FanIn)
		}
	}
	fmt.Fprintf(&b, "Tests:       %d test files, %d parity suites\n", m.Tests.TestFiles, m.Tests.ParitySuites)

	ready := m.Totals.Files > 0 && (m.Totals.Loc > 0)
	_ = ready
	return b.String()
}

// WriteJSON writes the structured report.
func (m *Metrics) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

// CompareDeltas is the difference between two metric snapshots (before/after
// an uplift stage). Fields are new-old so positive = improvement where noted.
type CompareDeltas struct {
	Before, After      Metrics `json:"-"`
	Loc                int     `json:"locDelta"`
	FileDelta          int     `json:"filesDelta"`
	TypeRatioDelta     float64 `json:"typeCoverageRatioDelta"` // up = good
	TypeAnnotatedDelta int     `json:"typeAnnotatedDelta"`
	ComplexityDelta    int     `json:"complexityDelta"` // down = good
	EdgesDelta         int     `json:"edgesDelta"`      // down = less coupling
	DensityDelta       float64 `json:"densityDelta"`
	CyclesDelta        int     `json:"cyclesDelta"`
	TestFilesDelta     int     `json:"testFilesDelta"`
	ParityDelta        int     `json:"paritySuitesDelta"`
}

// Compare computes the delta between two snapshots.
func Compare(old, nw *Metrics) *CompareDeltas {
	return &CompareDeltas{
		Before: *old, After: *nw,
		Loc:                nw.Totals.Loc - old.Totals.Loc,
		FileDelta:          nw.Totals.Files - old.Totals.Files,
		TypeRatioDelta:     nw.TypeCoverage.Ratio - old.TypeCoverage.Ratio,
		TypeAnnotatedDelta: nw.TypeCoverage.Annotated - old.TypeCoverage.Annotated,
		ComplexityDelta:    nw.Complexity.Total - old.Complexity.Total,
		EdgesDelta:         nw.Coupling.Edges - old.Coupling.Edges,
		DensityDelta:       nw.Coupling.Density - old.Coupling.Density,
		CyclesDelta:        nw.Coupling.Cycles - old.Coupling.Cycles,
		TestFilesDelta:     nw.Tests.TestFiles - old.Tests.TestFiles,
		ParityDelta:        nw.Tests.ParitySuites - old.Tests.ParitySuites,
	}
}

func sign(d int) string {
	if d > 0 {
		return "+"
	}
	return ""
}

// Render writes the deltas as a human-readable before/after.
func (d *CompareDeltas) Render() string {
	var b strings.Builder
	b.WriteString("Uplift delta (before -> after)\n")
	fmt.Fprintf(&b, "  Type coverage:     %.1f%% -> %.1f%%  (Δ%+.1f pp, %+d sites)  [higher is better]\n",
		d.Before.TypeCoverage.Ratio*100, d.After.TypeCoverage.Ratio*100,
		d.TypeRatioDelta*100, d.TypeAnnotatedDelta)
	fmt.Fprintf(&b, "  Complexity (total): %d -> %d  (Δ%+d)  [lower is better]\n",
		d.Before.Complexity.Total, d.After.Complexity.Total, d.ComplexityDelta)
	fmt.Fprintf(&b, "  Coupling edges:    %d -> %d  (Δ%+d, density %.3f -> %.3f)  [lower is better]\n",
		d.Before.Coupling.Edges, d.After.Coupling.Edges, d.EdgesDelta,
		d.Before.Coupling.Density, d.After.Coupling.Density)
	fmt.Fprintf(&b, "  Cycles:            %d -> %d  (Δ%+d)  [lower is better]\n",
		d.Before.Coupling.Cycles, d.After.Coupling.Cycles, d.CyclesDelta)
	fmt.Fprintf(&b, "  Tests / parity:    %d -> %d files, %d -> %d parity  [higher is better]\n",
		d.Before.Tests.TestFiles, d.After.Tests.TestFiles,
		d.Before.Tests.ParitySuites, d.After.Tests.ParitySuites)
	fmt.Fprintf(&b, "  LOC: %d -> %d (Δ%+d)\n", d.Before.Totals.Loc, d.After.Totals.Loc, d.Loc)
	return b.String()
}

// WriteDeltaJSON writes the deltas as JSON.
func (d *CompareDeltas) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
