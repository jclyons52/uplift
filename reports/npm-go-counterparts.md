# npm→Go counterpart registry (seed)

Which existing Go modules / stdlib cover an npm package vs which must be ported.
Query live with: `ts2go registry`, extend with `--overlay file.json`.

```
NPM PACKAGE                          VERDICT   GO COUNTERPART                 NOTE
------------------------------------------------------------------------------------------------------------------------
@eslint-community/regexpp            port      (port)                         *regex parser; port for parity
@humanwhocodes/module-importer       stdlib    (stdlib)                       *dynamic import; Go has no need (compiled)
@humanwhocodes/object-schema         port      (port)                         *schema merge/validation; port
acorn                                port      (port)                         *JS parser; no equivalent
acorn-jsx                            port      (port)                         *JSX extension; no equivalent
ajv                                  use_existing github.com/santhosh-tekuri/jsonschema *JSON-schema validation; mature fork or stdlib-adjacent
ansi-regex                           stdlib    (stdlib)                       *ANSI regex; regexp
ansi-styles                          stdlib    (stdlib)                       *ANSI code table; const map
chalk                                use_existing github.com/fatih/color         *ANSI styling; close behavior to chalk
commander                            use_existing github.com/spf13/cobra          CLI framework
cross-spawn                          stdlib    (stdlib)                       *os/exec wrap; absorb as helper
debug                                port      (port)                         *env-gated logger; small port or absorb
deep-is                              stdlib    (stdlib)                       *deep equality; reflect.DeepEqual
doctrine                             port      (port)                         *JSDoc parser; no equivalent
escape-string-regexp                 stdlib    (stdlib)                       *regexp.QuoteMeta
eslint-scope                         port      (port)                         *scope analysis; no equivalent
eslint-visitor-keys                  stdlib    (stdlib)                       *node-type→key map; const
espree                               port      (port)                         *JS parser; no faithful Go equivalent yet
esprima                              port      (port)                          JS parser; no equivalent
esquery                              port      (port)                         *AST selector engine; no equivalent
esrecurse                            port      (port)                         *AST walker; small port
estraverse                           port      (port)                         *AST walker; small port
esutils                              port      (port)                         *JS helpers; small port
fast-deep-equal                      use_existing reflect.DeepEqual              *deep equality; stdlib if semantics match
fast-json-stable-stringify           stdlib    (stdlib)                       *sorted JSON; sort keys
fast-levenshtein                     stdlib    (stdlib)                       *edit distance; small algorithm
fastq                                port      (port)                         *queue worker pool; port small
find-up                              stdlib    (stdlib)                       *filepath walk up
flatted                              port      (port)                         *safe JSON; port or use encoding/json
fs.realpath                          stdlib    (stdlib)                       *filepath.EvalSymlinks
glob                                 use_existing github.com/bmatcuk/doublestar  *glob matching; doublestar
glob-parent                          stdlib    (stdlib)                       *path parent of glob; small
graphemer                            stdlib    (stdlib)                       *grapheme clusters; small
ignore                               port      (port)                         *gitignore matching; no stdlib
imurmurhash                          port      (port)                         *murmur hash; tiny, port for byte parity
is-extglob                           stdlib    (stdlib)                       *tiny
is-glob                              stdlib    (stdlib)                       *glob detection; small
is-path-inside                       stdlib    (stdlib)                       *filepath.Rel prefix check
js-yaml                              use_existing gopkg.in/yaml.v3               *feature-complete YAML; js-yaml API maps cleanly
json-stable-stringify                stdlib    (stdlib)                        sorted JSON; sort keys
json-stable-stringify-without-jsonify stdlib    (stdlib)                       *sorted JSON; sort keys
level/fs                             use_existing                                 os/ioutil
levn                                 port      (port)                         *argument type checking; no equivalent
locate-path                          stdlib    (stdlib)                       *filepath walk up
lodash                               port      (port)                          huge surface; port only the used subset
lodash.merge                         port_partial (port)                         *deep merge; no stdlib; small port
minimatch                            port      (port)                         *glob matcher; port if behavior parity matters
ms                                   stdlib    (stdlib)                       *duration parse/format; time.Duration
optionator                           port      (port)                         *CLI option parser; flag is limited, port if parity
path-exists                          stdlib    (stdlib)                       *os.Stat
path-key                             stdlib    (stdlib)                       *os.Getenv PATH
prelude-ls                           port_partial (port)                         *fp helpers; port used subset
queue-microtask                      stdlib    (stdlib)                       *scheduler; goroutine
rimraf                               stdlib    (stdlib)                       *os.RemoveAll
semver                               use_existing golang.org/x/mod/semver         official semver
strip-ansi                           stdlib    (stdlib)                       *strip ANSI escape codes; rune scan
supports-color                       stdlib    (stdlib)                       *term env check; os+isatty, few lines
text-table                           use_existing text/tabwriter                 *column alignment; tabwriter is close
type-check                           port      (port)                         *type strings; no equivalent
uuid                                 use_existing github.com/google/uuid          stable known module
which                                stdlib    (stdlib)                       *exec.LookPath
word-wrap                            stdlib    (stdlib)                       *text wrap; strings/wordwrap (golang.org/x/text)
yaml                                 use_existing gopkg.in/yaml.v3                YAML parse/dump; safe to use direct
yargs                                use_existing github.com/spf13/cobra          CLI; cobra or pflag

* referenced by this codebase
```
