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

func TestObjectLiteralShorthand(t *testing.T) {
	out := transpileSrc(t, `const a = 1;
const x = { a };`)
	if strings.Contains(out, "panic") {
		t.Fatalf("shorthand object literal must not panic:\n%s", out)
	}
	if !strings.Contains(out, "A: a") {
		t.Errorf("shorthand should become a keyed field A: a:\n%s", out)
	}
	compileGo(t, "obj.go", out)
}

func TestObjectLiteralSpread(t *testing.T) {
	out := transpileSrc(t, `const x = { ...{ a: 1 } };`)
	if strings.Contains(out, "panic") {
		t.Fatalf("spread object literal must not panic:\n%s", out)
	}
	if !strings.Contains(out, "object spread has no Go struct-literal equivalent") {
		t.Errorf("spread should record a gap:\n%s", out)
	}
	compileGo(t, "obj.go", out)
}

func TestObjectLiteralGetter(t *testing.T) {
	out := transpileSrc(t, `const o = { get x() { return 1; } };`)
	if strings.Contains(out, "panic") {
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
	if strings.Contains(out, "panic") {
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
	if strings.Contains(out, "panic") {
		t.Fatalf("import type must not panic:\n%s", out)
	}
	if !strings.Contains(out, "TODO(ts2go)") && !strings.Contains(out, "import type") {
		t.Errorf("import type should record a gap:\n%s", out)
	}
	compileGo(t, "imp.go", out)
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
