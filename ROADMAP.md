# uplift — cleaning up projects, mechanically and safely

> **One-line thesis:** The safest way to clean up a messy codebase is to do
> the *mechanical* work with tools and sequence the *judgment* work behind
> safety gates — types first (they cannot change runtime behaviour), then
> decoupling, then inversion of control, then performance. `uplift` is the
> umbrella that wires the existing jclyons52 tools into that pipeline.

---

## 1. Why this exists

Cleaning up a poorly-typed, tightly-coupled, untested, slow codebase is
risky when done by hand, so it doesn't get done. But most of the work is
*mechanical* — and the parts that involve judgement are far safer if they're
gated by tests and by type information the compiler can vouch for.

`uplift` turns "clean up this project" from a scary open-ended effort into a
**staged, tool-assisted pipeline**, where every stage is either provably
safe (types) or gated by a verification loop (`uplift -verify`: transpile →
build the Go → feed every failure back). Each stage is a committable
checkpoint, so the project gets measurably cleaner and never red.

It is **not a new tool** — it is an *orchestration of tools you already
have*, plus a target workflow. The tools:

| Tool | Role in uplift |
|---|---|
| **`uplift`** | type engine: transpile TS→Go; `lift` (JS→TS via the checker); `-type-audit` (find where types are being thrown away); `-verify` (the feedback/safety gate) |
| **`go-typewryter`** | **inversion of control engine**: loads a dependencies type, infers providers, generates a tree-shaken DI container — the mechanism for swapping components in tests |
| **`ts-go-morph`** | shared foundation: Go port of ts-morph (bind `uplift` + `go-typewryter` to a common AST/type-checking layer) |
| **`go-gqlcodegen`** | proof of the port methodology (JS→Go codegen at byte parity) |

---

## 2. The uplift workflow (the methodology)

Applied to a target project, in this order:

1. **Types, first and safe.** Types **cannot cause runtime errors**, so
   filling them in is the single safest cleaning operation that exists.
   - `uplift lift` annotates JS/JSDoc: params, returns, typedefs → typed TS
     (`tsc --strict`-clean, commitable now).
   - `uplift -type-audit` lists every site still degraded to `any` where the
     checker resolves a concrete type — the exact "annotate here" list.
   - Refactoring *within* the type layer (renames, type narrowing) is
     mechanical and low-risk; it also makes the later stages easier.

