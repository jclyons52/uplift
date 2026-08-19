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
	if got := sanitizePkg("json-schema-traverse"); got != "jsonschematraverse" {
		t.Errorf("sanitizePkg(json-schema-traverse) = %q, want jsonschematraverse", got)
	}
	if got := sanitizePkg("eslint-visitor-keys"); got != "eslintvisitorkeys" {
		t.Errorf("sanitizePkg(eslint-visitor-keys) = %q, want eslintvisitorkeys", got)
	}
	if got := sanitizePkg("2to3"); got != "_2to3" {
		t.Errorf("sanitizePkg(2to3) = %q, want _2to3", got)
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

func TestPortQueueOf(t *testing.T) {
	repos := []Repo{
		{Name: "foo", Module: "github.com/jclyons52/foo-go", Dir: "foo", Exports: []string{"A", "B"}},
		{Name: "bar", Module: "github.com/jclyons52/bar-go", Dir: "bar"},
	}
	pq := PortQueueOf(repos, Options{})
	if pq.Schema != "port-queue/v1" {
		t.Errorf("schema = %q", pq.Schema)
	}
	if len(pq.Repos) != 2 {
		t.Fatalf("repos = %d, want 2", len(pq.Repos))
	}
	if len(pq.Repos[0].Units) != 2 || pq.Repos[0].Units[0].Name != "A" || pq.Repos[0].Units[0].Status != "pending" {
		t.Errorf("foo units wrong: %+v", pq.Repos[0].Units)
	}
	// a repo with no discovered exports still gets a sentinel unit
	if len(pq.Repos[1].Units) != 1 || pq.Repos[1].Units[0].Name != "$entry" {
		t.Errorf("bar units wrong: %+v", pq.Repos[1].Units)
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
