package measure

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a.ts": `export function add(x: number, y: number): number {
  let total = 0;
  for (let i = 0; i < 10; i++) {
    if (x > 0 && y > 0) { total += 1; }
  }
  return total;
}
export const k = 1;
`,
		"b.ts": `import { add } from "./a";
export function raw(x, y) {  // untyped
  switch (x) { case 1: break; default: break; }
  return x ? y : null;
}
`,
		"c_test.go": `package measure
import "testing"
func TestParity(t *testing.T) {}
`,
	}
	for p, c := range files {
		full := filepath.Join(dir, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
	}
	return dir
}

func TestAnalyze(t *testing.T) {
	m, err := Analyze(writeFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if m.Totals.Files < 2 {
		t.Errorf("expected >=2 files, got %d", m.Totals.Files)
	}
	// a.ts has typed params+return+const (5 annotated-ish); b.ts untyped.
	if m.TypeCoverage.Annotated == 0 {
		t.Errorf("expected some annotated sites, got 0")
	}
	if m.TypeCoverage.Unannotated == 0 {
		t.Errorf("expected some unannotated sites (b.ts), got 0")
	}
	if m.TypeCoverage.Ratio <= 0 || m.TypeCoverage.Ratio >= 1 {
		t.Errorf("ratio out of range: %v", m.TypeCoverage.Ratio)
	}
	// complexity: a.ts has for + if + && -> >=2; b.ts has switch + ternary -> >0
	if m.Complexity.Total == 0 {
		t.Errorf("expected non-zero cyclomatic complexity")
	}
	// parity test file counted
	if m.Tests.ParitySuites != 1 {
		t.Errorf("expected 1 parity suite, got %d", m.Tests.ParitySuites)
	}
	// coupling: b imports a -> an edge
	if m.Coupling.Edges < 1 {
		t.Errorf("expected >=1 coupling edge (b->a), got %d", m.Coupling.Edges)
	}
}

func TestCompare(t *testing.T) {
	low := &Metrics{SchemaVersion: "m", TypeCoverage: TypeCoverage{Annotated: 0, Unannotated: 100, Ratio: 0}}
	high := &Metrics{SchemaVersion: "m", TypeCoverage: TypeCoverage{Annotated: 60, Unannotated: 40, Ratio: 0.6}}
	d := Compare(low, high)
	if d.TypeAnnotatedDelta != 60 {
		t.Errorf("type delta = %d, want 60", d.TypeAnnotatedDelta)
	}
	if d.TypeRatioDelta != 0.6 {
		t.Errorf("ratio delta = %v, want 0.6", d.TypeRatioDelta)
	}
}
