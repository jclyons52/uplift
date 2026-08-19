package transpile

import (
	"strings"
	"testing"

	tsmorph "github.com/jclyons52/ts-go-morph"
)

// transpileSrc runs the full pipeline on inline TS source.
func transpileSrc(t *testing.T, src string) string {
	t.Helper()
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		t.Fatal(err)
	}
	sf := p.CreateSourceFile("/obj.ts", src)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// preShim returns the generated source without the auto-emitted jsrt shim,
// so body-level checks don't trip on the shim's own `panic(`/runtime lines.
func preShim(out string) string {
	if i := strings.Index(out, "// jsrt:"); i >= 0 {
		return out[:i]
	}
	return out
}

func TestObjectLiteralShorthand(t *testing.T) {
	out := transpileSrc(t, `const a = 1;
const x = { a };`)
	if strings.Contains(preShim(out), "panic") {
		t.Fatalf("shorthand object literal must not panic:\n%s", out)
	}
	// Untyped literals emit as an insertion-ordered *jsrtObj preserving the
	// original key spelling.
	if !strings.Contains(out, `jsrtKV{"a", a}`) {
		t.Errorf("shorthand should become a keyed jsrtObj entry jsrtKV{\"a\", a}:\n%s", out)
	}
	compileGo(t, "obj.go", out)
}

func TestObjectLiteralSpread(t *testing.T) {
	out := transpileSrc(t, `const x = { ...{ a: 1 } };`)
	if strings.Contains(preShim(out), "panic") {
		t.Fatalf("spread object literal must not panic:\n%s", out)
	}
	if !strings.Contains(out, "object spread has no Go struct-literal equivalent") {
		t.Errorf("spread should record a gap:\n%s", out)
	}
	compileGo(t, "obj.go", out)
}

func TestObjectLiteralGetter(t *testing.T) {
	out := transpileSrc(t, `const o = { get x() { return 1; } };`)
	if strings.Contains(preShim(out), "panic") {
		t.Fatalf("getter object literal must not panic:\n%s", out)
	}
	if !strings.Contains(out, "KindGetAccessor has no Go struct-literal equivalent") {
		t.Errorf("getter should record a gap:\n%s", out)
	}
	compileGo(t, "obj.go", out)
}

func TestObjectLiteralAnonymousFallback(t *testing.T) {
	// An object literal the checker resolves to an anonymous __object type
	// takes the anonymous-struct fallback; it must be valid Go.
	out := transpileSrc(t, `const x = { a: 1, b: "y" };`)
	if strings.Contains(preShim(out), "panic") {
		t.Fatalf("anonymous object literal must not panic:\n%s", out)
	}
	compileGo(t, "obj.go", out)
}

func TestObjectLiteralTypedShape(t *testing.T) {
	out := transpileSrc(t, `interface Pair { a: number; b: string; }
const x: Pair = { a: 1, b: "y" };`)
	compileGo(t, "obj.go", out)
}

// TestImportTypeDegrades: typeof import("fs") in a type position must not
// fail the statement — it becomes any + a gap.
func TestImportTypeDegrades(t *testing.T) {
	out := transpileSrc(t, `function f(x: typeof import("fs")): number { return 1; }`)
	if strings.Contains(preShim(out), "panic") {
		t.Fatalf("import type must not panic:\n%s", out)
	}
	if !strings.Contains(out, "TODO(uplift)") && !strings.Contains(out, "import type") {
		t.Errorf("import type should record a gap:\n%s", out)
	}
	compileGo(t, "imp.go", out)
}

// TestNestedFunctionBecomesClosure: a function declared inside a function is
// legal TS but illegal Go; it must become a local closure.
func TestNestedFunctionBecomesClosure(t *testing.T) {
	out := transpileSrc(t, `function outer() {
  function inner(x: number) { return x + 1; }
  return inner(1);
}`)
	if strings.Contains(out, "func inner") {
		t.Fatalf("nested function must not emit a Go declaration:\n%s", out)
	}
	if !strings.Contains(out, "inner := func(x float64) float64") {
		t.Errorf("nested function should become a closure:\n%s", out)
	}
	compileGo(t, "nest.go", out)
}

