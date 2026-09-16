# Port Tracker — ESLint leaf backlog (21)

Single source of truth for the JS→Go port backlog. Update the status + commit
hash as each leaf lands. Verify claims with `go test ./...` in the port dir
before marking done. Last updated: 2026-09-16.

**All 21 leaves are ported** (20 DONE, 1 NO-OP — esquery, the last TODO, landed
2026-09-16). Next work is composition (eslint-go Linter + stylish reporter,
LIF-8) rather than more leaves.

## Legend
- `DONE` — port committed, parity green, repo initialized (code + tests + README)
- `NO-OP` — no runtime surface to port (e.g. type-only package); verified, not a port
- `NEXT` — recommended order for the next session(s)
- `TODO` — not started

## Queue (suggested order)

| # | Package | LOC | Go repo | Status | Commit | Notes |
|---|---------|-----|---------|--------|--------|-------|
| 1 | json-schema-traverse | 331 | json-schema-traverse-go | DONE | 677438f | |
| 2 | uri-js | 2390 | uri-js-go | DONE | 53a3f7c | |
| 3 | debug | 735 | debug-go | DONE | 94dd949 | namespace-matching core only |
| 4 | espree | 783 | espree-go | DONE | e23fc73 | 3 gates: parse, pos+tokens, tokenize; 27 sources |
| 5 | acorn | 6436 | acorn-go | DONE | 6cb09e7 | 91/91 JS-oracle cases incl. dynamic import() + import.meta |
| 6 | type-fest | 2311 | — | NO-OP | — | type-only: 0 `.js` files, no `main`, all 39 files `.d.ts` (verified from real 0.20.2 tarball); TS erases to zero runtime → not a port |
| 7 | ignore | 1102 | ignore-go | DONE | e69deb4 | |
| 8 | argparse | 3691 | argparse-go | DONE | 2ed8a5b | |
| 9 | color-name | 151 | color-name-go | DONE | 03101bb | 148 entries, 0 mismatches |
| 10 | flatted | 374 | flatted-go | DONE | 4b87945 | 28/28 round-trip parity, 13/13 byte-parity; vendored 3.4.4 |
| 11 | @humanwhocodes/object-schema | 395 | humanwhocodes-object-schema-go | DONE | 068e5f6 | |
| 12 | @ungap/structured-clone | 609 | ungap-structured-clone-go | DONE | 81a1c85 | round-trip 29/0, structural 15/0 |
| 13 | @nodelib/fs.stat | 172 | nodelib-fs-stat-go | DONE | 835d3c9 | |
| 14 | prelude-ls | 1339 | prelude-ls-go | DONE | e0e686b | |
| 15 | eslint-scope | 1931 | eslint-scope-go | DONE | 53e9b23 | |
| 16 | estraverse | 756 | estraverse-go | DONE | 9caee81 | |
| 17 | lodash.merge | 1819 | lodash-merge-go | DONE | 8dfcab6 | 42/42 byte-parity; string/array-like sources, length resize, __proto__ primitives, args/typed/buffer |
| 18 | esquery | 15022 | esquery-go | DONE | 4721eef | 1850 parse + 29874 query + 6786 matches (94024 node results) cases, 0 mismatches; reuses estraverse-go (fixed its parent-arg bug: 8421809) |
| 19 | @eslint-community/regexpp | 4184 | eslint-community-regexpp | DONE | 88c4da2 | 62/62 byte-parity incl. named groups, v-mode, modifier groups; \p + non-ASCII excluded (README) |
| 20 | isexe | 318 | isexe-go | DONE | 395e9b1 | |
| 21 | esutils | 409 | esutils-go | DONE | c9662c6 | |

## Composition target (not a leaf)
- **eslint-go** — minimal Linter core running real rules (68b7ca4, 19 cases 0
  mismatches). This is where the leaves get composed; the stylish reporter is
  blocked on chalk/sinon/proxyquire shims (see eslint-baseline.md).

## Session start protocol
1. Read this file.
2. Pick the top `NEXT`/`TODO` row (or ask).
3. Follow the `js-to-go-port` skill (scaffold, parity harness, commit).
4. Mark `DONE` + commit hash here, commit the uplift repo.
