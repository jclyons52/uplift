# ts2go

Convert TypeScript to Go — for the speed.

`ts2go` uses [ts-go-morph](https://github.com/jclyons52/ts-go-morph) (the Go
port of the TypeScript compiler) to build a **fully type-checked AST** of your
TypeScript, then emits idiomatic Go: interfaces → structs, enums → `iota`,
classes → structs + methods, and function bodies translated
statement-by-statement.

```bash
ts2go -o out.go src/app.ts
```

## Why

You have a TypeScript codebase that's too slow in Node and you want it in Go
without a manual rewrite. `ts2go` does the mechanical 90%: types, classes,
control flow, arithmetic — all mapped to Go idioms. You keep the small 10%
(hand-rolled hot loops, Go-specific optimizations) for yourself.

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

## What works today (v0)

| TypeScript | Go |
|---|---|
| `interface` (data) | `struct` |
| `interface` (methods) | Go `interface` |
| `class` | `struct` + methods, `extends` → embedding |
| `constructor(private x: T)` | field + `NewX()` constructor |
| `enum` | `int` + `iota` constants |
| `type X = ...` | Go type alias |
| `T[]`, `Array<T>` | `[]T` |
| `T \| null` | `*T` |
| string-literal unions | `any` (v1: sealed-interface/consts) |
| `if`/`else`, `for`, `while`, `do`, `switch` | same (switch gets a `panic` default) |
| `for (const x of arr)` | `for _, x := range arr` |
| `arr.push(x)` | `arr = append(arr, x)` |
| `s.length`, `s.toUpperCase()`, `s.includes()` | `len(s)`, `strings.ToUpper`, `strings.Contains` |
| template literals | `fmt.Sprintf` |
| `throw` | `panic` |
| `try`/`catch` | `defer` + `recover` |

## Banned (no faithful Go equivalent)

`ts2go` fails up front on these — the whole point is idiomatic Go, not
emulating JS dynamism:

- Prototype manipulation: `Object.create`, `Object.setPrototypeOf`,
  `__proto__`, `Reflect.setPrototypeOf`, `Reflect.construct`, `Reflect.apply`
- Dynamic dispatch: `eval`, `new Function`, `Proxy`
- `instanceof`, `in`, spread arguments, `with`

## Development

```bash
go build ./...        # build
go test ./...         # end-to-end tests (generated Go must compile)
go run ./cmd/ts2go testdata/bank.ts   # try it
```

## Roadmap

- String-literal unions → typed constants + validation
- Object unions → sealed-interface pattern
- `.map`/`.filter`/`.reduce` → Go loops
- Multi-file projects (package mapping, imports)
- Async/await → goroutines/channels
