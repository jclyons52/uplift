package scaffold

import "testing"

func TestSanitizeDir(t *testing.T) {
	cases := map[string]string{
		"chalk":                        "chalk",
		"@humanwhocodes/object-schema": "humanwhocodes-object-schema",
		"@scoped/pkg":                  "scoped-pkg",
		"json-schema-traverse":         "json-schema-traverse",
		"@nodelib/fs.stat":             "nodelib-fs-stat",
		"flat-cache LT 2":              "flat-cache-lt-2", // letter only caveat: spaces dropped
	}
	for in, want := range cases {
		if got := sanitizeDir(in); got != want {
			t.Errorf("sanitizeDir(%q) = %q, want %q", in, got, want)
		}
	}
	if got := sanitizePkg("chalk"); got != "chalk" {
		t.Errorf("sanitizePkg(chalk) = %q", got)
	}
	if got := sanitizePkg("json-schema-traverse"); got != "json" {
		t.Errorf("sanitizePkg(json-schema-traverse) = %q, want json", got)
	}
}

func TestDescribe(t *testing.T) {
	r := Repo{
		Name: "text-table", Dir: "text-table",
		Module: "github.com/jclyons52/text-table-go", Package: "text",
		Loc: 239, Exports: []string{}, NeededBy: []string{"stylish.js"},
	}
	d := r.Describe()
	for _, want := range []string{"text-table", "239 LOC", "github.com/jclyons52/text-table-go", "stylish.js"} {
		if !containsStr(d, want) {
			t.Errorf("Describe() missing %q:\n%s", want, d)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || containsIndex(s, sub) >= 0)
}

func containsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
