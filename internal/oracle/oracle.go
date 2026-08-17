// Package oracle implements the parity gate: transpile a JS module and its
// test to Go, run the test's assertions, and report pass/fail against the
// JS baseline. This is the Corsa "one number" health metric applied to
// transpiled output.
//
// v1 scope: synchronous, pure-function slices (no fs/async), one module +
// one test file per run, chai assert style.
package oracle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Result is the parity outcome for one module+test pair.
type Result struct {
	Module string
	Test   string

	GoPass, GoFail int
	JSPass, JSFail int

	GoBuildOK bool
	GoBuildEr string
	GoRunOut  string
	JSOut     string
}

// ParityOK reports whether Go results match the JS baseline.
func (r *Result) ParityOK() bool {
	return r.GoPass == r.JSPass && r.GoFail == r.JSFail && r.JSPass > 0
}

// Summary renders the one-line parity verdict.
func (r *Result) Summary() string {
	if !r.GoBuildOK {
		return fmt.Sprintf("%s: GO BUILD FAILED: %s", filepath.Base(r.Module), oneLine(r.GoBuildEr))
	}
	return fmt.Sprintf("%s: JS %d/%d, Go %d/%d %s",
		filepath.Base(r.Module), r.JSPass, r.JSPass+r.JSFail, r.GoPass, r.GoPass+r.GoFail,
		map[bool]string{true: "PARITY", false: "DIVERGED"}[r.ParityOK()])
}

// Run executes the full oracle pipeline for one module+test pair.
func Run(module, test, workDir string) (*Result, error) {
	r := &Result{Module: module, Test: test}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return r, err
	}

	// 1. JS baseline with the stub chai + driver.
	jsdir := filepath.Join(workDir, "js")
	if err := os.MkdirAll(jsdir, 0o755); err != nil {
		return r, err
	}
	writeFile(filepath.Join(jsdir, "chai.js"), stubChai)
	writeFile(filepath.Join(jsdir, "driver.js"), driverJS)
	// node resolves require("chai") from the test's directory upward; the
	// stub must live where the test looks for it.
	if dir := filepath.Dir(test); dir != jsdir {
		writeFile(filepath.Join(dir, "node_modules", "chai", "index.js"), stubChai)
		writeFile(filepath.Join(dir, "node_modules", "chai", "package.json"), `{"name":"chai","main":"index.js"}`)
	}
	out, _ := runCmd(jsdir, "node", "driver.js", test)
	r.JSOut = out
	r.JSPass, r.JSFail = parseOracleCounts(out, "ORACLE JS:")

	// 2. Lift module + test to TS (module keeps its basename so the default
	// export derives the right Go name: interpolate.js -> Interpolate).
	modTS := filepath.Join(workDir, basenameAsTS(module))
	if o, err := runCmd("", "/tmp/ts2go", "lift", "-o", modTS, module); err != nil {
		return r, fmt.Errorf("lift module: %v\n%s", err, o)
	}
	testTS := filepath.Join(workDir, "ztest.ts")
	if o, err := runCmd("", "/tmp/ts2go", "lift", "-o", testTS, test); err != nil {
		return r, fmt.Errorf("lift test: %v\n%s", err, o)
	}

	// 3. Transpile both to one Go package.
	godir := filepath.Join(workDir, "go")
	_ = os.RemoveAll(godir)
	if err := os.MkdirAll(godir, 0o755); err != nil {
		return r, err
	}
	if o, err := runCmd("", "/tmp/ts2go", "-o", filepath.Join(godir, basenameAsGo(module)), modTS); err != nil {
		r.GoBuildEr = o
		return r, nil
	}
	if o, err := runCmd("", "/tmp/ts2go", "-o", filepath.Join(godir, "ztest.go"), testTS); err != nil {
		r.GoBuildEr = o
		return r, nil
	}

	// 4. Fix-up pass: map test scaffolding to harness helpers.
	fixUp(filepath.Join(godir, "ztest.go"), filepath.Base(module))

	// 5. Harness + build + run.
	writeFile(filepath.Join(godir, "go.mod"), "module oracle\n\ngo 1.22\n")
	writeFile(filepath.Join(godir, "harness_main.go"), harnessGo)
	if o, err := runCmd(godir, "go", "build", "-o", "oracle.bin", "."); err != nil {
		r.GoBuildEr = o
		return r, nil
	}
	r.GoBuildOK = true
	out, _ = runCmd(godir, filepath.Join(godir, "oracle.bin"))
	r.GoRunOut = out
	r.GoPass, r.GoFail = parseOracleCounts(out, "ORACLE GO:")
	return r, nil
}

// fixUp rewrites the transpiled test file's scaffolding calls to the
// harness: describe/it → oracleDescribe/oracleIt (statement position), the
// dropped CJS default-import name → alias to the module's exported symbol,
// and `func() any` scaffolding callbacks → `func()` (the harness ignores
// return values; Go demands a return from `func() any` bodies).
func fixUp(path, moduleBase string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	s := string(b)
	// Scaffolding, only at statement position (line start + indent).
	s = regexp.MustCompile(`(?m)^(\s*)describe\(`).ReplaceAllString(s, "${1}oracleDescribe(")
	s = regexp.MustCompile(`(?m)^(\s*)it\(`).ReplaceAllString(s, "${1}oracleIt(")
	// Callbacks passed to the scaffolding: value-returning form → plain.
	s = strings.ReplaceAll(s, "func() any {", "func() {")
	// Alias the module's default export (exported Go name) to the lowercase
	// local name the test uses: var interpolate = Interpolate.
	name := strings.TrimSuffix(moduleBase, filepath.Ext(moduleBase))
	if name == "" {
		writeFile(path, s)
		return
	}
	alias := "var " + name + " = " + strings.ToUpper(name[:1]) + name[1:]
	if !strings.Contains(s, alias) {
		// Insert after the import block when present, else after package.
		switch {
		case strings.Contains(s, "import ("):
			if i := strings.LastIndex(s, ")\n"); i >= 0 {
				s = s[:i+2] + "\n" + alias + "\n" + s[i+2:]
			}
		case strings.HasPrefix(s, "//"):
			// Header comments: insert after the package line.
			if i := strings.Index(s, "\npackage "); i >= 0 {
				if j := strings.Index(s[i+1:], "\n"); j >= 0 {
					at := i + 1 + j + 1
					s = s[:at] + "\n" + alias + "\n" + s[at:]
				}
			}
		}
	}
	writeFile(path, s)
}

// parseOracleCounts extracts "N pass, M fail" after the marker.
func parseOracleCounts(out, marker string) (int, int) {
	i := strings.Index(out, marker)
	if i < 0 {
		return 0, 0
	}
	var p, f int
	fmt.Sscanf(out[i:], marker+" %d pass, %d fail", &p, &f)
	return p, f
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// basenameAsTS maps foo.js -> foo.ts (keeping the basename).
func basenameAsTS(p string) string {
	b := filepath.Base(p)
	return strings.TrimSuffix(b, filepath.Ext(b)) + ".ts"
}

// basenameAsGo maps foo.js -> foo.go.
func basenameAsGo(p string) string {
	return strings.TrimSuffix(basenameAsTS(p), ".ts") + ".go"
}

func writeFile(path, content string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(content), 0o644)
}

func runCmd(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
