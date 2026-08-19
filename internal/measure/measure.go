// Package measure computes a per-codebase quality metric set so uplift
// progress is trackable ("measure or it didn't happen"). It produces a
// versioned, comparable JSON report: type coverage, cyclomatic complexity,
// coupling (from the module graph), and test/parity coverage. Re-run after
// each uplift stage to show the before/after delta.
package measure

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
	"github.com/jclyons52/ts2go/internal/deps"
)

// SchemaVersion marks the metric definitions so different report versions
// are never compared blindly.
const SchemaVersion = "measure/v1"

// Metrics is the full report.
type Metrics struct {
	SchemaVersion string              `json:"schemaVersion"`
	Root          string              `json:"root"`
	Totals        Totals              `json:"totals"`
	TypeCoverage  TypeCoverage        `json:"typeCoverage"`
	Complexity    complexityReport    `json:"complexity"`
	Coupling      deps.CouplingReport `json:"coupling"`
	Tests         Tests               `json:"tests"`
}

// Totals are basic size numbers.
type Totals struct {
	Files int `json:"files"`
	Loc   int `json:"loc"`
}

// TypeCoverage is the fraction of declaration sites that carry a type
// annotation (params, returns, variables). For JS this is ~0 before lift and
// rises after, making it the "types filled" progress signal.
type TypeCoverage struct {
	Annotated   int         `json:"annotated"`
	Unannotated int         `json:"unannotated"`
	Ratio       float64     `json:"ratio"`
	ByFile      []FileSites `json:"byFile,omitempty"` // per-file, worst-first
}

// FileSites is one file's annotation counts.
type FileSites struct {
	File        string `json:"file"`
	Annotated   int    `json:"annotated"`
	Unannotated int    `json:"unannotated"`
}

// complexityReport aggregates per-file cyclomatic complexity.
type complexityReport struct {
	Total int         `json:"total"`
	Avg   float64     `json:"avg"`
	Max   int         `json:"max"`
	Top   []FileScore `json:"topFiles"`
}

// FileScore is one file's complexity / loc.
type FileScore struct {
	File       string `json:"file"`
	Loc        int    `json:"loc"`
	Cyclomatic int    `json:"cyclomatic"`
}

// Tests describes what is under test.
type Tests struct {
	TestFiles    int `json:"testFiles"`
	ParitySuites int `json:"paritySuites"`
}

// Analyze computes metrics for a source root.
func Analyze(root string) (*Metrics, error) {
	m := &Metrics{
		SchemaVersion: SchemaVersion,
		Root:          root,
	}
	abs, _ := filepath.Abs(root)

	// Collect source files (JS/TS/Go), skipping node_modules and .d.ts.
	var srcs []string
	filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".ts") && !strings.HasSuffix(p, ".d.ts") ||
			strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs") ||
			strings.HasSuffix(p, ".cjs") || strings.HasSuffix(p, ".tsx") {
			srcs = append(srcs, p)
		}
		return nil
	})
	sort.Strings(srcs)
	// A .js file superseded by a same-basename .ts/.tsx is not counted twice
	// (lift produces the typed twin in place).
	srcs = dedupeSuperseded(srcs)

	proj, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		return nil, err
	}

	tc := &m.TypeCoverage
	cp := &m.Complexity
	for _, f := range srcs {
		code, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel := relname(abs, f)
		lines := nonEmptyLines(code)
		m.Totals.Files++
		m.Totals.Loc += lines

		sf := createSourceFile(proj, f, code)
		if sf == nil {
			continue
		}
		cyc, ann, unann := analyzeFile(sf)
		cp.Total += cyc
		cp.Max = maxInt(cp.Max, cyc)
		cp.Top = append(cp.Top, FileScore{File: rel, Loc: lines, Cyclomatic: cyc})
		tc.Annotated += ann
		tc.Unannotated += unann
		tc.ByFile = append(tc.ByFile, FileSites{File: rel, Annotated: ann, Unannotated: unann})
	}
	if tc.Annotated+tc.Unannotated > 0 {
		tc.Ratio = float64(tc.Annotated) / float64(tc.Annotated+tc.Unannotated)
	}
	// worst-first for the "lift these modules next" ranking
	sort.Slice(tc.ByFile, func(i, j int) bool {
		if tc.ByFile[i].Unannotated != tc.ByFile[j].Unannotated {
			return tc.ByFile[i].Unannotated > tc.ByFile[j].Unannotated
		}
		return tc.ByFile[i].File < tc.ByFile[j].File
	})
	if m.Totals.Files > 0 {
		cp.Avg = float64(cp.Total) / float64(m.Totals.Files)
	}
	sort.Slice(cp.Top, func(i, j int) bool { return cp.Top[i].Cyclomatic > cp.Top[j].Cyclomatic })
	if len(cp.Top) > 10 {
		cp.Top = cp.Top[:10]
	}

	// Coupling from the module graph.
	res, err := deps.Analyze(root)
	if err == nil {
		m.Coupling = deps.CouplingOf(res)
	}

	// Tests: Go _test files and parity suites (TestParity).
	filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, "_test.go") {
			m.Tests.TestFiles++
			if b, err := os.ReadFile(p); err == nil {
				if strings.Contains(string(b), "TestParity") {
					m.Tests.ParitySuites++
				}
			}
		}
		return nil
	})

	return m, nil
}

