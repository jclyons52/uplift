# Inline shims — assessed once, do not split

Tiny npm leaves in the counterpart registry carry the verdict **`inline`**:
they are too small / too stdlib-adjacent to warrant their own Go repo. The
judgment is made **once**, here, and encoded in `internal/deps/counterparts.json` —
so it is never re-derived per consumer or per port.

To retrieve the canonical Go snippet to copy into a consumer:

```
uplift shim <name>      # print one snippet
uplift shim             # list all available shims + their do-not-split note
```

The snippets themselves live in `internal/deps/shims.go` (`InlineShims`) and are
asserted consistent with the registry by `shims_test.go` (every `inline` verdict
that references a shim must have it, and every shim must be backed by an
`inline` verdict).

## Available shims

| npm leaf                 | inlines as                              |
|--------------------------|-----------------------------------------|
| `strip-ansi`             | ANSI-escape stripper                    |
| `ansi-regex`             | CSI color-code regexp                   |
| `cross-spawn`            | `os/exec` + PATH-resolved `Command`     |
| `is-path-inside`         | `filepath.Rel` prefix containment       |
| `fast-levenshtein`       | edit distance                           |
| `glob-parent`            | directory portion of a glob            |
| `is-glob` (+`is-extglob`) | glob-magic detection                   |
| `supports-color`         | TERM / NO_COLOR check                   |
| `json-stable-stringify`  | sorted-key JSON encoder (+without-jsonify / fast-json-stable-stringify) |

## Rule of thumb

If a leaf's whole behavior collapses to a few stdlib calls (like `@nodelib/fs.stat`
→ `os.Lstat`/`os.Stat`), it belongs here as an `inline` shim or a `stdlib`
verdict — **not** in the port backlog. Reserve `port` for packages with real,
non-trivial logic that Go's stdlib doesn't already provide.
