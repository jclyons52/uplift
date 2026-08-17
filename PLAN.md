# ts2go — TypeScript → Go transpiler

**Goal:** convert a TypeScript codebase to Go for speed, using ts-go-morph
(Go port of the TS compiler) for a type-checked AST. The tool is an
**LLM-assist pipeline**: it emits as much idiomatic Go as possible, marks
everything it cannot translate as a compiling `TODO(ts2go)` placeholder, and
produces a structured post-work report so the LLM gets a precise worklist
instead of hunting through the output.

## Core insight: syntax-first, checker-second

- **Syntax tree preserves names.** `type uint8 = number` + `x: uint8` — the
  checker collapses this to `number`, but the syntax node still says `uint8`.
  The transpiler reads *written* type annotations for names and only falls
  back to the checker where syntax is insufficient.
- **Checker resolves semantics.** Conditional types, mapped types, generics,
  unions, nullability — pre-computed by the TypeScript checker. Also used to
  type map/filter/reduce loops and to pick `math.Mod` for `%`.

## Resilience contract (the LLM-first reorientation)

Transpilation **never aborts** on unsupported constructs:

1. **Statement gaps** → `// TODO(ts2go): ...` + the TS snippet as a comment +
   `_ = 0` (compiles).
2. **Expression gaps** → `any(nil) /* TODO(ts2go): ... */` (compiles in most
   contexts; conditions are coerced to `false`).
3. **Bans** (prototype tricks, eval, Proxy, `with`) are detected on the AST
   (call-target matching — no false positives from strings/comments) and
   become placeholder + `banned` report items.
4. **Fatal internal errors** become `_ = 0` TODO statements and are recorded;
   the rest of the file still emits.

## Post-work report

- `Report` (`internal/transpile/assess.go`): per-gap work items with severity
  (`banned` w10 / `todo` w3 / `approx` w1), TS line, message, snippet.
- **Complexity score** labels the LLM effort: TRIVIAL/LOW/MEDIUM/HIGH/VERY
  HIGH — the dry-run assessment.
- The manifest is embedded as a comment block at the top of the generated
  file, so the TODO list travels with the code.
- CLI: `-dry-run` (assess only), `-report <file>` (full report to disk).

## Type mapping

| TS | Go | Notes |
|---|---|---|
| `number` | `float64` | bare number = JS double |
| `type uint8 = number` | `uint8` | the convention: alias NAME wins |
| `type Money = int64` | `type Money = int64` | chains through the fixpoint |
| `string` / `boolean` | `string` / `bool` | |
| `interface` (data only) | `struct` | fields exported (PascalCase) |
| `interface` (methods) | Go `interface` | method signatures |
| `class X` | `type X struct` + methods | `extends` → embedded field |
| `constructor(private name: string)` | `Name string` field + `NewX(name)` | |
| `enum` (numeric) | `type X int` + explicit consts | initializers folded TS-style |
| `enum` (string) | `type X string` + string consts | |
| string-literal union alias | named string type + consts | inline unions → `any` + gap |
| `T[]` / `Array<T>` | `[]T` | type args converted recursively |
| `Record<K,V>` | `map[K]V` | recursive too |
| `T \| null` / `T \| undefined` | `*T` | |
| `x?: T` | `*T` | flagged approx |
| tuple | `any` + gap | v1: struct |
| `void` | (no return type) | |

## Statement mapping

| TS | Go |
|---|---|
| `const x = e` | `x := e` (numeric literals typed `float64`) |
| `let x: T = e` | `var x T = e` |
| `if/else`, `for`, `while`, `do`, `switch` | same shape (missing switch default → `panic`) |
| `for (const x of arr)` | `for _, x := range arr` |
| `.map`/`.filter`/`.reduce` | typed loop IIFEs (checker-typed, approx-flagged) |
| `.forEach(cb)` | inline for-range + closure body |
| `arr.push(x)` (statement) | `arr = append(arr, x)` |
| `s.length` | `float64(len(s))` |
| `a % b` (floats) | `math.Mod(a, b)` |
| `a ** b` | `math.Pow` |
| template literals | `fmt.Sprintf` |
| `throw` | `panic` |
| `throw new Error(m)` | `panic(errors.New(m))` / `fmt.Errorf` |
| `try/catch/finally` | IIFE + `defer/recover`; catch binding kept; returns inside become TODOs |
| `new X(...)` | `NewX(...)` |
| top-level expression statements | synthesized `func init()` |
| undefined locals (legal TS, illegal Go) | `_ = x` appended heuristically |

## Ban list (AST-matched call targets)

`Object.create/setPrototypeOf/defineProperty(s)`, `Reflect.setPrototypeOf/
construct/apply/defineProperty`, `eval`, `Function`, `Proxy`, `with` —
each becomes a placeholder + `banned` work item (redesign required).

## Architecture

```
cmd/ts2go/main.go          CLI: -o, -dry-run, -report, -package, -verify;
                           single file or directory/multi-file package mode
cmd/ts2go/verify.go        Driver loop: build generated Go in a temp module,
                           parse `go build` findings, attach per-file
internal/transpile/
  transpile.go             Transpiler, goType, alias fixpoint, string-union
                           aliases, declared-name pre-pass
  package.go               Package: multi-file → one Go package per directory,
                           import classification (same/sibling/external),
                           once-per-package jsrt.go
  decls.go                 interfaces, classes, enums, aliases, functions,
                           variable statements, signatures, import/export
                           handling
  statements.go            statement + expression emitters, try/catch,
                           identifier-call builtins
  arrays.go                map/filter/reduce/forEach → typed loops
  gap.go                   WorkItem recording, placeholders, unused-local fix
  assess.go                Report: scoring, complexity label, manifest,
                           human rendering, per-file aggregation
  ban.go                   AST ban scan + banned-callee matching
  writer.go                indent-aware Go writer
testdata/bank.ts           end-to-end fixture
```

