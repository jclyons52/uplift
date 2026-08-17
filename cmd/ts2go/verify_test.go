package main

import (
	"strings"
	"testing"

	"github.com/jclyons52/ts2go/internal/transpile"
)

func TestParseGoErrors(t *testing.T) {
	out := `# demo/lib
lib/format.go:14:26: undefined: jsrtPromise
# demo
./main.go:14:21: undefined: fmtPrice
main.go:9:5: expected ';', found '}'
	some continuation line
internal/x.go:3: cannot use int as string
# demo [./main.test]
`
	errs := parseGoErrors(out)
	if len(errs) != 4 {
		t.Fatalf("want 4 errors, got %d: %v", len(errs), errs)
	}
	checks := []struct {
		file string
		line int
		col  int
		msg  string
	}{
		{"lib/format.go", 14, 26, "undefined: jsrtPromise"},
		{"main.go", 14, 21, "undefined: fmtPrice"},
		{"main.go", 9, 5, "expected ';', found '}'"},
		{"internal/x.go", 3, 0, "cannot use int as string"},
	}
	for i, c := range checks {
		if errs[i].File != c.file || errs[i].Line != c.line || errs[i].Col != c.col || errs[i].Message != c.msg {
			t.Errorf("err %d = %+v, want %+v", i, errs[i], c)
		}
	}
}

func TestRewritePackageMain(t *testing.T) {
	in := "// header\n\npackage main\n\nvar x = 1\n"
	out := rewritePackageMain(in)
	if !strings.Contains(out, "package ts2go_verify") || strings.Contains(out, "package main") {
		t.Errorf("package main not rewritten:\n%s", out)
	}
	if got := rewritePackageMain("package src\n\nvar x = 1\n"); !strings.Contains(got, "package src") {
		t.Errorf("non-main package should be untouched:\n%s", got)
	}
}

func TestAttachCompileErrors(t *testing.T) {
	files := []*transpile.FileResult{
		{Name: "a.go", Code: "// a", Report: &transpile.Report{Input: "a.ts"}},
		{Name: "jsrt.go", RelDir: "lib", Code: "// shim"}, // no Report
	}
	errs := []transpile.CompileError{
		{File: "a.go", Line: 3, Col: 1, Message: "undefined: x"},
		{File: "lib/jsrt.go", Line: 5, Message: "something"},
		{File: "nope.go", Message: "found packages"},
	}
	unmatched := attachCompileErrors(files, errs)

	if len(files[0].Report.CompileErrors) != 1 {
		t.Fatalf("a.go should carry its error, got %v", files[0].Report.CompileErrors)
	}
	if !strings.Contains(files[0].Code, "ts2go compile finding") {
		t.Errorf("finding should be appended to a.go code:\n%s", files[0].Code)
	}
	if !strings.Contains(files[1].Code, "ts2go compile finding") {
		t.Errorf("finding should be appended to jsrt.go code (no report to attach to):\n%s", files[1].Code)
	}
	if len(unmatched) != 1 || unmatched[0].File != "nope.go" {
		t.Errorf("unmatched errors should be returned, got %v", unmatched)
	}
}

func TestVerifyBuildFindsErrors(t *testing.T) {
	// A file referencing an undefined identifier must produce a compile
	// finding; a clean file must not.
	bad := &transpile.FileResult{
		Name:   "main.go",
		Code:   "package main\n\nvar x = undefinedThing()\n",
		Report: &transpile.Report{},
	}
	errs := verifyBuild([]*transpile.FileResult{bad})
	found := false
	for _, e := range errs {
		if e.File == "main.go" && strings.Contains(e.Message, "undefinedThing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected undefined: undefinedThing finding, got %v", errs)
	}

	clean := &transpile.FileResult{
		Name:   "ok.go",
		Code:   "package ts2go_verify\n\nvar x = 1\n",
		Report: &transpile.Report{},
	}
	if errs := verifyBuild([]*transpile.FileResult{clean}); len(errs) != 0 {
		t.Fatalf("clean file should build, got %v", errs)
	}
}
