# uplift reports

- `eslint-deps.md` — module-graph / leaf-node analysis (`uplift deps fmt/...`)
- `eslint-port-backlog.md` — the 21 leaf repos to port (registry-aware; excludes packages Go already covers)
- `npm-go-counterparts.md` — the npm→Go counterpart registry index (`uplift registry`)

## Completed ports
- **esutils** → `github.com/jclyons52/esutils-go`
  PARITY PASS: **111,949 cases, 0 mismatches**.
- **@humanwhocodes/object-schema** → `github.com/jclyons52/humanwhocodes-object-schema-go`
  PARITY PASS: **26 cases, 0 mismatches** (object merge/validation schema behind ESLint flat-config).

## Performance (uplift bench)
- **esutils `IsIdentifierNameES6`** Go vs node: **1.61x faster**, ~0.2MB Go alloc vs ~49MB node peak RSS (400k iters x 7 inputs). Full: `reports/esutils-bench.md`.
