# uplift

**Port TypeScript/JavaScript to Go and raise codebase quality — mechanically, safely, measurably.**

`uplift` started as `ts2go`, a TypeScript → Go transpiler. As the port
toolchain grew, the scope outgrew the name: it now spans the whole
"take a JS/TS library, measure it, lift it to typed TS, transpile it to Go,
port its dependencies as standalone Go libraries, and prove parity" pipeline.
The command that ties it all together is `uplift`: point it at a codebase and
it tells you its current state and what to do next, in a form both a human and
an agent can act on.

```
uplift <dir>          # full status + prioritized next actions (types → structure → decouple → tests → ports)
uplift <dir> --json   # the same truth, as a stable machine report (schema uplift/v1)
```

## The one-line pitch

`uplift` does the **mechanical 90%** of porting a codebase to Go, marks
precisely what it can't translate (so an LLM or a human gets a worklist, not a
mystery), and **measures the result** so you can prove each stage made things
better rather than trusting vibes. Every dependency becomes its own library
port in its own repo — *unless* it's too small to split (the `leftPad` class)
or too close to something Go's stdlib already does.

## The full process

The methodology is a repeatable six-stage loop. Each stage is small,
committable, and leaves the project strictly cleaner — and each has a command.

| # | Stage | Command | What it does |
|---|-------|---------|--------------|
| 1 | **Types, first and safe** | `uplift lift` | Annotate JS/JSDoc → typed TS (`tsc --strict`-clean). Types can't cause runtime errors, so this is the safest operation that exists. |
| 2 | **Measure** | `uplift measure` | Baseline the codebase's quality (type coverage, complexity, coupling, tests). Run again after every stage to see the delta. |
| 3 | **Transpile to Go** | `uplift <file.ts\|dir>` | Convert typed TS → idiomatic Go, with a type-checked AST and a `TODO(uplift)` worklist for anything mechanical translation can't map. |
| 4 | **Port dependencies** | `uplift deps`, `uplift registry`, `uplift scaffold` | Map the module graph, decide which npm leaves become their own Go library repos, and scaffold them (stub + parity-test shell + README + queue). |
| 5 | **Benchmark** | `uplift bench` | Compare the ported function vs node on the same inputs: wall time, memory, ops/sec. |
| 6 | **Status & next actions** | `uplift` | Compose measure + deps into a ranked plan of what to do next, in a stage-safe order. Re-run after each stage to track progress. |

The ordering matters: **types before structure, structure before decoupling,
decoupling before tests, tests before performance work.** Each step makes the
next lower-risk.

## CLI surface

```
uplift <file.ts|dir>            # TS -> Go transpile (one Go package per directory)
uplift lift js... -o out        # JS -> annotated TS via the checker
uplift deps <dir> [--json]      # module graph: leaves, size, exports, split-vs-absorb verdict
uplift registry [dir]           # npm -> Go counterpart registry (stdlib, first-party port, or port/absorb)
uplift scaffold <dir> --out R   # lay down a standalone library repo per "own-repo" leaf
uplift measure <dir> [--json]   # quality metrics (schema measure/v1); --compare diff two snapshots
uplift bench ...                # runtime comparator: node vs the Go port
uplift <dir> [--json]           # status + prioritized next actions (schema uplift/v1)
```

Every subcommand that reports structured data also accepts `--json` and emits
a stable schema, so an **agent** can consume the same truth a human reads.

## Types, first: `uplift lift`

`uplift lift` converts JavaScript (JSDoc-typed or not) to annotated TypeScript:
it asks the TypeScript checker for the resolved type of every parameter and
return value and inserts the annotations, leaving everything else
byte-for-byte. It's the JS→TS bridge for the pipeline — a codebase like ESLint
(100% JS + JSDoc) can be lifted to typed TS, then transpiled to Go without the
`any`/dynamic gap cascade.

- safe: the lifted output typechecks under `tsc --strict` with zero logic
  changes; types the checker can't resolve are left unannotated rather than forced
- JSDoc `@param {string}` → `name: string`; `@param {...number}` →
  `...args: number[]`; `@returns {T}` → `: T`
- contextually-typed callbacks: `arr.forEach((x) => …)` → `(x: T) => …`
- `-type-audit` lists every site still degraded to `any` where the checker
  resolves a concrete type — the exact "annotate here" list

## Measure: `uplift measure`

Comparable quality metrics per codebase (schema `measure/v1`):

- **type coverage** — annotated-sites ratio, with per-file worst-first sites
- **cyclomatic complexity** — total / avg / max + top files
- **coupling** — modules, edges, density, fan-out, hubs, cycles
- **tests** — test files and parity suites

`uplift measure --compare before.json after.json` diffs two snapshots into an
annotated delta — the progress report that makes "is it actually improving?"
answerable.

## Transpile: `uplift <file.ts|dir>`

