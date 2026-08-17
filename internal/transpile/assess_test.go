package transpile

import (
	"strings"
	"testing"

	tsmorph "github.com/jclyons52/ts-go-morph"
)

// reportWith builds a Report with the given items (callers can also append
// to Items directly).
func reportWith(items ...WorkItem) *Report {
	return &Report{Input: "a.ts", Items: items}
}

func TestGroupedItemsCollapse(t *testing.T) {
	r := reportWith(
		WorkItem{Severity: SevTodo, Category: "module", Message: "call to foo", Line: 1},
		WorkItem{Severity: SevTodo, Category: "module", Message: "call to foo", Line: 10},
		WorkItem{Severity: SevTodo, Category: "module", Message: "call to foo", Line: 5},
		WorkItem{Severity: SevApprox, Category: "module", Message: "call to foo", Line: 2}, // different severity
		WorkItem{Severity: SevTodo, Category: "dynamic", Message: "call to bar", Line: 3},
	)
	g := r.GroupedItems()
	if len(g) != 3 {
		t.Fatalf("want 3 distinct groups, got %d: %+v", len(g), g)
	}
	// The todo/module/call-to-foo group has count 3, first 1, last 10.
	found := false
	for _, it := range g {
		if it.Category == "module" && it.Severity == SevTodo && it.Message == "call to foo" {
			if it.Count != 3 || it.FirstLine != 1 || it.LastLine != 10 {
				t.Errorf("module/foo group = %+v, want count 3 first 1 last 10", it)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("expected the collapsed module/foo group, got %+v", g)
	}
}

func TestManifestGrouped(t *testing.T) {
	r := reportWith(
		WorkItem{Severity: SevTodo, Category: "module", Message: "call to foo", Line: 1, Snippet: "foo(1)"},
		WorkItem{Severity: SevTodo, Category: "module", Message: "call to foo", Line: 12, Snippet: "foo(2)"},
	)
	m := r.Manifest()
	if !strings.Contains(m, "x2 call to foo") {
		t.Errorf("manifest should show x2 count:\n%s", m)
	}
	if !strings.Contains(m, "a.ts:1-12") {
		t.Errorf("manifest should show the line range:\n%s", m)
	}
}

func TestTopCategories(t *testing.T) {
	r := reportWith(
		WorkItem{Severity: SevTodo, Category: "module", Message: "m1"},
		WorkItem{Severity: SevTodo, Category: "module", Message: "m2"},
		WorkItem{Severity: SevTodo, Category: "module", Message: "m3"},
		WorkItem{Severity: SevApprox, Category: "dynamic", Message: "d1"},
		WorkItem{Severity: SevBanned, Category: "banned", Message: "b1"},
	)
	top := r.TopCategories(2)
	if len(top) != 2 {
		t.Fatalf("want 2 top categories, got %d", len(top))
	}
	if top[0].Category != "module" || top[0].Count != 3 {
		t.Errorf("top category should be module x3, got %+v", top[0])
	}
	if top[1].Category != "dynamic" && top[1].Category != "banned" {
		t.Errorf("second category should be dynamic or banned, got %+v", top[1])
	}
}

// transpileReport runs the pipeline and returns both the report and source.
func transpileReport(t *testing.T, src string) *Report {
	t.Helper()
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		t.Fatal(err)
	}
	sf := p.CreateSourceFile("/a.ts", src)
	tp := NewTranspiler(p, sf)
	if _, err := tp.Transpile(); err != nil {
		t.Fatal(err)
	}
	return tp.Report()
}

// TestTypeAuditFindsTightenableParam: a default-valued param (foo = true)
// has no annotation, but the checker infers bool — a tightenable site.
func TestTypeAuditFindsTightenableParam(t *testing.T) {
	r := transpileReport(t, `function f(foo = true): number { return 1; }`)
	if len(r.Audit) == 0 {
		t.Fatal("expected a tighten site for the default-valued param")
	}
	if r.Audit[0].Category != "param" {
		t.Errorf("category should be param, got %q", r.Audit[0].Category)
	}
	if r.Audit[0].CheckerType != "bool" {
		t.Errorf("checker should resolve the param to bool, got %q (site %+v)", r.Audit[0].CheckerType, r.Audit[0])
	}
	if !strings.Contains(r.AuditString(), "tightenable") {
		t.Errorf("AuditString should render a header")
	}
}

// TestTypeAuditCleanWhenTyped: fully annotated code has no tighten sites.
func TestTypeAuditCleanWhenTyped(t *testing.T) {
	r := transpileReport(t, `function f(foo: boolean): number { return 1; }`)
	for _, s := range r.Audit {
		if s.Category == "param" {
			t.Errorf("annotated param should not be a tighten site: %+v", s)
		}
	}
}
