# Handoff — lodash.merge port + uplift skill condensation

Written 2026-08-23 (session cut off mid-work). Two tasks were in flight:
1. **Finish the lodash.merge port** (port-tracker row 17) — analysis complete, no code changed yet.
2. **Explore uplift tooling, condense Hermes memories into a project skill** — exploration mostly done, nothing written yet.

Repo: `~/Projects/jclyons52/lodash-merge-go` (NOT yet `git init`'d; no README, no test files).

---

## Task 1: lodash-merge-go

### State
- Go port code exists and compiles: `merge.go` (620 L), `decode.go`, `serialize.go`,
  `values.go` (~1,040 L total), package `lodashmerge`, module
  `github.com/jclyons52/lodash-merge-go`, `go 1.26.5`.
- `original/` vendors real lodash.merge **4.6.2** (index.js 1977 L + package.json).
  (LICENSE is at repo root, not in original/ — add/sweep per commit discipline.)
- `testdata/corpus.json`: 20 cases. Spec format = plain JSON values, plus an
  explicit graph encoding for shared/circular refs:
  `{"$id":N,"$t":"obj","$props":{...}}`, `{"$t":"arr","$items":[...]}`,
  `{"$ref":N}`, `{"$t":"undef"}`. NOTE: `$id` keys appear INSIDE `$props`
  (e.g. shared-obj-across-array-indices) — the builder must strip `$id` keys.
- `serialize.go` `SerializeGraph`: lossless cycle-aware JSON graph
  (`{"root":<valnode>,"objects":[...]}`, refs by first-encounter DFS order,
  obj props emitted key-sorted). Designed so a JS driver using the same
  algorithm is byte-comparable.
- `inputs.json` (20 entries) looks like an older/parallel corpus draft —
  decide whether to fold in or delete.

### Missing (the actual work)
1. `testdata/driver.js` — JS oracle driver (design below).
2. `parity_test.go` — Go test: build graph from corpus, run `Merge`,
   `SerializeGraph`, byte-compare vs driver output; log
   `PARITY PASS: N cases, 0 mismatches`; skip cleanly if no `node`.
3. Port fixes (below), then gates: `gofmt -l .`, `go build/vet/test ./...`,
   `git init` + commit as `"Joey" <dev@jclyons52.com>`, README with parity #.
4. Update `uplift/reports/port-tracker.md` row 17 → DONE + SHA (it still says
   TODO; the dir has been in-flight since Aug 22).

### Divergences found (Go port vs real lodash 4.6.2, verified against original/index.js)
Fix these in `merge.go`:
1. **String sources**: lodash `isArrayLike("abc")` is true (length 3) → a
   string source merges as index keys `dest["0"]='a'...`. Go `isArrayLike`
   has no string case → no-op. Add: `case string: return isLength(float64(len(x)))`
   + string case in `arrayLikeKeys` (index keys) + `safeGet`/direct-get string
   case (char at index).
2. **`baseFor` uses `safeGet` but lodash `createBaseFor` reads `iterable[key]`
   DIRECTLY** (no `__proto__`/`constructor` guards). Add a guard-free
   `getDirect` and use it in `baseFor`; keep `safeGet` in `baseMergeDeep`
   (lodash does use safeGet there). Matters for own `__proto__` primitive
   values (srcValue should be the value, not undefined).
3. **`arrayLikeKeys` JObject case drops `"length"`** — lodash includes ALL own
   keys (inherited pass) for non-array-like-indexed objects. Fix: return every
   own key. (Iteration order is irrelevant to result; keep sorted.)
4. **`copyArray` fails on array-like `*JObject` source** — returns `array`
   (nil) when source isn't `*JArray`. lodash: `Array(length)` then
   `array[i] = source[i]` (missing index → undefined element). Fix: build a
   fresh `*JArray`, items = direct-get at each index (missing → `Undefined`).
5. **`keyInObject` needs `in`-semantics**: lodash `assignMergeValue` checks
   `key in object` (inherited counts). Add: `constructor` → true for
   JObject/JArray/JArguments; `length` → true for JArray/JArguments.
6. **`baseAssignValue` on JArray/JArguments with key `"length"`** — lodash
   resizes the array (`arr.length = n`). Add resize (pad with `Undefined`,
   truncate) for non-negative integer numbers. Corpus only uses clean ints.
7. **`safeGet`/direct-get need `JArray`/`JArguments` `"length"`** →
   `float64(len)`.
8. **`Stack.Get` for non-container keys**: `ptrID` returns 0 for undefinedT
   etc.; safe today only because no container has id 0, but make it explicit
   (type-check before map lookup).
9. **`fmtJSNumber`** (serialize.go): correct for ints < 1e21; non-integers use
   `'e'` format which diverges from JS `String(n)`. Either fix
   (`FormatFloat 'g' -1`, strip leading zero in exponent, `-0` → `"0"`) or
   simply keep the corpus to integers with |n| < 1e16 and document it.

### Oracle driver design (agreed while analyzing)
- `node testdata/driver.js <corpus.json> <original/index.js>`; driver requires
  the vendored merge, builds each case's graph from the same spec
  (`$id`/`$t`/`$props`/`$items`/`$ref`/`$undef`), runs
  `merge(obj, ...sources)`, serializes the result with the SAME graph
  algorithm as Go `SerializeGraph` (register-before-children DFS, sorted obj
  keys, same valnode/container encoding, typed-array `bytes` = b64 of
  `String(length)` placeholder, fn → `v.name`, args via `[object Arguments]`).
- Output: JSON array `[{"name":..., "out":"<graph json>"}]`, file order.
- Test compares per-case byte-for-byte; on mismatch print first diff index +
  context. Build argv slice explicitly (`[]string{driverPath, corpus, pkg}`) —
  per skill, `-e` + spread form is rejected by the compiler here.

### Planned corpus additions (beyond existing 20)
- string source → index keys; falsy sources incl. `$undef`; `$fn` function
  values (opaque leaves; fn-dest→object replacement); `$args` (arguments
  object → toPlainObject path); `$typed` (Int8Array/Uint32Array clones);
  `$buffer` (b64, clone); array-like plain object with `length` prop both
  directions (src array → copyArray; src obj → all keys incl. length);
  `length`-key merge into array dest (resize); own `__proto__` with PRIMITIVE
  value (works after fix #2).
- **Exclude & document as known limitations** (prototype-chain semantics the
  Go model can't express): primitive destinations (lodash boxes via
  `Object(value)`); object-valued `__proto__` sources (real lodash 4.6.2
  prototype-POLLUTES Object.prototype — would contaminate the oracle process
  and diverge); function-valued `constructor` keys; arrays with extra string
  keys; non-null prototypes / inherited props; BigInt/NaN/Infinity;
  hole-vs-undefined array distinction; |int| ≥ 1e16 (byte format).
  README must state covered-vs-limits explicitly (parity-locked subset).

### Verified-correct (no change needed)
- `eq` = SameValueZero ✓; `safeGet` constructor-fn + `__proto__` guards ✓
  (match source lines 1418+); `toPlainObject` = copyObject(value, keysIn) ✓;
  `initCloneObject` ≡ `{}` for corpus protos ✓; `cloneBuffer(true)` = slice
  copy ✓; `cloneTypedArray` ≡ ctor+len model ✓; baseMerge `object===source`
  early-return ✓; baseMergeDeep array/buffer/typed/plain/args dispatch ✓
  (fresh `[]` for array src w/ non-arraylike dest ✓); `createAssigner`
  falsy-source skip ≡ `isTruthySource` ✓.

---

## Task 2: uplift tooling + skill condensation (after lodash done)

Question: "is it clear how to use uplift to port Go libraries **as an agent**?"

### What exists (explored)
- `uplift/reports/` + `uplift/` CLI: `uplift <dir> [--json]` (status+next
  actions, schema uplift/v1), `lift`, `measure [--compare]`, `deps [--json]`,
  `registry`, `scaffold <dir> --out R --only <pkg> --vendor`, `ts2go`,
  `bench`, `spec`, `shim`. All report commands have stable `--json` schemas —
  the agent-consumption story is real.
- **Hermes memories** (the "condense into a skill" source):
  - `~/.hermes/memories/MEMORY.md` §4: ts2go→uplift rename, module/binary
    names, oracle at /tmp/uplift, dogfood ESLint oracle-parity list,
    stylish blocked on chalk/sinon/proxyquire + dynamic method-call emission,
    transpiler gap list (jsrtNum +=, string-return coercion, expr-bodied
    arrow, Record<dyn>→[]string), LIFT arrow-paren fix, fixUp alias.
  - `~/.hermes/memories/USER.md` §6: tracker location, 17/21 state,
    remaining = lodash.merge/regexpp/esquery, session-start protocol.
- **Existing Hermes skills** (raw material; overlap + drift to reconcile):
  - `~/.hermes/skills/development/js-to-go-port/SKILL.md` (+ scripts/) — the
    main port workflow; referenced by port-tracker's session-start protocol.
  - `development/uplift-js-go-toolchain` (+ references/ts-go-morph-ast-and-spec.md)
  - `development/uplift-measurement` (+ references/)
  - `development/subagent-port-delegation` — delegation + independent verify
    (uncached `go test -count=1 -v` grep, never trust self-reports).
  - `development/js-oracle-parity-testing` — driver/corpus/diff techniques
    (BigInt replacer, freeze `.slice().sort()`, wrapper option-resolution,
    token-stream gotchas, two gates structural vs exact-positions).
  - `~/.hermes/skills/software-development/parity-port-orchestration` —
    batch delegation + budget-resilient mode; overlaps heavily with
    subagent-port-delegation (reconcile/dedupe).
  - `software-development/ts-to-go-porting`, `go-gqlcodegen-porting` — check
    relevance; gqlcodegen likely out of scope.

### Plan
1. Answer the clarity question with evidence (run `uplift --help`,
   `uplift <eslint-dir> --json` dry-run; check what an agent needs: entry
   command, machine schemas, skill pointer).
2. Write ONE condensed skill in the uplift project (e.g.
   `uplift/skills/js-to-go-port/SKILL.md` or `uplift/SKILL.md` — decide by
   project convention; check pi/hermes skill discovery for project-local
   skills) merging: port workflow + toolchain CLI + measurement + oracle
   harness gotchas + delegation/verification, updated with this session's
   learnings (lodash-merge divergences above, string-source/array-like
   semantics, `in`-semantics, prototype-chain limitation class).
3. Note in the skill that Hermes memories were the seed; leave memories in
   place (don't delete user data) but mark the skill as the living doc.
