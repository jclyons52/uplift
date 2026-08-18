package transpile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tsmorph "github.com/jclyons52/ts-go-morph"
)

// transpileFile runs the full pipeline on a TS file.
func transpileFile(t *testing.T, path string) string {
	t.Helper()
	code, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		t.Fatal(err)
	}
	sf := p.CreateSourceFile("/"+path, string(code))
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// compileGo verifies generated code compiles by writing it into a temp module.
func compileGo(t *testing.T, name, src string) {
	t.Helper()
	dir := t.TempDir()
	// Compile as a library package: `package main` requires a main() func
	// that a transpiled module won't have.
	src = strings.Replace(src, "package main", "package ts2gotest", 1)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	gomod := "module ts2gotest\n\ngo 1.26.5\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Go does not compile:\n%s\n--- generated source ---\n%s", out, src)
	}
}

func TestRegExpMethodCalls(t *testing.T) {
	// Oracle-surfaced regression: re.test(s) was emitted as re.Test(s) (the
	// method name got export-cased) — the Go regexp package has no .Test.
	out := transpileFile(t, "../../testdata/regex_call.ts")
	for _, want := range []string{
		"regexp.MustCompile", // regex literal still maps
		".MatchString(",      // re.test(s) -> re.MatchString(s)
		".FindString(",       // re.exec(s) -> re.FindString(s)
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// The mangled .Test method must NOT appear.
	if strings.Contains(out, ".Test(") {
		t.Errorf("regexp .test must not be export-cased to .Test:\n%s", out)
	}
	compileGo(t, "regex.go", out)
}

func TestAssertDeepStrictEqualMapping(t *testing.T) {
	// Oracle-surfaced: chai assert.deepStrictEqual gapped to a vacuous pass
	// (dynamic access on any) instead of a real check — it must map to the
	// strict structural helper assertDeepEqual.
	out := transpileFile(t, "../../testdata/assert_deep.ts")
	if !strings.Contains(out, "assertDeepEqual(") {
		t.Errorf("assert.deepStrictEqual should map to assertDeepEqual:\n%s", out)
	}
}

func TestDynamicAnyTraversal(t *testing.T) {
	// The jsrt dynamic-value runtime: any-typed JS values are traversed via
	// shims — member access -> jsrtGet, .length -> jsrtLen, x||y -> jsrtOr,
	// for-of over any -> jsrtArray. This is the Phase B runtime core.
	out := transpileFile(t, "../../testdata/dynamic_any.ts")
	for _, want := range []string{
		"jsrtArray(", // for (const result of results) where results is any
		"jsrtGet(",   // result.filePath / result.line / result.severity
		"jsrtLen(",   // result.messages.length
		"jsrtOr(",    // result.line || 0
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	compileGo(t, "dynamic.go", out)
}

func TestJSONAndObjectShims(t *testing.T) {
	// JSON.stringify/parse and Object.keys/values/entries/freeze map to the
	// jsrt shims (impl supplied beside the target, e.g. the oracle harness).
	// Guarded at the mapping level (substring) since the shim defs live
	// outside a standalone transpile.
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/obj.ts", `
const o: any = { a: 1 };
const s = JSON.stringify(o);
const p2 = JSON.parse(s);
const ks = Object.keys(o);
const es = Object.entries(o);
const vs = Object.values(o);
const f = Object.freeze(o);
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"jsrtStringify(", "jsrtParse(",
		"jsrtKeys(", "jsrtEntries(", "jsrtValues(",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Object.freeze passes its argument through (no runtime freeze in Go).
	if strings.Contains(out, ".freeze(") || strings.Contains(out, ".Freeze(") {
		t.Errorf("Object.freeze should pass through its argument, not call .freeze:\n%s", out)
	}
}

func TestBankEndToEnd(t *testing.T) {
	out := transpileFile(t, "../../testdata/bank.ts")

	for _, want := range []string{
		"type Money = int64",
		"type Account struct {",
		"Balance int64",
		"type AccountKind int",
		"AccountKindChecking",
		"func NewBank(name string) *Bank {",
		"r.Name = name",
		"func (r *Bank) total() int64 {",
		"for _, a := range r.Accounts {",
		"sum += a.Balance",
		"if a > b {",
		"func max(a float64, b float64) float64 {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// The append statement must be an assignment.
	if !strings.Contains(out, "= append(r.Accounts, acc)") {
		t.Errorf("push should emit append assignment, got:\n%s", out)
	}
	// Builtin aliases must NOT be re-declared (recursive in Go).
	if strings.Contains(out, "type uint8") || strings.Contains(out, "type int64") {
		t.Errorf("builtin aliases must be skipped, got:\n%s", out)
	}
	compileGo(t, "bank.go", out)
}

func TestAliasConvention(t *testing.T) {
	out := transpileFile(t, "../../testdata/bank.ts")
	// The core trick: `type uint8 = number` makes `uint8` resolve to Go's
	// builtin uint8 — visible via field types (Balance: int64 from the
	// `type Money = int64` alias) and via the absence of a float64 mapping.
	if strings.Contains(out, "type uint8 = float64") || strings.Contains(out, "type int64 = float64") {
		t.Errorf("builtin alias fell back to float64:\n%s", out)
	}
	if !strings.Contains(out, "type Money = int64") {
		t.Errorf("Money alias should resolve through the convention:\n%s", out)
	}
}

func TestBanList(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/banned.ts", `
class Foo {
  method() {
    Object.setPrototypeOf(this, Foo.prototype);
    const x = eval("1+1");
    return x;
  }
}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	// Bans no longer abort: they become placeholders + report items.
	r := tp.Report()
	var banned int
	for _, it := range r.Items {
		if it.Severity == SevBanned {
			banned++
		}
	}
	if banned < 2 {
		t.Fatalf("expected banned items in report, got: %s", r.String())
	}
	if !strings.Contains(out, "TODO(ts2go)") {
		t.Errorf("expected placeholders in output:\n%s", out)
	}
	compileGo(t, "banned.go", out)
}

func TestReportAndPlaceholders(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/gaps.ts", `
function gaps(s: string): string {
  delete (globalThis as any).x;
  return s.substring(0, 3);
}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	r := tp.Report()
	if r.Complexity() == "TRIVIAL" {
		t.Errorf("expected non-trivial complexity, report:\n%s", r.String())
	}
	// Manifest embedded in the generated file.
	if !strings.Contains(out, "ts2go post-work manifest") {
		t.Errorf("output missing manifest:\n%s", out)
	}
	compileGo(t, "gaps.go", out)
}

func TestEnumValues(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/e.ts", `
enum Kind { A, B = 5, C }
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"KindA Kind = 0",
		"KindB Kind = 5",
		"KindC Kind = 6",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	compileGo(t, "e.go", out)
}

func TestStringLiteralUnion(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/u.ts", `
type Status = "active" | "in-progress" | "closed";
function label(s: Status): string {
  return s;
}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type Status string",
		"StatusActive Status = \"active\"",
		"StatusInProgress Status = \"in-progress\"",
		"StatusClosed Status = \"closed\"",
		"func label(s Status) string",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	compileGo(t, "u.go", out)
}

func TestRecursiveTypeArgs(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/r.ts", `
function f(m: Record<string, number[]>, a: Array<string>): number {
  return m["a"].length + a.length;
}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "m map[string][]float64") {
		t.Errorf("Record<string, number[]> not converted recursively:\n%s", out)
	}
	if !strings.Contains(out, "a []string") {
		t.Errorf("Array<string> not converted:\n%s", out)
	}
	compileGo(t, "r.go", out)
}

func TestArrayMethods(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/m.ts", `
function sum(xs: number[]): number {
  return xs.reduce((acc, x) => acc + x, 0);
}
function doubles(xs: number[]): number[] {
  return xs.map(x => x * 2);
}
function evens(xs: number[]): number[] {
  return xs.filter(x => x % 2 === 0);
}
function logAll(xs: number[]): void {
  xs.forEach(x => console.log(x));
}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	compileGo(t, "m.go", out)
	for _, want := range []string{"for _, x := range", "append("} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// runGo compiles generated code with an appended main() harness and returns
// its stdout — verifies runtime behavior, not just compilation.
func runGo(t *testing.T, name, src, mainFn string) string {
	t.Helper()
	dir := t.TempDir()
	src = strings.Replace(src, "package main", "package main", 1)
	src += "\nfunc main() {\n" + mainFn + "\n}\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module ts2gorun\n\ngo 1.26.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated Go does not run:\n%s\n--- generated source ---\n%s", out, src)
	}
	return string(out)
}

func TestAsyncShim(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/a.ts", `
async function greet(name: string): Promise<string> {
  return "hi " + name;
}

async function demo(): Promise<string> {
  const g = await greet("bob");
  const both = await Promise.all([greet("a"), greet("b")]);
  return g + " " + String(both);
}

function plain(x: number): number { return x * 2; }
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func greet(name string) *jsrtPromise",
		"return jsrtAsync(func() any {",
		"jsrtAwait(greet(\"bob\"))",
		"jsrtAll(greet(\"a\"), greet(\"b\"))",
		"type jsrtPromise struct",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	compileGo(t, "a.go", out)

	// Execute: the shim must actually run the async flow.
	got := runGo(t, "a.go", out, `p := demo(); fmt.Println(jsrtAwait(p))`)
	if want := "hi bob [hi a hi b]"; strings.TrimSpace(got) != want {
		t.Errorf("runtime output = %q, want %q", got, want)
	}
}

func TestAsyncRejection(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/r.ts", `
async function boom(): Promise<number> {
  throw new Error("kaput");
}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	compileGo(t, "r.go", out)
	// A throw inside an async body must reject the promise, not crash.
	got := runGo(t, "r.go", out, `
p := boom()
jsrtRun()
if p.err != nil {
	fmt.Println("rejected:", p.err)
} else {
	fmt.Println("resolved:", p.val)
}`)
	if want := "rejected: kaput"; !strings.HasPrefix(strings.TrimSpace(got), want) {
		t.Errorf("runtime output = %q, want prefix %q", got, want)
	}
}

func TestTryCatchBinding(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/t.ts", `
function guard(): string {
  try {
    return risky();
  } catch (e) {
    return "fallback";
  } finally {
    cleanup();
  }
}
function risky(): string { return "ok"; }
function cleanup(): void {}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "e := r") {
		t.Errorf("catch binding not emitted:\n%s", out)
	}
	compileGo(t, "t.go", out)
}

func TestSimpleFunction(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/f.ts", `
function add(a: number, b: number): number {
  return a + b;
}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "func add(a float64, b float64) float64 {") {
		t.Errorf("unexpected function signature:\n%s", out)
	}
	if !strings.Contains(out, "return a + b") {
		t.Errorf("unexpected body:\n%s", out)
	}
	compileGo(t, "f.go", out)
}
