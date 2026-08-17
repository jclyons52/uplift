package transpile

import (
	"strings"
	"testing"
)

func TestCJSAnonymousExport(t *testing.T) {
	out := transpileSrc(t, `exports.greet = function(name) { return "hi"; };`)
	if !strings.Contains(out, "func greet(") {
		t.Errorf("anonymous exports should become an exported func:\n%s", out)
	}
	if strings.Contains(out, "dynamic property access") {
		t.Errorf("exports.foo should not produce a dynamic gap:\n%s", out)
	}
	compileGo(t, "cjs.go", out)
}

func TestCJSModuleExportsReExport(t *testing.T) {
	out := transpileSrc(t, `function parse(s) { return "x"; }
module.exports = { parse };`)
	if !strings.Contains(out, "func parse(") {
		t.Errorf("re-export should keep the top-level func:\n%s", out)
	}
	if strings.Contains(out, "dynamic property access") {
		t.Errorf("module.exports object re-export should not be dynamic:\n%s", out)
	}
	compileGo(t, "cjs.go", out)
}

func TestCJSModuleExportsInlineFuncs(t *testing.T) {
	out := transpileSrc(t, `module.exports = {
  greet: function(name) { return "hi"; },
  tag: "adder"
};`)
	if !strings.Contains(out, "func greet(") {
		t.Errorf("inline object-member func should be exported:\n%s", out)
	}
	if !strings.Contains(out, "var tag = \"adder\"") {
		t.Errorf("inline object-member value should be exported:\n%s", out)
	}
	compileGo(t, "cjs.go", out)
}

func TestCJSRequireExternalGap(t *testing.T) {
	out := transpileSrc(t, `const fs = require("fs");
module.exports = { read: function(p) { return "x"; } };`)
	// The external require binding is dropped (no `var fs = require(...)`),
	// leaving it to a wire-up work item in package mode; no crash.
	if strings.Contains(out, "var fs = require") {
		t.Errorf("external require binding should be dropped:\n%s", out)
	}
	if !strings.Contains(out, "func read(") {
		t.Errorf("CJS export alongside a require should still emit:\n%s", out)
	}
}
