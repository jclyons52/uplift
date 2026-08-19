package uplift

import (
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a.ts": `export function add(x: number) { return x; }
export function raw(x) { return x; }
`,
		"b.js": `module.exports = function untyped(a) { return a; };`,
		"c_test.go": `package x
import "testing"
func TestParity(t *testing.T) {}
`,
	}
	for p, c := range files {
		full := filepath.Join(dir, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
	}
	return dir
}

func TestAnalyze(t *testing.T) {
	rep, err := Analyze(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if rep.SchemaVersion != "uplift/v1" {
		t.Errorf("schema = %q", rep.SchemaVersion)
	}
	// type gap present (untyped sites exist)
	hasTypesGap := false
	for _, g := range rep.Gaps {
		if g.Category == "types" {
			hasTypesGap = true
		}
	}
	if !hasTypesGap {
		t.Errorf("expected a types gap, got %+v", rep.Gaps)
	}
	// JSON round-trips
	var b []byte
	var w = &nopWriter{}
	_ = b
	if err := rep.WriteJSON(w); err != nil {
		t.Fatal(err)
	}
	if w.n == 0 {
		t.Error("WriteJSON wrote nothing")
	}
}

type nopWriter struct{ n int }

func (w *nopWriter) Write(p []byte) (int, error) { w.n += len(p); return len(p), nil }

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"chalk":                        "chalk",
		"@humanwhocodes/object-schema": "humanwhocodes-object-schema",
		"@eslint-community/regexpp":    "eslint-community-regexpp",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