// TestConstructorTypeMapsToFunc: `new (args) => T` becomes a func type with
// an approx flag, not raw TS text.
func TestConstructorTypeMapsToFunc(t *testing.T) {
	out := transpileSrc(t, `interface BoxLike { v: number; }
function makeIt(f: new (v: number) => BoxLike): number { return 1; }`)
	body := out[strings.Index(out, "package "):]
	if strings.Contains(body, "=> BoxLike") {
		t.Fatalf("constructor type must not leak raw TS into the body:\n%s", out)
	}
	if !strings.Contains(body, "func(v float64)") {
		t.Errorf("constructor type should map to a func type:\n%s", out)
	}
	if !strings.Contains(out, "constructor type mapped to func") {
		t.Errorf("constructor type should be flagged approx:\n%s", out)
	}
	compileGo(t, "ctor.go", out)
}

// TestTemplateLiteralWithQuotes: template chunks containing double quotes
// must be escaped inside the fmt.Sprintf format string.
func TestTemplateLiteralWithQuotes(t *testing.T) {
	src := "const name = \"x\";\nconst s = `\"${name}\"`;\n"
	out := transpileSrc(t, src)
	body := out[strings.Index(out, "package "):]
	if strings.Contains(body, `""%v""`) {
		t.Fatalf("embedded quotes must be escaped in the format string:\n%s", out)
	}
	if !strings.Contains(body, `\"%v\"`) {
		t.Errorf("expected escaped quotes around %%v:\n%s", out)
	}
	compileGo(t, "tpl.go", out)
}

// TestCheckerGenericBrackets: checker-rendered generic types (Map<K, V>,
// NodeArray<Node>) must use Go square brackets, not TS angle brackets.
func TestCheckerGenericBrackets(t *testing.T) {
	out := transpileSrc(t, `interface Box2 { v: number; }
function f(xs: Map<string, Box2>): number { return 1; }`)
	body := out[strings.Index(out, "package "):]
	if strings.Contains(body, "Map<string") || strings.Contains(body, "Box2>") {
		t.Fatalf("checker generic text must not leak angle brackets:\n%s", out)
	}
	if !strings.Contains(body, "map[string]Box2") {
		t.Errorf("generic should become map[string]Box2:\n%s", out)
	}
	compileGo(t, "gen.go", out)
}

// TestAssignmentAsExpression: `Set(x, sym = make())` — assignment used as a
// value — must become an IIFE, not raw Go.
func TestAssignmentAsExpression(t *testing.T) {
	out := transpileSrc(t, `let sym: string = "";
function f(): string {
  const x = (sym = "a");
  return sym;
}`)
	body := out[strings.Index(out, "package "):]
	if !strings.Contains(body, "func() any { sym = ") {
		t.Fatalf("assignment-as-value should become an IIFE:\n%s", out)
	}
	if !strings.Contains(out, "assignment used as a value") {
		t.Errorf("assignment-as-value should be flagged approx:\n%s", out)
	}
	compileGo(t, "asn.go", out)
}

// TestPanicRecovery proves the resilience contract holds even when a
// transpile step panics: the panic becomes an error (recorded as a fatal
// item, emitted as a TODO placeholder) instead of aborting the file. Both
// the statement loop and emitBlock wire this via `defer recoverToError`.
func TestPanicRecovery(t *testing.T) {
	err := func() (err error) {
		defer recoverToError(&err)
		panic("synthetic accessor bug")
	}()
	if err == nil || !strings.Contains(err.Error(), "synthetic accessor bug") {
		t.Fatalf("expected recovered panic error, got %v", err)
	}
}
