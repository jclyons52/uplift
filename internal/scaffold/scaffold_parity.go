package scaffold

import (
	_ "embed"
	"strings"
)

//go:embed templates/parity_test.go.tmpl
var parityTemplate string

// jsDriverSrc is the node script embedded (inside backticks) into the
// generated parity_test.go. It must not contain backticks or ${}.
var jsDriverSrc = `'use strict';
const fs = require('fs');
const corpus = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
// Point this at your original's main entry (or submodules) as you add cases.
const orig = require(process.env.PORT_ORIG);
function r(e) {
  switch (e.op) {
    // TODO: add one case per exported function, e.g.
    // case 'IsFoo': return orig.isFoo(e.a, e.b);
    default: return 'UNKNOWN_OP:' + e.op;
  }
}
const out = [];
for (const e of corpus) out.push(r(e));
fs.writeFileSync(process.argv[3], JSON.stringify(out));
`

// parityHarness renders a real parity-harness skeleton for a ported repo. It
// encodes the machinery every JS→Go port needs (node driver, corpus builder,
// Go/Js dispatcher, comparison loop) that used to be hand-written per port,
// with the per-library cases left as marked sections. It compiles and passes
// trivially with an empty corpus, so a port starts green and adds cases as
// functions land.
func parityHarness(r Repo) string {
	var exports string
	if len(r.Exports) > 0 {
		exports = "Candidates for parity cases (from the dependency analysis):\n//   " + strings.Join(r.Exports, ", ")
	} else {
		exports = "Inspect `original/` for the exported functions to cover."
	}
	out := parityTemplate
	out = strings.ReplaceAll(out, "{{PACKAGE}}", r.Package)
	out = strings.ReplaceAll(out, "{{EXPORTS}}", exports)
	out = strings.ReplaceAll(out, "{{JSDRIVER}}", jsDriverSrc)
	return out
}