The core engine. Uses [ts-go-morph](https://github.com/jclyons52/ts-go-morph)
(the Go port of the TypeScript compiler) to build a **fully type-checked AST**
of your TypeScript, then emits idiomatic Go: interfaces → structs, enums →
typed constants, classes → structs + methods, and function bodies translated
statement-by-statement. The output targets **LLM cleanup**: anything that
cannot be translated mechanically is emitted as a compiling `TODO(uplift)`
placeholder, listed in a post-work manifest, and summarized in an assessment
report. Every run (unless `-verify=false`) builds the generated Go in a temp
module and feeds every compiler failure back into the report.

```bash
uplift src/app.ts               # write app.go + print assessment summary
uplift -dry-run src/app.ts      # assess only: no output file
uplift src/                     # multi-file: one Go package per directory
uplift -o out/ -package mypkg src/
```

### The type-alias convention

TypeScript only has `number`; Go has a dozen integer types. Declare aliases in
your TS and `uplift` honors the **name**:

```ts
type uint8 = number;   // → Go's uint8
type Money = int64;    // → type Money = int64
```

Bare `number` becomes `float64`.

### What maps cleanly

| TypeScript | Go |
|---|---|
| `interface` (data) | `struct` |
| `interface` (methods) | Go `interface` |
| `class` | `struct` + methods, `extends` → embedding |
| `enum` (numeric) | `int` type + explicit constants |
| `"a" \| "b"` union alias | named `string` type + constants |
| `T[]`, `Array<T>`, `Record<K,V>` | `[]T`, `map[K]V` |
| `T \| null`, `x?: T` | `*T` |
| `for...of` | `for _, x := range arr` |
| `.map`/`.filter`/`.reduce` | typed loop IIFEs (flagged approx) |
| `.forEach(cb)` | inline for-range loop |
| `s.length` | `float64(len(s))` |
| template literals | `fmt.Sprintf` |
| `throw new Error(msg)` | `panic(errors.New(msg))` |
| `try`/`catch`/`finally` | `defer` + `recover` (flagged approx) |
| `async`/`await`/`Promise` | jsrt event-loop shim |

### Async: the jsrt shim

Async code transpiles against a small runtime shim emitted only when async
constructs are detected. `async function f()` → `func f() *jsrtPromise`,
`await e` → `jsrtAwait(e)`, `Promise.all/resolve/reject` and `setTimeout` get
shim equivalents. The shim is goroutine-per-async-call and deliberately
exposes seams (`jsrtAsync`, `jsrtAwait`, ...) so a later rewrite pass can swap
in native Go concurrency (channels, errgroup) without touching call sites.

## Port dependencies: `uplift deps`, `registry`, `scaffold`

The port strategy: **every JS dependency becomes its own library repo in a
separate repo** — but only if Go has no counterpart and it's worth splitting.

- `uplift deps <dir>` builds the module graph: which modules need each
  dependency, which external packages are **leaf nodes** (own their whole
  subtree), their size (LOC) and exported-symbol count, and a
  **split-vs-absorb** recommendation (too small / stdlib-covered → absorb;
  leaf library → own repo; big with children → split with its subtree).
- `uplift registry` prints the npm→Go counterpart registry: which existing Go
  modules or stdlib cover a package vs which must be ported. Extend with
  `--overlay file.json`.
- `uplift scaffold <dir> --out R` creates a standalone library repo per
  "own-repo" leaf: a compiling package stub, a parity-test shell, a README,
  and optionally the original source. `--queue` emits a machine-readable
  work queue (schema `port-queue/v1`) with resumable per-export units.

### Dependency freshness

Porting an out-of-date library is wasted effort — you'd port the old behavior
and immediately be behind. `uplift deps` reads the **installed (pinned)
version** from each dependency's `node_modules/.../package.json`, and with
`--check-updates` queries the npm registry for the latest published version,
flagging anything stale (`↯ vX install → latest vY`) before you decide to
port. The scaffold and port queue then lock against the current release.

## Benchmark: `uplift bench`

Runtime comparator for a single-argument pure function (string|int) across the
same inputs in node vs the Go port: wall time, memory, ops/sec. Uses a temp
Go module + `replace` directive to import the ported repo. Directional memory
comparison (TotalAlloc vs heap RSS); exact numbers in the reports.

## Post-work report

Every translation gap lands in one of three severities:

- **banned** — no idiomatic Go equivalent (`eval`, `Proxy`, `with`); needs redesign
- **todo** — a compiling placeholder was emitted; must be filled in
- **approx** — translated, but semantics approximate (try/catch, optional → pointers); verify by hand

The report scores the work (`TRIVIAL`→`VERY HIGH`) so you can gauge LLM
post-effort before committing.

## Development

```bash
go build ./...                 # build (binary: uplift)
go test ./...                  # unit + end-to-end + 7 oracle parity suites
gofmt -l .                     # must be empty
go run ./cmd/uplift testdata/bank.ts   # try it
```

The oracle parity suites transpile real ESLint/JS fixtures and diff the Go
output against the node original byte-for-byte — the red/green gate that turns
"does the port behave right?" into a checkable fact.

## Repos

| Repo | Role |
|------|------|
| `uplift` (this) | the toolchain: transpile, lift, deps, registry, scaffold, measure, bench, uplift status |
| `ts-go-morph` | shared foundation: Go port of the TS compiler/AST/type-checker |
| `esutils-go`, `humanwhocodes-object-schema-go` | completed library ports (parity-verified) |

See `ROADMAP.md` for the phased strategy, and `reports/` for the live
measurement reports (ESLint baseline, deps, port backlog/queue, bench).
