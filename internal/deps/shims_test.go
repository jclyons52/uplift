package deps

import (
	"regexp"
	"testing"
)

// Every registry verdict of "inline" must be an assessed-once, do-not-split
// package that has a canonical shim to inline — and every collected shim must
// be backed by an inline registry verdict. This keeps the two sides of the
// judgment in sync so a package is never marked "inline" with nowhere to get
// the code, nor given a shim without the do-not-split verdict.
func TestInlineShimsBackVerdicts(t *testing.T) {
	reg := LoadCounterparts(nil)

	noteRef := regexp.MustCompile(`uplift shim ([A-Za-z0-9@_./-]+)`)
	shimOf := func(note string) []string {
		m := noteRef.FindAllStringSubmatch(note, -1)
		var out []string
		for _, g := range m {
			out = append(out, g[1])
		}
		return out
	}

	inlineWithShim := map[string]bool{}
	for name, c := range reg {
		if c.Verdict != InlineIt {
			continue
		}
		for _, ref := range shimOf(c.Note) {
			if _, ok := InlineShims[ref]; !ok {
				t.Errorf("registry %q is 'inline' but references missing shim %q", name, ref)
			}
			inlineWithShim[ref] = true
		}
	}

	for _, name := range ListInlineShims() {
		c, ok := reg[name]
		if !ok {
			t.Errorf("shim %q has no registry entry — add an 'inline' verdict", name)
			continue
		}
		if c.Verdict != InlineIt && c.Verdict != UseStdlib {
			t.Errorf("shim %q exists but registry verdict is %q (want inline)", name, c.Verdict)
		}
		if !inlineWithShim[name] && !containsShimRef(c.Note) {
			t.Errorf("shim %q exists but no 'inline' verdict references it", name)
		}
	}
}

func containsShimRef(note string) bool {
	for _, n := range ListInlineShims() {
		if regexp.MustCompile(`uplift shim ` + regexp.QuoteMeta(n)).MatchString(note) {
			return true
		}
	}
	return false
}

func TestInlineShimsNonEmpty(t *testing.T) {
	if len(InlineShims) == 0 {
		t.Fatal("inline shims collection is empty")
	}
	if len(ListInlineShims()) != len(InlineShims) {
		t.Fatal("ListInlineShims should return one entry per shim")
	}
	for name, s := range InlineShims {
		if s == "" {
			t.Errorf("shim %q is empty", name)
		}
	}
}
