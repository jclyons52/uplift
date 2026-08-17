package lift

import (
	"strings"
	"testing"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/core"
)

// lift parses JS with JSDoc and runs the lift.
func lift(t *testing.T, src string) string {
	t.Helper()
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{
		UseInMemoryFileSystem: true,
		CompilerOptions:       &core.CompilerOptions{AllowJs: core.TSTrue, CheckJs: core.TSTrue},
	})
	if err != nil {
		t.Fatal(err)
	}
	sf := p.CreateSourceFile("/lift.js", src)
	if sf == nil {
		t.Fatal("nil source file")
	}
	out, err := Lift(p, sf)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLiftFunctionFromJSDoc(t *testing.T) {
	out := lift(t, `/**
 * @param {string} name
 * @returns {string}
 */
function greet(name) {
  return "hi " + name;
}
`)
	if !strings.Contains(out, "function greet(name: string): string {") {
		t.Errorf("param + return should be annotated:\n%s", out)
	}
}

func TestLiftOptionalParam(t *testing.T) {
	out := lift(t, `/**
 * @param {number} [count]
 * @returns {number}
 */
function f(count) {
  return 0;
}
`)
	// count is optional: checker says number | undefined — emit count?: number.
	if !strings.Contains(out, "count?: number") {
		t.Errorf("optional param should be count?: number:\n%s", out)
	}
	if !strings.Contains(out, ": number {") {
		t.Errorf("return type missing:\n%s", out)
	}
}

func TestLiftUnannotatedParamLeftAlone(t *testing.T) {
	out := lift(t, `function f(x) {
  return x;
}
`)
	// x has no JSDoc and is inferred any — the lift must not force a type.
	if strings.Contains(out, "x:") {
		t.Errorf("unannotated param should be left alone:\n%s", out)
	}
}

func TestLiftNestedArrowContextual(t *testing.T) {
	// x is contextually typed by forEach's callback.
	out := lift(t, `const arr = [1, 2, 3];
arr.forEach((x) => console.log(x * 2));
`)
	if !strings.Contains(out, "(x: number)") && !strings.Contains(out, "(x: any)") {
		t.Errorf("contextually-typed arrow param should be annotated:\n%s", out)
	}
	if strings.Contains(out, "=> any") {
		t.Errorf("arrow should not gain a spurious return type:\n%s", out)
	}
}

func TestLiftPreservesRestOfFile(t *testing.T) {
	src := `/**
 * @param {string} a
 * @returns {number}
 */
function len(a) {
  return a.length;
}
const marker = "keep-me";
`
	out := lift(t, src)
	if !strings.Contains(out, `const marker = "keep-me";`) {
		t.Errorf("unrelated source must be preserved byte-for-byte:\n%s", out)
	}
	if !strings.Contains(out, "function len(a: string): number {") {
		t.Errorf("annotation wrong:\n%s", out)
	}
}

func TestLiftRestParam(t *testing.T) {
	out := lift(t, `/**
 * @param {...number} nums
 * @returns {number}
 */
function sum(...nums) {
  return nums.length;
}
`)
	if !strings.Contains(out, "...nums: number[]") {
		t.Errorf("rest param should be typed as array:\n%s", out)
	}
}