Key implementation notes:

- **Preamble emitted last**: imports computed from `tr.used` so only
  referenced stdlib packages are declared.
- **Operator extraction**: slice source between `left.End()` and `right.Pos()`
  (ts-go-morph's `OperatorText()` is unreliable).
- **Return-type conversions**: `retStack` tracks the enclosing Go return type;
  union-typed values returned as `string` get `string(...)` conversion.
- **inDeferred**: `return expr` inside recover/finally/try-IIFE handlers is
  invalid Go — the value is discarded with a precise TODO.
- **Numeric inference**: `let s = 0` / `for (let i = 0; ...)` emit explicit
  `float64` vars, since Go would infer `int` and break number arithmetic.
- **`.length` → `float64(len(...))`** keeps `number` arithmetic consistent.

## Testing

`transpile_test.go` runs end-to-end and **compiles the generated Go** in a
temp module (GOWORK=off). Tests cover: the alias convention, classes, param
properties, loops, templates, switch, enums with initializers, string-union
aliases, recursive type args, array-method loops, try/catch binding, ban
reporting, placeholders + manifest, and report complexity.

## Port discipline (from the TypeScript team's Corsa port)

The TypeScript team ported their compiler to Go in ~18 months (announced
Mar 2025, TypeScript 7.0 shipped) and documented the process in their blog
posts, the typescript-go repo, and their agent playbook
(`.github/agents/strada-corsa-port.md`). The lessons that map onto ts2go:

- **Faithful port, not rewrite.** They wrote new Go code while keeping the
  structure and logic of the original 1:1, so results stay consistent
  between the two compilers. ts2go is structural by construction (it
  transpiles), but the rule still applies: never "improve" behavior during
  transpile — anything behavioral is a flagged `approx`.
- **The original test suite is the oracle.** The Go repo imports the whole
  TypeScript test corpus via a git submodule pinned to the commit being
  ported. Tests are never ported — they are shared. Their headline parity
  metric is a number: ~20,000 tests, ~6,000 that produce errors, and the
  divergence count (was 74 mid-port, now 0). → ts2go's equivalent: run the
  ported project's own tests against the transpiled Go under the jsrt shim
  and report pass/fail vs the JS baseline.
- **Diffs are the worklist.** Their loop: port → build → run tests →
  baseline diffs appear → adopt them → `git diff testdata/**/*.diff` must
  SHRINK. "Your change is not correct unless diffs are reduced." ts2go's
  driver loop (compile findings) is the compile-level half of this; the
  behavior-level half is the test corpus above.
- **Pin the source.** The submodule pins the exact commit being ported;
  the JS line was feature-frozen during the port. Port a pinned snapshot of
  the source repo so reports are reproducible, then re-pin deltas.
- **Port in dependency order.** Parser → binder → checker (bottom-up);
  the port was usable the whole way. For ts2go: start with leaf packages
  (no imports) — each compiles green per v0.3 — then move up; the package
  report's cross-package wiring items point exactly at the next layer.
- **Keep both sides runnable.** They ran TS 6 and 7 side-by-side with a
  toggle, using the old implementation as the differential reference. The
  jsrt shim models JS semantics, so a transpiled program and the original
  can run the same inputs and be compared.
- **Agents are the porters, humans own the results.** They encode the
  porting protocol as Copilot instructions (fetch PR patch → translate →
  build → test → accept baselines → verify diffs reduced → commit) but
  ban bulk agent-driven PRs. The ts2go driver loop + report is the same
  protocol shape; keep a human (or operator) accountable per change-set.

## Roadmap

North star: an ESLint-class rewrite. Async is handled via the **jsrt shim**
(`shim.go`): goroutine-per-async-call, `await` parks the goroutine,
`Promise<T>` → `*jsrtPromise`, panics reject promises. The shim is emitted
into the output only when used, and its functions are seams for a later
native-concurrency pass. Next steps toward the north star:

1. ~~Multi-file~~ — **done (v0.2)**: directory/multi-file input, one Go
   package per directory, same-package imports dropped, sibling/external
   imports as work items, once-per-package jsrt.go, aggregate per-file
   report. Remaining: cross-package imports still need manual Go import
   wiring (that's the LLM's work item).
2. ~~Driver loop~~ — **done (v0.3)**: `-verify` builds the generated Go in a
   temp module (package main rewritten, go.mod with the installed toolchain
   version) and feeds every `go build` failure back as a per-file compile
   finding — in the report and appended to the generated file itself.
   Remaining: wire-up of the loop into an agent (LLM reads findings, fixes,
   re-runs); tsconfig `paths`/`baseUrl` resolution
3. **Test-corpus oracle + parity metric** — the Corsa insight applied:
   transpile a real project (and its tests) under the jsrt shim, run the
   tests, and report pass/fail against the JS baseline as one number (the
   equivalent of their "74/6,000" count). This makes the port dirigible:
   every transpiler change either reduces divergence or is reverted.
4. jsrt v1: serialized sync segments, microtask ordering, `.then` chains
5. Node-API surface library (fs, path) for real CLI tools
6. Tuple types → structs; object unions → sealed interface pattern
7. Generic constraints (`T extends {id: string}` → generated interface)
8. tsconfig support (paths, baseUrl) for import resolution beyond relative

