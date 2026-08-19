# ESLint baseline metrics (from uplift measure, schema measure/v1)

```
Measure report (schema measure/v1)
for /tmp/eslint-inspect/lib

Totals:      383 files, 70858 LOC
TypeCover:   2/2254 sites annotated (0.1%)
Complexity:  5184 total, avg 13.5, max 213
  most complex:
    rules/utils/ast-utils.js                       cyc=213 loc=2014
    linter/code-path-analysis/code-path-analyzer.js cyc=139 loc=726
    linter/linter.js                               cyc=138 loc=1825
    rules/no-extra-parens.js                       cyc=134 loc=1124
    rules/indent.js                                cyc=123 loc=1552
Coupling:    389 internal modules, 638 edges (density 0.004)
             avg fan-out 1.64, max fan-out 292, 0 cycle(s), 0 node(s) in cycles
  top hubs:
    rules/index.js                                 out=292 in=6
    linter/linter.js                               out=20 in=1
    cli-engine/cli-engine.js                       out=8 in=2
    rule-tester/flat-rule-tester.js                out=8 in=1
    source-code/token-store/cursors.js             out=7 in=1
Tests:       0 test files, 0 parity suites
```
