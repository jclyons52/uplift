# ESLint uplift status (uplift uplift, schema uplift/v1)

```
uplift status (schema uplift/v1) — /tmp/eslint-inspect/lib

  size           383 files, 70858 LOC
  type coverage  2/2254 sites (0%)
  complexity     total 5184, avg 13.5, max 213
  coupling       389 modules, 638 edges, 0 cycle(s)
  tests+ports    0 test files, 0 parity suites, 21 port backlog

gaps:
  [high] types: 2252/2254 declaration sites untyped (0% coverage)
  [high] tests: no test files / parity suites

next actions (highest value first):
  1. [types] lift rules/utils/ast-utils.js — 147 untyped declaration sites
  2. [types] lift linter/linter.js — 115 untyped declaration sites
  3. [types] lift rule-tester/rule-tester.js — 60 untyped declaration sites
  4. [structure] simplify rules/utils/ast-utils.js — cyclomatic complexity 213
  5. [structure] simplify linter/code-path-analysis/code-path-analyzer.js — cyclomatic complexity 139
  6. [decouple] hub rules/index.js — fan-out 292
  7. [decouple] hub linter/linter.js — fan-out 20
  8. [decouple] hub cli-engine/cli-engine.js — fan-out 8
  9. [tests] add-test parity — no parity suites; add one per ported library
  10. [port] port esquery — 15022 LOC leaf; no Go counterpart
  11. [port] port acorn — 6436 LOC leaf; no Go counterpart
  12. [port] port @eslint-community/regexpp — 4184 LOC leaf; no Go counterpart
```
