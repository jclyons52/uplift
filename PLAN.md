# ts2go — TypeScript → Go transpiler

**Goal:** convert a TypeScript codebase to idiomatic Go for speed, using
ts-go-morph (Go port of the TS compiler) for a type-checked AST. The output
should compile and preserve behavior for a well-defined subset; everything
that cannot map to idiomatic Go is either banned up front or emits a clear
error.

## Core insight: syntax-first, checker-second

The spike (2026-08-16) proved the two representations are complementary:

- **Syntax tree preserves names.** `type uint8 = number` + `x: uint8` — the
  checker collapses this to `number` (AliasSymbol is empty for primitive
  aliases), but the syntax node still says `uint8`. So the transpiler reads
  *written* type annotations for names, and only falls back to the checker
  where the syntax is insufficient.
- **Checker resolves semantics.** Conditional types (`A extends B ? C : D`),
  mapped types, generics, unions, nullability — all pre-computed by the
  TypeScript checker. `type Cond = Mapped["a"] extends uint8 ? true : false`
  resolves to `true` with zero work on our side.

## Type mapping table (v0)

| TS | Go | Notes |
|---|---|---|
| `number` | `float64` | bare number = JS double |
| `type uint8 = number` | `uint8` | **the convention**: alias NAME wins |
| `type int64 = number` | `int64` | same |
| `type Money = int64` | `type Money = int64` | chains through the fixpoint |
| `string` / `boolean` | `string` / `bool` | |
| `interface` (data only) | `struct` | fields exported (PascalCase) |
| `interface` (methods) | Go `interface` | method signatures |
| `class X` | `type X struct` + methods | `extends` → embedded field |
| `constructor(private name: string)` | `Name string` field + `NewX(name)` | param properties become fields + assignments |
| `enum` (numeric) | `type X int` + `iota` consts | `XUnspecified` first |
| `enum` (string) | `type X int` + string consts | v0 maps to int; revisit |
| `type X = T` | `type X = T` | skip if T is a Go builtin (recursive) |
| `T[]` / `Array<T>` | `[]T` | |
| `Record<K,V>` | `map[K]V` | |
| `T \| null` / `T \| undefined` | `*T` | pointer for nullability |
| string-literal union | `any` + comment | v1: consts + named type |
| object union | `any` + comment | v1: sealed interface |
| `T extends { id: string }` (generic constraint) | `any` constraint | v1: generated constraint interface |
| tuple | `any` | v1: struct |
| `void` | (no return type) | |
| `any`/`unknown` | `any` | |

## Statement mapping (v0)

| TS | Go |
|---|---|
| `const x = e` | `x := e` |
| `let x: T = e` | `var x T = e` |
| `if/else` | `if/else` (blocks un-wrapped) |
| `for (i; c; i++)` | `for i; c; i++` |
| `for (const x of arr)` | `for _, x := range arr` |
| `for (const k in obj)` | `for k := range obj` |
| `while` | `for c` |
| `do/while` | `for { ... if !(c) { break } }` |
| `switch` | `switch` + `default: panic` when no default |
| `return e` | `return e` |
| `throw e` | `panic(e)` |
| `try/catch` | `defer`+`recover` IIFE |
| `arr.push(x)` (statement) | `arr = append(arr, x)` |
| `s.length` | `len(s)` |
| `console.log` | `fmt.Println` |
| `s.toUpperCase()` etc. | `strings.*` |
| `a ** b` | `math.Pow(a, b)` |
| `a + b` (string operands) | `fmt.Sprintf("%s%s", a, b)` |
| `` `${a} x ${b}` `` | `fmt.Sprintf("%v x %v", a, b)` |
| `a === b` / `!==` | `a == b` / `!=` |
| `a ?? b` | nil-guard IIFE |
| ternary | IIFE returning `any` |
| object literal → typed var | `var x T = T{...}` |
| `this.x` (in method) | `r.x` |

## Ban list (checked before transpiling)

Prototype/dynamic-dispatch mechanisms with no idiomatic Go equivalent:

- `Object.create`, `Object.setPrototypeOf`, `Object.defineProperty`
- `Reflect.setPrototypeOf`, `Reflect.construct`, `Reflect.apply`,
  `Reflect.defineProperty`
- `__proto__`, `X.prototype.member = ...` (AST-level check)
- `eval(`, `new Function`, `new Proxy`
- `instanceof`, `in` operator, spread args (v0)

## Architecture

```
cmd/ts2go/main.go          CLI: read TS → Transpile() → gofmt → write
internal/transpile/
  transpile.go             Transpiler, goType (syntax-first type mapping),
                           alias collection (fixpoint), primitive-alias table
  decls.go                 interfaces, classes, enums, aliases, functions,
                           variable statements, signatures
  statements.go            statement + expression emitters (bodies)
  ban.go                   ban-list checks (text + AST)
  writer.go                indent-aware Go writer
internal/tswriter/         reusable writer (may fold into transpile)
testdata/bank.ts           end-to-end fixture
```

Key implementation notes:

- **Preamble emitted last**: body is written first, imports are computed from
  `tr.used` so only referenced stdlib packages are declared (avoids unused
  imports).
- **Operator extraction**: ts-go-morph's `OperatorText()` is unreliable
  (`IsToken` treats identifiers as tokens → returns the left operand). We
  slice the source between `left.End()` and `right.Pos()` instead.
- **Object literals**: the checker types them as anonymous `__object`; the
  contextual type is only known at the declaration site, so
  `emitVariableStatement` handles `var x T = T{...}` directly.
- **Return types**: declared return-type node is preferred over the checker's
  text so aliases (`Money`) survive (`signatureFromChildren`).
- **Builtin aliases skipped**: `type uint8 = uint8` would be recursive; when
  the alias name IS a Go builtin, no declaration is emitted and references
  resolve to the builtin.
- **for-of loop var**: digs through VariableDeclarationList →
  VariableDeclaration → name (`loopVarFrom`).
- **switch**: Go has no fallthrough; a missing `default` gets
  `panic("unhandled switch case")` so functions with switch returns compile.

## Testing

`transpile_test.go` runs end-to-end and **compiles the generated Go** in a
temp module (GOWORK=off) — the acceptance bar is that output must build.
`TestBankEndToEnd` exercises the alias convention, classes, param properties,
loops, templates, switch. `TestBanList` verifies prototype tricks are
rejected.

## Roadmap

1. String-literal unions → named string type + consts (v1)
2. Object unions → sealed-interface pattern
3. `.map`/`.filter`/`.reduce` → Go loops
4. Multi-file: package mapping, imports/exports
5. Generics constraints (`T extends {id: string}` → generated interface)
6. Async/await → goroutines/channels
7. Tuple types → structs
