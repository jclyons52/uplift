# ts2go

Convert TypeScript to Go — for the speed.

`ts2go` uses [ts-go-morph](https://github.com/jclyons52/ts-go-morph) (the Go
port of the TypeScript compiler) to build a **fully type-checked AST** of your
TypeScript, then emits idiomatic Go: interfaces → structs, enums → typed
constants, classes → structs + methods, and function bodies translated
statement-by-statement.

The output targets **LLM cleanup**: anything that cannot be translated
mechanically is emitted as a compiling `TODO(ts2go)` placeholder, listed in a
post-work manifest embedded at the top of the generated file, and summarized
in an assessment report. The LLM gets its worklist handed to it — no hunting.

```bash
ts2go src/app.ts              # write app.go + print assessment summary
ts2go -dry-run src/app.ts     # assess only: complexity report, no output file
ts2go -report r.txt src/app.ts # also write the full report to r.txt
ts2go src/                    # multi-file: walk src/, one Go package per directory
ts2go -o out/ -package mypkg src/  # direct output + root package name
```

## Multi-file projects

Point `ts2go` at a directory (or several files) and it transpiles the whole
tree into Go, following Go's one-directory-one-package rule:

- every `.ts` file becomes a `.go` file in the same relative location,
  `package <dirname>` (root directory uses `-package` or the output dir name);
- imports between files of the same directory are **dropped** — the
  identifiers resolve directly in the shared Go package;
- imports into a sibling directory become *"wire up the Go import"* work
  items (the report names the target package);
- node_modules / stdlib imports become *"port or stub"* work items;
- the jsrt async shim is emitted **once** per package that uses it, as
  `jsrt.go`, instead of being duplicated into every file;
- the assessment report aggregates across all files with a per-file
  breakdown.

## Why

You have a TypeScript codebase that's too slow in Node and you want it in Go
without a manual rewrite. `ts2go` does the mechanical 90%: types, classes,
control flow, arithmetic — all mapped to Go idioms. Everything it can't map
is marked precisely, so an LLM (or you) finishes the remaining 10% with a
clear worklist instead of reverse-engineering the output.

## The type-alias convention

TypeScript only has `number`. Go has a dozen integer types. The trick:
declare aliases in your TS and `ts2go` honors the **name**:

```ts
type uint8 = number;   // → Go's uint8
type int64 = number;   // → Go's int64
type Money = int64;    // → type Money = int64
```

Any `number` typed via one of these aliases comes out as the real Go integer
type; bare `number` becomes `float64`.

## What maps cleanly

| TypeScript | Go |
|---|---|
| `interface` (data) | `struct` |
| `interface` (methods) | Go `interface` |
| `class` | `struct` + methods, `extends` → embedding |
| `constructor(private x: T)` | field + `NewX()` constructor |
| `enum` (numeric) | `int` type + explicit constants (initializers folded) |
| `enum` (string) | `string` type + constants |
| `"a" \| "b"` union alias | named `string` type + constants |
| `type X = ...` | Go type alias |
| `T[]`, `Array<T>`, `Record<K,V>` | `[]T`, `map[K]V` (type args converted recursively) |
| `T \| null` | `*T` |
| `x?: T` | `*T` (flagged approx) |
| `if`/`else`, `for`, `while`, `do`, `switch` | same (switch gets a `panic` default when needed) |
| `for (const x of arr)` | `for _, x := range arr` |
| `.map`/`.filter`/`.reduce` | typed loop IIFEs (flagged approx) |
| `.forEach(cb)` | inline for-range loop |
| `arr.push(x)` | `arr = append(arr, x)` |
| `s.length` | `float64(len(s))` |
| `s.toUpperCase()`, `s.includes()`, `s.substring(a,b)` | `strings.*`, slicing |
| `parseInt`/`parseFloat`/`isNaN` | `strconv`/`math` equivalents |
| template literals | `fmt.Sprintf` |
| `throw new Error(msg)` | `panic(errors.New(msg))` / `fmt.Errorf` |
| `try`/`catch`/`finally` | `defer` + `recover` (flagged approx) |
| `async`/`await`/`Promise` | jsrt event-loop shim (see below) |

## Async: the jsrt shim

Async code transpiles against a small runtime shim that is emitted into the
output file only when async constructs are detected:

- `async function f()` → `func f() *jsrtPromise { return jsrtAsync(func() any { ... }) }`
- `await e` → `jsrtAwait(e)` (parks the async-call goroutine)
- `Promise.resolve/reject/all`, `setTimeout` → shim equivalents
- a `throw` inside an async body rejects the promise; `await` re-panics at
  the await site — JS-like control flow, portable 1:1

The shim is goroutine-per-async-call, so it deliberately exposes seams
(`jsrtAsync`, `jsrtAwait`, ...) that a later rewrite pass can swap for native
Go concurrency (channels, errgroup, worker pools) without touching the
transpiled call sites. This is what makes ambitious rewrites tractable: the
port runs first, the concurrency model is a separate, mechanical follow-up.

## Post-work report

Every gap lands in one of three severities:

- **banned** — no idiomatic Go equivalent (prototype manipulation, `eval`,
  `Proxy`, `with`); needs redesign, not translation
- **todo** — a compiling placeholder was emitted; must be filled in
  (async/await, `instanceof`, spread, dynamic access on `any`, unknown
  imports, ...)
- **approx** — translated, but semantics are approximate (try/catch via
  recover, optional properties → pointers, forEach → loops); verify by hand

The report scores the work (`TRIVIAL`/`LOW`/`MEDIUM`/`HIGH`/`VERY HIGH`) so
you can gauge LLM post-effort before committing:

```
$ ts2go -dry-run src/api.ts
ts2go assessment: /src/api.ts
  TS source:      412 lines, 38 top-level declarations
  Work items:     11 (1 banned, 4 todo, 6 approx)
  Complexity:     HIGH (score 28)

  Work items (grouped by line):
       6  [approx] optional     optional property retries mapped to pointer
      14  [todo  ] async        await: synchronous value used; ...
      26  [banned] dynamic      banned: Object.setPrototypeOf (...)
```

## Development

```bash
go build ./...        # build
go test ./...         # end-to-end tests (generated Go must compile)
go run ./cmd/ts2go testdata/bank.ts   # try it
```

## Roadmap

North star: an ESLint-class rewrite. The pieces needed to get there:

- Multi-file projects: package mapping, import/export graph, per-file output
  and per-project reports
- Node-API surface (fs, path, module resolution) as a typed support library
- Compile-error feedback loop: driver feeds `go build` errors back as extra
  work items
- jsrt v1: serialized synchronous segments (JS single-thread semantics),
  microtask ordering, `then` chains
- Tuples → structs, object unions → sealed interfaces
- Generated constraint interfaces for `T extends { ... }`
