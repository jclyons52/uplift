# npm→Go counterpart registry (seed)

Which existing Go modules / stdlib cover an npm package vs which must be ported. Query live with: `uplift registry`, extend with `--overlay file.json`. Verdict `inline` = assessed-once tiny shim, do NOT split — get the code with `uplift shim <name>`.

```
NPM PACKAGE                          VERDICT   GO COUNTERPART                 NOTE
------------------------------------------------------------------------------------------------------------------------
@eslint-community/regexpp            port      (port)                          regex parser; port for parity
@humanwhocodes/module-importer       stdlib    (stdlib)                        dynamic import; Go has no need (compiled)
@humanwhocodes/object-schema         port      (port)                          schema merge/validation; port
acorn                                port      (port)                          JS parser; no equivalent
acorn-jsx                            port      (port)                          JSX extension; no equivalent
ajv                                  use_existing github.com/santhosh-tekuri/jsonschema  JSON-schema validation; mature fork or stdlib-adjacent
ansi-regex                           inline    (inline)                        DO NOT SPLIT; ANSI regex shim → `uplift shim ansi-regex`
ansi-styles                          stdlib    (stdlib)                        ANSI code table; const map
chalk                                use_existing github.com/fatih/color          ANSI styling; close behavior to chalk
commander                            use_existing github.com/spf13/cobra          CLI framework
cross-spawn                          inline    (inline)                        DO NOT SPLIT; os/exec wrap shim → `uplift shim cross-spawn`
debug                                port      (port)                          env-gated logger; small port or absorb
deep-is                              stdlib    (stdlib)                        deep equality; reflect.DeepEqual
doctrine                             port      (port)                          JSDoc parser; no equivalent
escape-string-regexp                 stdlib    (stdlib)                        regexp.QuoteMeta
eslint-scope                         port      (port)                          scope analysis; no equivalent
eslint-visitor-keys                  stdlib    (stdlib)                        node-type→key map; const
espree                               port      (port)                          JS parser; no faithful Go equivalent yet
esprima                              port      (port)                          JS parser; no equivalent
esquery                              port      (port)                          AST selector engine; no equivalent
esrecurse                            port      (port)                          AST walker; small port
estraverse                           port      (port)                          AST walker; small port
esutils                              port      (port)                          JS helpers; small port
fast-deep-equal                      use_existing reflect.DeepEqual               deep equality; stdlib if semantics match
fast-json-stable-stringify           inline    (inline)                        DO NOT SPLIT; sorted-key JSON shim → `uplift shim json-stable-stringify`
fast-levenshtein                     inline    (inline)                        DO NOT SPLIT; edit-distance shim → `uplift shim fast-levenshtein`
fastq                                port      (port)                          queue worker pool; port small
find-up                              stdlib    (stdlib)                        filepath walk up
flatted                              port      (port)                          safe JSON; port or use encoding/json
fs.realpath                          stdlib    (stdlib)                        filepath.EvalSymlinks
glob                                 use_existing github.com/bmatcuk/doublestar   glob matching; doublestar
glob-parent                          inline    (inline)                        DO NOT SPLIT; glob dir shim → `uplift shim glob-parent`
graphemer                            stdlib    (stdlib)                        grapheme clusters; small
ignore                               port      (port)                          gitignore matching; no stdlib
imurmurhash                          port      (port)                          murmur hash; tiny, port for byte parity
is-extglob                           inline    (inline)                        DO NOT SPLIT; extglob = subset of is-glob magic
is-glob                              inline    (inline)                        DO NOT SPLIT; glob-detect shim → `uplift shim is-glob`
is-path-inside                       inline    (inline)                        DO NOT SPLIT; path-contains shim → `uplift shim is-path-inside`
js-yaml                              use_existing gopkg.in/yaml.v3                feature-complete YAML; js-yaml API maps cleanly
json-stable-stringify                inline    (inline)                        DO NOT SPLIT; sorted-key JSON shim → `uplift shim json-stable-stringify`
json-stable-stringify-without-jsonify inline    (inline)                        DO NOT SPLIT; sorted-key JSON shim → `uplift shim json-stable-stringify` (verify byte parity if hashing)
level/fs                             use_existing                                 os/ioutil
levn                                 port      (port)                          argument type checking; no equivalent
locate-path                          stdlib    (stdlib)                        filepath walk up
lodash                               port      (port)                          huge surface; port only the used subset
lodash.merge                         port_partial (port)                          deep merge; no stdlib; small port
minimatch                            port      (port)                          glob matcher; port if behavior parity matters
ms                                   stdlib    (stdlib)                        duration parse/format; time.Duration
optionator                           port      (port)                          CLI option parser; flag is limited, port if parity
path-exists                          stdlib    (stdlib)                        os.Stat
path-key                             stdlib    (stdlib)                        os.Getenv PATH
prelude-ls                           port_partial (port)                          fp helpers; port used subset
queue-microtask                      stdlib    (stdlib)                        scheduler; goroutine
rimraf                               stdlib    (stdlib)                        os.RemoveAll
semver                               use_existing golang.org/x/mod/semver         official semver
strip-ansi                           inline    (inline)                        DO NOT SPLIT; ANSI-strip shim → `uplift shim strip-ansi`
supports-color                       inline    (inline)                        DO NOT SPLIT; term env check shim → `uplift shim supports-color`
text-table                           use_existing text/tabwriter                  column alignment; tabwriter is close
type-check                           port      (port)                          type strings; no equivalent
uuid                                 use_existing github.com/google/uuid          stable known module
which                                stdlib    (stdlib)                        exec.LookPath
word-wrap                            stdlib    (stdlib)                        text wrap; strings/wordwrap (golang.org/x/text)
yaml                                 use_existing gopkg.in/yaml.v3                YAML parse/dump; safe to use direct
yargs                                use_existing github.com/spf13/cobra          CLI; cobra or pflag
```
