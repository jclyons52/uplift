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

	// Module and test are transpiled by separate invocations, so each may
	// carry a full copy of the auto-emitted jsrt shim -> duplicate
	// definitions in one package. Keep the first, strip the rest.
	dedupeShim(filepath.Join(godir, basenameAsGo(module)), filepath.Join(godir, "ztest.go"))

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
	// Any `func() any {` inside the auto-emitted jsrt shim must NOT be
	// converted (jsrtAsync/jsrtAll really do return `any`); only the
	// describe/it scaffolding callbacks are value-returning in the JS that
	// Go can't accept. Isolate the pre-shim head for the conversions.
	head, tail := s, ""
	if i := strings.Index(s, "// jsrt:"); i >= 0 {
		head, tail = s[:i], s[i:]
	}
	// Scaffolding, only at statement position (line start + indent).
	head = regexp.MustCompile(`(?m)^(\s*)describe\(`).ReplaceAllString(head, "${1}oracleDescribe(")
	head = regexp.MustCompile(`(?m)^(\s*)it\(`).ReplaceAllString(head, "${1}oracleIt(")
	// Callbacks passed to the scaffolding: value-returning form → plain.
	head = strings.ReplaceAll(head, "func() any {", "func() {")
	s = head + tail
	// Alias the module's default export (exported Go name) to the lowercase
	// local name the test uses. Two cases:
	//  - the transpiler kept a cross-package require: `var X = require("...")`
	//    where the target matches this module's basename. Rewire that line's
	//    require to the module's Go symbol so the test's real local binding
	//    (which need NOT equal the basename, e.g. json.js -> `formatter`)
	//    resolves.
	//  - the require was dropped (same package): inject `var <base> = <Upper>`
	//    as before (interpolate-style).
	name := strings.TrimSuffix(moduleBase, filepath.Ext(moduleBase))
	if name == "" {
		writeFile(path, s)
		return
	}
	upperName := goExportName(name)
	reqPat := regexp.MustCompile(`(?m)^(\s*)var (\w+) = require\("([^"]*/` + regexp.QuoteMeta(name) + `)"\)$`)
	if reqPat.MatchString(s) {
		s = reqPat.ReplaceAllString(s, "${1}var ${2} = "+upperName)
	} else {
		alias := "var " + name + " = " + upperName
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
	}
	writeFile(path, s)
}

// goExportName mirrors the transpiler's defaultExportName(): the Go symbol
// derived from a module basename (json-with-metadata -> JsonWithMetadata),
// so the alias fixUp writes matches the exported symbol in the transpiled
// module file exactly.
func goExportName(base string) string {
	var b strings.Builder
	up := true
	for _, r := range base {
		letter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
		digit := r >= '0' && r <= '9'
		switch {
		case letter:
			if up {
				if r >= 'a' && r <= 'z' {
					r -= 'a' - 'A'
				}
				up = false
			}
			b.WriteRune(r)
		case digit:
			if up && b.Len() == 0 {
				b.WriteRune('D')
			}
			b.WriteRune(r)
			up = false
		default:
			up = true
		}
	}
	res := strings.TrimSuffix(b.String(), "_")
	if res == "" {
		res = "Module"
	}
	return res
}

// dedupeShim removes the auto-emitted jsrt shim block (everything from the
// `// jsrt:` marker to EOF — the shim is the last code in a transpiled file,
// trailing compile-finding comments are safe to drop too) from all but the
// first file that carries one. Module and test are independently transpiled,
// so each may contain a full copy; a single Go package needs exactly one.
func dedupeShim(paths ...string) {
	kept := false
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s := string(b)
		i := strings.Index(s, "// jsrt:")
		if i < 0 {
			continue
		}
		if !kept {
			kept = true
			continue
		}
		// Strip the shim block (marker onward; trailing compile-finding
		// comments are safe to drop) and prune imports that are now unused.
		_ = os.WriteFile(p, []byte(pruneImports(s[:i])), 0o644)
	}
}

// pruneImports removes entries from the standard `import (` block whose
// package is not referenced in the rest of the file, so a transpiled file
// that lost its shim (whose imports are shim-only) still compiles.
func pruneImports(s string) string {
	start := strings.Index(s, "import (")
	if start < 0 {
		return s
	}
	depth := 0
	end := -1
	for k := start; k < len(s); k++ {
		switch s[k] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = k
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return s
	}
	block := s[start : end+1]
	rest := s[:start] + s[end+1:]
	var keep []string
	for _, ln := range strings.Split(block, "\n") {
		tr := strings.TrimSpace(ln)
		if strings.HasPrefix(tr, "\"") && strings.HasSuffix(tr, "\"") {
			p := strings.Trim(tr, "\"")
			short := p
			if i := strings.LastIndex(p, "/"); i >= 0 {
				short = p[i+1:]
			}
			// blank alias (`json "encoding/json"`) handled later if needed;
			// plain stdlib form referenced as `pkg.` or `pkg(`.
			if !referenced(short, rest) {
				continue
			}
		}
		keep = append(keep, ln)
	}
	return strings.TrimRight(s[:start], " 	") + "\n" + strings.Join(keep, "\n") + s[end+1:]
}

func referenced(short, body string) bool {
	return strings.Contains(body, short+".") || strings.Contains(body, short+"(")
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
