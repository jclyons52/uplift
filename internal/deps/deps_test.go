package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixture creates a small project tree (including a fake node_modules)
// in a temp dir so the analysis is self-contained and repeatable.
func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"main.js": `const { x } = require("./util");
const chalk = require("chalk");
const fs = require("fs");
const h = require("imurmurhash");
module.exports = function(){ return { x, chalk, fs, h }; };`,
		"util.js":   `const helper = require("./helper"); module.exports = { helper };`,
		"helper.js": `module.exports = 42;`,
		// chalk depends on ansi-styles -> so chalk is NOT a leaf
		"node_modules/chalk/package.json":       `{"name":"chalk","main":"index.js","dependencies":{"ansi-styles":"^4.0.0"}}`,
		"node_modules/chalk/index.js":           `module.exports = { red: s => s };`,
		"node_modules/ansi-styles/index.js":     `module.exports = { red: [0,31] };`,
		"node_modules/ansi-styles/package.json": `{"name":"ansi-styles"}`,
		// single tiny leaf -> "too small to split" class
		"node_modules/imurmurhash/index.js":     `module.exports = function h(){ return 1; };`,
		"node_modules/imurmurhash/package.json": `{"name":"imurmurhash"}`,
	}
	for p, c := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAnalyze(t *testing.T) {
	res, err := Analyze(writeFixture(t))
	if err != nil {
		t.Fatal(err)
	}

	// internal files discovered
	for _, want := range []string{"main.js", "util.js", "helper.js"} {
		if res.Nodes[want] == nil || res.Nodes[want].Kind != KindInternal {
			t.Errorf("expected internal node %q, got %+v", want, res.Nodes[want])
		}
	}

	// builtins
	if n := res.Nodes["node:fs"]; n == nil || n.Kind != KindBuiltin {
		t.Errorf("expected builtin node:fs, got %+v", n)
	}
	// main.js requires chalk and node:fs and ./util
	main := res.Nodes["main.js"]
	for _, want := range []string{"chalk", "node:fs", "util.js"} {
		if !contains(main.Requires, want) {
			t.Errorf("main.js should require %q; got %v", want, main.Requires)
		}
	}

	// chalk depends on ansi-styles (external) -> not a leaf
	if res.IsLeaf("chalk") {
		t.Errorf("chalk depends on ansi-styles, should NOT be a leaf")
	}
	// ansi-styles has no external deps -> leaf
	if !res.IsLeaf("ansi-styles") {
		t.Errorf("ansi-styles has no external deps, should be a leaf")
	}

	// a tiny leaf classifies as "not worth a repo" (absorb)
	if r := res.Recommend("imurmurhash"); !strings.Contains(r, "absorb") && !strings.Contains(r, "too small") {
		t.Errorf("expected small-leaf absorb recommend, got %q", r)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestEntryExports(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "object-literal exports",
			src:  `module.exports = { red, green, blue: (x) => x, helper: require("./h") };`,
			want: []string{"red", "green", "blue", "helper"},
		},
		{
			name: "member assignments",
			src:  "exports.validate = f; exports.compile = g; module.exports.formats = {};",
			want: []string{"validate", "compile", "formats"},
		},
		{
			name: "multi-line object with nested literal",
			src:  "module.exports = {\n  a: 1,\n  b: { x: 1, y: 2 },\n  c: 3,\n};",
			want: []string{"a", "b", "c"},
		},
		{
			name: "esm named exports",
			src:  "export { foo, bar as baz };",
			want: []string{"foo", "bar as baz"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := entryExports(c.src)
			if len(got) != len(c.want) {
				t.Fatalf("entryExports(%q) = %v, want %v", c.src, got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("entryExports(%q)[%d] = %q, want %q (all: %v)", c.src, i, got[i], c.want[i], got)
				}
			}
		})
	}
}