func analyzeFile(sf *tsmorph.SourceFile) (cyc, ann, unann int) {
	// cyclomatic decision points
	for _, k := range []ast.Kind{
		ast.KindIfStatement, ast.KindForStatement, ast.KindWhileStatement,
		ast.KindDoStatement, ast.KindCaseClause, ast.KindCatchClause,
		ast.KindConditionalExpression, ast.KindForInStatement, ast.KindForOfStatement,
	} {
		cyc += len(sf.DescendantsOfKind(k))
	}
	// short-circuit operators && and ||
	for _, n := range sf.DescendantsOfKind(ast.KindBinaryExpression) {
		if bin, ok := n.AsBinaryExpression(); ok {
			op := bin.OperatorText()
			if op == "&&" || op == "||" {
				cyc++
			}
		}
	}

	// type coverage: functions (params + return) and variable declarations
	for _, fn := range sf.Functions() {
		if _, has := fn.ReturnTypeNode(); has {
			ann++
		} else {
			unann++
		}
		for _, p := range fn.Parameters() {
			if _, has := p.TypeNode(); has {
				ann++
			} else {
				unann++
			}
		}
	}
	for _, vd := range sf.VariableDeclarations() {
		if _, has := vd.TypeNode(); has {
			ann++
		} else {
			unann++
		}
	}
	return cyc, ann, unann
}

// createSourceFile parses a source file, renaming .js to .ts for the parser.
func createSourceFile(p *tsmorph.Project, path string, code []byte) *tsmorph.SourceFile {
	virt := filepath.ToSlash(path)
	ext := filepath.Ext(virt)
	if ext != ".ts" && ext != ".tsx" {
		base := filepath.Base(virt)
		virt = filepath.ToSlash(filepath.Join(filepath.Dir(path),
			strings.TrimSuffix(base, ext)+".ts"))
	}
	return p.CreateSourceFile(virt, string(code))
}

func relname(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// dedupeSuperseded drops a .js/.mjs/.cjs file when a same-basename .ts/.tsx
// twin exists (the typed version supersedes it after lift).
func dedupeSuperseded(files []string) []string {
	var superseded map[string]bool
	for _, f := range files {
		if strings.HasSuffix(f, ".ts") || strings.HasSuffix(f, ".tsx") {
			base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			if superseded == nil {
				superseded = map[string]bool{}
			}
			superseded[filepath.Join(filepath.Dir(f), base)] = true
		}
	}
	if superseded == nil {
		return files
	}
	var out []string
	for _, f := range files {
		if strings.HasSuffix(f, ".js") || strings.HasSuffix(f, ".mjs") || strings.HasSuffix(f, ".cjs") {
			base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			if superseded[filepath.Join(filepath.Dir(f), base)] {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

func nonEmptyLines(code []byte) int {
	n := 0
	for _, line := range strings.Split(string(code), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
