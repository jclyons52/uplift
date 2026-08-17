package transpile

import (
	"strings"
	"testing"
)

func TestUnaryNotBug(t *testing.T) {
	out := transpileSrc(t, `function f(data) {
  if (!data) { return "empty"; }
  return "full";
}`)
	if strings.Contains(out, "datadata") {
		t.Errorf("unary ! must not collapse to operand duplication:\n%s", out)
	}
	if !strings.Contains(out, "!jsrtTruthy(data)") {
		t.Errorf("expected `!jsrtTruthy(data)`:\n%s", out)
	}
	compileGo(t, "unary.go", out)
}

func TestStringLiteralUnionReturn(t *testing.T) {
	out := transpileSrc(t, `function f(): "a" | "b" { return "a"; }`)
	if strings.Contains(out, `"a" | "b"`) {
		t.Errorf("string-literal union should collapse to string:\n%s", out)
	}
	if !strings.Contains(out, "string") {
		t.Errorf("expected string return type:\n%s", out)
	}
	compileGo(t, "lit.go", out)
}