2. **Run code in isolation.** Transpile a module to Go (`uplift`) and execute
   it independently (jsrt shim) so its real behaviour is observable without
   its neighbours. The **parity smoke test** — run the transpiled piece
   against its original and diff — turns "does the port behave right?" into
   a red/green gate (the TypeScript team's 74/6,000 metric).

3. **Decouple to simplify unit testing.** Use dependency analysis (which
   components touch which) to find tight couplings; push behavior behind
   interfaces/seams. Smaller, seam-ful components are unit-testable in
   isolation.

4. **Inversion of control + integration testing.** This is where
   **`go-typewryter`** enters: generate a DI container for a package, so
   each service **declares its deps and receives them, rather than
   constructing them**. For tests, swap the real integrations (fs, network,
   DB) for fakes *through the container* — no production code changes
   needed. That is integration testing simplified: the wiring is explicit
   and inspectable.

5. **Test → measure → optimise.** Now that components run in isolation and
   are covered by tests, you can profile them and refactor for
   performance — the big, fun refactors, done *behind* a safety net instead
   of on a prayer.

6. **Repeat.** Each pass is small, committable, and leaves the project
   strictly cleaner.

---

## 3. Roadmap phases

The phases below fold in the current `uplift` work (the "first things" being
worked on) and build up to a full uplift demonstration.

### Phase A — finish the type engine + safety gate (in progress)
Completing `uplift` so the type stage and the verification loop are sound on
real codebases. *(This is the current backlog.)*
- **CommonJS** (`require` / `module.exports`) — the blocker for transpiling
  a whole JS codebase (ESLint class) to Go. (pushed back)
- **Parity/oracle** — transpile a project + its tests, run under jsrt, report
  pass/fail vs the JS baseline as one number (the Corsa "74/6,000" metric).
  (pushed back)
- **binder/transformers long-tail** — remaining compile findings in the TS
  compiler dogfood slice. (pushed back)
- **type-audit-driven auto-annotation** — close the diagnose→fix loop.
  (pushed back)
- **concurrent `-verify` caching** — make the feedback gate fast at scale.
  (pushed back)

### Phase B — the isolation bridge (run transpiled code)
The runtime half of "run in isolation" and the parity gate.
- jsrt runtime v1: serialized sync segments, microtask ordering, `.then`
  chains — so transpiled async behaves like the JS original.
- **Parity smoke test**: transpiled parser/scanner vs the real compiler on
  TS input; a red/green "does the port match?" gate that makes this session's
  dogfood loop into a permanent, actionable metric instead of a one-off.
- Node-API surface (fs, path) so real CLI-shaped modules run.

### Phase C — inversion of control via typewryter
The link between "run in isolation" and "integration-testable services."
- `go-typewryter` `generate` against a transpiled TS package: produce a
  container whose providers are the transpiled (or lifted) services.
- **Swap harness**: a test-friendly container where a fake provider can
  replace a real one with no production edits — the "simplify integration
  testing" payoff, demonstrated on a real package.

### Phase D — uplift on a real target project (the proof)
Pick a target (ESLint-class JS or a jclyons52 codebase such as Conduit) and
run the **full workflow** end-to-end: lift → transpose → decouple → container
→ test → measure. Deliverable: a documented case study + the before/after
numbers (items, coupling, testability, runtime) that validate the
methodology — and expose where the tools still hurt.

### Phase E — the `uplift` CLI
Orchestrate lift / transpile / audit / typewryter-generate / parity across a
codebase from one command, producing a per-project progress report (types
filled, any remaining, coupling graph, testability, parity). This is the
"cleaning up projects" product surface. Priority order within a project
(stage 1→6 above) is the default plan `uplift` proposes.

The **measurement layer exists** (`uplift measure`, schema `measure/v1`): a
comparable per-codebase report — type coverage ratio, cyclomatic complexity,
coupling (modules/edges/density/fan-out/hubs/cycles), and tests/parity. `uplift
measure --compare before.json,after.json` diffs two snapshots into an
annotated delta. And the **`uplift` status CLI exists** (`uplift uplift <dir>`,
schema `uplift/v1`): it measures a codebase and emits a stage-ordered,
prioritized next-action plan (types → structure → decouple → tests → ports)
that both a human (`--json`-less) and an agent (`--json`) can consume. What
remains for Phase E is driving the *stage sequence* (lift → verify → re-measure
per module) from one command and threading the harness through it.

---

## 4. Guiding principles

- **Safe first, then big.** Never do a large refactor before the smaller,
  safer ones that make it possible to verify.
- **Types are free wins.** They cannot change runtime behaviour — do them
  everywhere, early. Numbers don't move, but every later stage gets easier.
- **Isolation before testing.** You can't unit-test tightly-coupled code and
  you can't integration-test un-wireable code. IoC (via typewryter) exists
  to make the wiring explicit and swappable — that's its whole job.
- **Every stage is a checkpoint.** Generate code → build it → feed errors
  back (`-verify`). If it's red, the stage isn't done.
- **Measure or it didn't happen.** The parity number, the type-audit count,
  the runtime before/after — uplift earns its keep by making "cleaner" mean
  "measurably cleaner."

---

## 5. Open questions to settle

- **Target project for Phase D**: a JS/JSDoc codebase (ESLint) — exercises
  lift + CommonJS + full JS→TS→Go — vs a TS codebase (Conduit) — exercises
  IoC harder and reuses go-typewryter's own ecosystem. Probably Conduit for
  the IoC story; ESLint as the lift/transpiler stress test.
- **Where `uplift` lives as a product**: a thin orchestrating CLI over the
  existing binaries, vs a first-class repo that vendors them. (Phase E
  decides.)
- **jsrt scope**: how much of Node's async/microtask semantics the isolation
  bridge needs for the parity gate to be honest.

---

## 6. Future exploration: a recursive LLM harness

`uplift`/`uplift` already lean on the boundary between **mechanical** work
(the transpiler, the oracle, `-verify`) and **judgement** work (resolving the
gaps/issues the mechanical layer reports). Today the gap list is an LLM
work-item manifest dropped wholesale into the prompt. As a codebase scales,
that manifest — and the verification loop around it — can dominate the
context budget.

Worth exploring: make the harness itself **recursive and stateful** — hold
the full issue set out-of-context in program variables (a work queue, not a
document), and feed issues into the LLM **gradually, on demand**, as the
loop makes progress:

- A driver keeps the authoritative issue/todo list in its own state
  (persistent, deduped, prioritized), and passes the LLM only a small,
  current slice — the next N items plus the minimal code/context to act on
  each.
- Each resolution cycles back: the harness runs `-verify`/the oracle, updates
  the queue in **its own variables** (not by re-reading a giant report), and
  recurses with a fresh, short prompt.
- Effect: context stays short and cheap regardless of issue volume; the
  work-queue state (dedupe, ordering, "what's resolved") lives where it
  belongs — as data — rather than being re-inflated into context every turn.
- This is the same shape as the team's parity loop (do the minimal work,
  verify, feed the delta back) applied to the *driver* itself.

Open sub-questions: single long-running recursive agent vs a tool layer that
manages the queue; whether the queue should survive as a file/DB (crash
resume) or live in the driver's live state; and how much of this the
`lift`/`-type-audit`/`-verify` CLI should expose natively so the harness can
be thin.
