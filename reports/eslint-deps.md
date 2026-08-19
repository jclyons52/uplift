# ESLint dependency / leaf-node analysis

Generated with `ts2go deps lib` — module-graph + leaf-node + size + exports + split-vs-absorb recommendations.

```
Module graph for /tmp/eslint-inspect/lib

Internal files: 388
Builtins: 8
External packages: 98 (52 leaves)

=== EXTERNAL DEPENDENCIES ===
DEP                          KIND    LEAF   LOC     EXP   NEEDED-BY                          RECOMMEND
------------------------------------------------------------------------------------------------------------------------------------
eslint                       external no     0       5     @eslint-community/eslint-utils     split as a repo WITH its subtree (owns children)
globals                      external no     6       0     @eslint/eslintrc +3                split as a repo WITH its subtree (owns children)
queue-microtask              external yes    10      0     run-parallel                       Go stdlib covers this — scheduler; goroutine
shebang-command              external no     14      0     cross-spawn                        split as a repo WITH its subtree (owns children)
path-is-absolute             external yes    16      2     glob                               too small to split (leftPad-class) — absorb/inline; not worth a repo
strip-ansi                   external no     16      0     cli-engine/formatters/stylish.js +1 Go stdlib covers this — strip ANSI escape codes; rune scan
is-extglob                   external yes    17      0     is-glob                            Go stdlib covers this — tiny
shebang-regex                external yes    18      0     shebang-command                    too small to split (leftPad-class) — absorb/inline; not worth a repo
escape-string-regexp         external yes    24      0     cli-engine/file-enumerator.js +5   Go stdlib covers this — regexp.QuoteMeta
parent-module                external no     28      0     import-fresh                       split as a repo WITH its subtree (owns children)
wrappy                       external yes    29      0     once +1                            too small to split (leftPad-class) — absorb/inline; not worth a repo
is-path-inside               external yes    30      0     eslint/eslint-helpers.js +1        Go stdlib covers this — filepath.Rel prefix check
@humanwhocodes/module-importer external yes    33      1     cli.js +1                          Go stdlib covers this — dynamic import; Go has no need (compiled)
has-flag                     external yes    35      0     supports-color                     too small to split (leftPad-class) — absorb/inline; not worth a repo
ansi-regex                   external yes    36      0     strip-ansi                         Go stdlib covers this — ANSI regex; regexp
inherits                     external yes    36      0     glob                               too small to split (leftPad-class) — absorb/inline; not worth a repo
once                         external no     38      1     inflight +1                        split as a repo WITH its subtree (owns children)
resolve-from                 external yes    39      1     import-fresh                       too small to split (leftPad-class) — absorb/inline; not worth a repo
path-exists                  external yes    42      1     find-up                            Go stdlib covers this — os.Stat
path-key                     external yes    44      1     cross-spawn                        Go stdlib covers this — os.Getenv PATH
run-parallel                 external no     44      0     @nodelib/fs.scandir                split as a repo WITH its subtree (owns children)
natural-compare              external yes    46      0     eslint +1                          small leaf (0 exports, 46 LOC) — absorb or one-file lib
import-fresh                 external no     48      0     @eslint/eslintrc                   split as a repo WITH its subtree (owns children)
inflight                     external no     48      0     glob                               split as a repo WITH its subtree (owns children)
balanced-match               external yes    52      0     brace-expansion                    small leaf (0 exports, 52 LOC) — absorb or one-file lib
concat-map                   external yes    53      0     brace-expansion                    small leaf (0 exports, 53 LOC) — absorb or one-file lib
glob-parent                  external no     62      0     cli-engine/file-enumerator.js +2   Go stdlib covers this — path parent of glob; small
p-locate                     external no     81      0     locate-path                        split as a repo WITH its subtree (owns children)
callsites                    external yes    88      1     parent-module                      small leaf (1 exports, 88 LOC) — absorb or one-file lib
p-limit                      external no     89      0     p-locate                           split as a repo WITH its subtree (owns children)
word-wrap                    external yes    94      0     optionator                         Go stdlib covers this — text wrap; strings/wordwrap (golang.org/x/text)
strip-json-comments          external yes    95      0     @eslint/eslintrc                   small leaf (0 exports, 95 LOC) — absorb or one-file lib
yocto-queue                  external yes    95      0     p-limit                            small leaf (0 exports, 95 LOC) — absorb or one-file lib
which                        external no     102     0     cross-spawn                        Go stdlib covers this — exec.LookPath
json-buffer                  external yes    106     2     keyv                               small leaf (2 exports, 106 LOC) — absorb or one-file lib
fast-levenshtein             external yes    114     0     optionator                         Go stdlib covers this — edit distance; small algorithm
supports-color               external no     115     5     chalk                              Go stdlib covers this — term env check; os+isatty, few lines
imurmurhash                  external yes    117     0     cli-engine/hash.js +1              tiny hash — absorb
locate-path                  external no     119     1     find-up                            Go stdlib covers this — filepath walk up
is-glob                      external no     132     0     glob-parent +3                     Go stdlib covers this — glob detection; small
deep-is                      external yes    145     0     optionator                         Go stdlib covers this — deep equality; reflect.DeepEqual
color-name                   external yes    151     297   color-convert                      small leaf library (297 exports) — port as its own repo (candidate)
ms                           external yes    151     0     debug                              Go stdlib covers this — duration parse/format; time.Duration
reusify                      external yes    166     0     fastq                              small leaf (0 exports, 166 LOC) — absorb or one-file lib
@nodelib/fs.stat             external yes    172     3     @nodelib/fs.scandir                small leaf library (3 exports) — port as its own repo (candidate)
find-up                      external no     175     3     eslint/flat-eslint.js +1           Go stdlib covers this — filepath walk up
esrecurse                    external no     184     3     eslint-scope                       split as a repo WITH its subtree (owns children)
fast-deep-equal              external yes    190     0     ajv +3                             use existing Go: reflect.DeepEqual — deep equality; stdlib if semantics match
fast-json-stable-stringify   external yes    201     0     ajv                                Go stdlib covers this — sorted JSON; sort keys
cross-spawn                  external no     239     4     eslint +1                          Go stdlib covers this — os/exec wrap; absorb as helper
text-table                   external yes    239     0     cli-engine/formatters/stylish.js +1 use existing Go: text/tabwriter — column alignment; tabwriter is close
file-entry-cache             external no     247     5     cli-engine/lint-result-cache.js +1 split as a repo WITH its subtree (owns children)
flat-cache                   external no     247     27    file-entry-cache                   split as a repo WITH its subtree (owns children)
json-stable-stringify-without-jsonify external yes    291     0     cli-engine/lint-result-cache.js +1 Go stdlib covers this — sorted JSON; sort keys
@eslint/js                   external yes    296     1     cli-engine/cli-engine.js +3        small leaf (1 exports, 296 LOC) — absorb or one-file lib
fs.realpath                  external yes    314     2     glob                               Go stdlib covers this — filepath.EvalSymlinks
keyv                         external no     315     0     flat-cache                         split as a repo WITH its subtree (owns children)
isexe                        external yes    318     0     which                              leaf library — port as its own repo
json-schema-traverse         external yes    331     55    ajv                                leaf library — port as its own repo
type-check                   external no     342     4     levn +1                            split as a repo WITH its subtree (owns children)
brace-expansion              external no     344     0     minimatch                          split as a repo WITH its subtree (owns children)
rimraf                       external no     372     0     flat-cache                         Go stdlib covers this — os.RemoveAll
flatted                      external yes    374     4     flat-cache                         leaf library — port as its own repo
@nodelib/fs.scandir          external no     376     3     @nodelib/fs.walk                   split as a repo WITH its subtree (owns children)
eslint-visitor-keys          external yes    394     3     espree +5                          Go stdlib covers this — node-type→key map; const
@humanwhocodes/object-schema external yes    395     3     @humanwhocodes/config-array        leaf library — port as its own repo
esutils                      external yes    409     3     doctrine +3                        port as leaf lib (JS util) — own repo
ansi-styles                  external no     449     0     chalk                              Go stdlib covers this — ANSI code table; const map
levn                         external no     462     3     linter/config-comment-parser.js +2 split as a repo WITH its subtree (owns children)
@nodelib/fs.walk             external no     495     4     eslint/eslint-helpers.js +1        split as a repo WITH its subtree (owns children)
@ungap/structured-clone      external yes    609     2     config/flat-config-schema.js +1    leaf library — port as its own repo
chalk                        external no     669     2     cli-engine/formatters/stylish.js +1 use existing Go: github.com/fatih/color — ANSI styling; close behavior to chalk
acorn-jsx                    external no     686     253   espree                             split as a repo WITH its subtree (owns children)
debug                        external no     735     11    @eslint/eslintrc +13               port as leaf lib (env-gated logging) or absorb as helper
estraverse                   external yes    756     8     esrecurse +2                       leaf library — port as its own repo
optionator                   external no     779     6     options.js +1                      split as a repo WITH its subtree (owns children)
espree                       external no     783     8     @eslint/eslintrc +5                port as leaf lib (JS parser) — own repo
color-convert                external no     805     0     ansi-styles                        split as a repo WITH its subtree (owns children)
@humanwhocodes/config-array  external no     837     2     config/flat-config-array.js +2     split as a repo WITH its subtree (owns children)
minimatch                    external no     852     0     @eslint/eslintrc +5                split as a repo WITH its subtree (owns children)
ignore                       external yes    1102    1     @eslint/eslintrc +3                leaf library — port as its own repo
glob                         external no     1247    7     rimraf                             use existing Go: github.com/bmatcuk/doublestar — glob matching; doublestar
fastq                        external no     1290    1     @nodelib/fs.walk                   split as a repo WITH its subtree (owns children)
prelude-ls                   external yes    1339    126   levn +2                            leaf library — port as its own repo
lodash.merge                 external yes    1819    1     linter/linter.js +2                leaf library — port as its own repo
eslint-scope                 external no     1931    9     linter/linter.js +2                port as leaf lib (scope analysis) — own repo
doctrine                     external no     1941    8     eslint +1                          split as a repo WITH its subtree (owns children)
type-fest                    external yes    2311    35    globals                            leaf library — port as its own repo
uri-js                       external yes    2390    12    ajv                                leaf library — port as its own repo
@eslint-community/eslint-utils external no     2617    37    eslint +18                         split as a repo WITH its subtree (owns children)
argparse                     external yes    3691    18    js-yaml                            leaf library — port as its own repo
@eslint/eslintrc             external no     4081    2     cli-engine/cli-engine.js +8        split as a repo WITH its subtree (owns children)
@eslint-community/regexpp    external yes    4184    7     eslint +10                         port as leaf lib (regex parser) — own repo
js-yaml                      external no     6248    14    @eslint/eslintrc +2                use existing Go: gopkg.in/yaml.v3 — feature-complete YAML; js-yaml API maps cleanly
acorn                        external yes    6436    25    espree +1                          leaf library — port as its own repo
ajv                          external no     12142   75    @eslint/eslintrc +2                use existing Go: github.com/santhosh-tekuri/jsonschema — JSON-schema validation; mature fork or stdlib-adjacent
graphemer                    external yes    12342   1     eslint +1                          Go stdlib covers this — grapheme clusters; small
esquery                      external no     15022   9     linter/node-event-generator.js +1  port as leaf lib (selector engine) — own repo

=== NODE BUILTINS USED ===
  node:fs                  needed by cli-engine/cli-engine.js +7
  node:path                needed by cli-engine/cli-engine.js +12
  node:node:punycode       needed by uri-js
  node:assert              needed by cli-engine/lint-result-cache.js +6
  node:util                needed by cli.js +5
  node:url                 needed by eslint/flat-eslint.js
  node:module              needed by shared/relative-module-resolver.js
  node:os                  needed by shared/runtime-info.js

=== INTERNAL COUPLING (388 files) ===
FILE                                           LOC      IMPORTS
  api.js                                       44       eslint/index.js, eslint/flat-eslint.js, linter/index.js, rule-tester/index.js, source-code/index.js
  cli-engine/cli-engine.js                     939      node:fs, node:path, /tmp/eslint-inspect/conf/default-cli-options.js, /tmp/eslint-inspect/package.json, @eslint/eslintrc, cli-engine/file-enumerator.js, linter/index.js, rules/index.js …(+5)
  cli-engine/file-enumerator.js                477      node:fs, node:path, glob-parent, is-glob, escape-string-regexp, minimatch, @eslint/eslintrc, debug …(+1)
  cli-engine/formatters/checkstyle.js          45       cli-engine/xml-escape.js
  cli-engine/formatters/compact.js             44       —
  cli-engine/formatters/html.js                308      —
  cli-engine/formatters/jslint-xml.js          29       cli-engine/xml-escape.js
  cli-engine/formatters/json-with-metadata.js  14       —
  cli-engine/formatters/json.js                11       —
  cli-engine/formatters/junit.js               66       cli-engine/xml-escape.js, node:path
  cli-engine/formatters/stylish.js             84       chalk, strip-ansi, text-table
  cli-engine/formatters/tap.js                 80       js-yaml
  cli-engine/formatters/unix.js                42       —
  cli-engine/formatters/visualstudio.js        46       —
  cli-engine/hash.js                           27       imurmurhash
  cli-engine/index.js                          5        cli-engine/cli-engine.js
  cli-engine/lint-result-cache.js              170      node:assert, node:fs, file-entry-cache, json-stable-stringify-without-jsonify, /tmp/eslint-inspect/package.json, cli-engine/hash.js, debug
  cli-engine/load-rules.js                     36       node:fs, node:path
  cli-engine/xml-escape.js                     32       —
  cli.js                                       397      node:fs, node:path, node:util, eslint/index.js, eslint/flat-eslint.js, options.js, shared/logging.js, shared/runtime-info.js …(+4)
  config/default-config.js                     58       rules/index.js, espree
  config/flat-config-array.js                  218      @humanwhocodes/config-array, config/flat-config-schema.js, config/rule-validator.js, config/default-config.js, @eslint/js
  config/flat-config-helpers.js                89       —
  config/flat-config-schema.js                 493      @ungap/structured-clone, shared/severity.js
  config/rule-validator.js                     124      shared/ajv.js, config/flat-config-helpers.js, /tmp/eslint-inspect/conf/replacements.json
  eslint/eslint-helpers.js                     771      node:path, node:fs, is-glob, cli-engine/hash.js, minimatch, node:util, @nodelib/fs.walk, glob-parent …(+1)
  eslint/eslint.js                             629      node:path, node:fs, node:util, cli-engine/cli-engine.js, rules/index.js, @eslint/eslintrc, /tmp/eslint-inspect/package.json
  eslint/flat-eslint.js                        967      node:fs, node:path, find-up, /tmp/eslint-inspect/package.json, linter/index.js, config/flat-config-helpers.js, @eslint/eslintrc, eslint/eslint-helpers.js …(+4)
  eslint/index.js                              7        eslint/eslint.js, eslint/flat-eslint.js
  linter/apply-disable-directives.js           399      escape-string-regexp
  linter/code-path-analysis/code-path-analyzer.js 726      node:assert, shared/ast-utils.js, linter/code-path-analysis/code-path.js, linter/code-path-analysis/code-path-segment.js, linter/code-path-analysis/id-generator.js, linter/code-path-analysis/debug-helpers.js
  linter/code-path-analysis/code-path-segment.js 224      linter/code-path-analysis/debug-helpers.js
  linter/code-path-analysis/code-path-state.js 2015     linter/code-path-analysis/code-path-segment.js, linter/code-path-analysis/fork-context.js
  linter/code-path-analysis/code-path.js       297      linter/code-path-analysis/code-path-state.js, linter/code-path-analysis/id-generator.js
  linter/code-path-analysis/debug-helpers.js   169      debug
  linter/code-path-analysis/fork-context.js    306      node:assert, linter/code-path-analysis/code-path-segment.js
  linter/code-path-analysis/id-generator.js    37       —
  linter/config-comment-parser.js              150      levn, @eslint/eslintrc, shared/directives.js, debug
  linter/index.js                              10       linter/linter.js, linter/interpolate.js, linter/source-code-fixer.js
  linter/interpolate.js                        22       —
  linter/linter.js                             1825     node:path, eslint-scope, eslint-visitor-keys, espree, lodash.merge, /tmp/eslint-inspect/package.json, shared/ast-utils.js, shared/directives.js …(+19)
  linter/node-event-generator.js               303      esquery
  linter/report-translator.js                  315      node:assert, linter/rule-fixer.js, linter/interpolate.js
  linter/rule-fixer.js                         122      —
  linter/rules.js                              67       rules/index.js
  linter/safe-emitter.js                       47       —
  linter/source-code-fixer.js                  129      debug
  linter/timing.js                             129      —
  options.js                                   375      optionator
  rule-tester/flat-rule-tester.js              967      node:assert, node:util, node:path, fast-deep-equal, shared/traverser.js, config/flat-config-helpers.js, linter/index.js, linter/code-path-analysis/code-path.js …(+5)
  rule-tester/index.js                         4        rule-tester/rule-tester.js
  rule-tester/rule-tester.js                   1048     node:assert, node:path, node:util, lodash.merge, fast-deep-equal, shared/traverser.js, shared/config-validator.js, linter/index.js …(+3)
  rules/accessor-pairs.js                      300      rules/utils/ast-utils.js
  rules/array-bracket-newline.js               231      rules/utils/ast-utils.js
  rules/array-bracket-spacing.js               218      rules/utils/ast-utils.js
  rules/array-callback-return.js               374      rules/utils/ast-utils.js
  rules/array-element-newline.js               265      rules/utils/ast-utils.js
  rules/arrow-body-style.js                    259      rules/utils/ast-utils.js
  rules/arrow-parens.js                        161      rules/utils/ast-utils.js
  rules/arrow-spacing.js                       139      rules/utils/ast-utils.js
  rules/block-scoped-var.js                    114      —
  rules/block-spacing.js                       150      rules/utils/ast-utils.js
  rules/brace-style.js                         170      rules/utils/ast-utils.js
  rules/callback-return.js                     149      —
  rules/camelcase.js                           351      rules/utils/ast-utils.js
  rules/capitalized-comments.js                254      rules/utils/patterns/letters.js, rules/utils/ast-utils.js
  rules/class-methods-use-this.js              164      rules/utils/ast-utils.js
  rules/comma-dangle.js                        330      rules/utils/ast-utils.js
  rules/comma-spacing.js                       160      rules/utils/ast-utils.js
  rules/comma-style.js                         272      rules/utils/ast-utils.js
  rules/complexity.js                          139      rules/utils/ast-utils.js, shared/string-utils.js
  rules/computed-property-spacing.js           183      rules/utils/ast-utils.js
  rules/consistent-return.js                   177      rules/utils/ast-utils.js, shared/string-utils.js
  rules/consistent-this.js                     131      —
  rules/constructor-super.js                   374      —
  rules/curly.js                               418      rules/utils/ast-utils.js
  rules/default-case-last.js                   35       —
  rules/default-case.js                        76       —
  rules/default-param-last.js                  51       —
  rules/dot-location.js                        90       rules/utils/ast-utils.js
  rules/dot-notation.js                        154      rules/utils/ast-utils.js, rules/utils/keywords.js
  rules/eol-last.js                            98       —
  rules/eqeqeq.js                              150      rules/utils/ast-utils.js
  rules/for-direction.js                       119      @eslint-community/eslint-utils
  rules/func-call-spacing.js                   206      rules/utils/ast-utils.js
  rules/func-name-matching.js                  223      rules/utils/ast-utils.js, esutils
  rules/func-names.js                          166      rules/utils/ast-utils.js
  rules/func-style.js                          81       —
  rules/function-call-argument-newline.js      107      —
  rules/function-paren-newline.js              253      rules/utils/ast-utils.js
  rules/generator-star-spacing.js              181      —
  rules/getter-return.js                       172      rules/utils/ast-utils.js
  rules/global-require.js                      73       —
  rules/grouped-accessor-pairs.js              182      rules/utils/ast-utils.js
  rules/guard-for-in.js                        59       —
  rules/handle-callback-err.js                 83       —
  rules/id-blacklist.js                        214      —
  rules/id-denylist.js                         199      —
  rules/id-length.js                           154      shared/string-utils.js
  rules/id-match.js                            247      —
  rules/implicit-arrow-linebreak.js            73       rules/utils/ast-utils.js
  rules/indent-legacy.js                       954      rules/utils/ast-utils.js
  rules/indent.js                              1552     rules/utils/ast-utils.js
  rules/index.js                               302      rules/utils/lazy-loading-rule-map.js, rules/accessor-pairs.js, rules/array-bracket-newline.js, rules/array-bracket-spacing.js, rules/array-callback-return.js, rules/array-element-newline.js, rules/arrow-body-style.js, rules/arrow-parens.js …(+284)
  rules/init-declarations.js                   121      —
  rules/jsx-quotes.js                          84       rules/utils/ast-utils.js
  rules/key-spacing.js                         611      rules/utils/ast-utils.js, shared/string-utils.js
  rules/keyword-spacing.js                     558      rules/utils/ast-utils.js, rules/utils/keywords.js
  rules/line-comment-position.js               105      rules/utils/ast-utils.js
  rules/linebreak-style.js                     91       rules/utils/ast-utils.js
  rules/lines-around-comment.js                412      rules/utils/ast-utils.js
  rules/lines-around-directive.js              173      rules/utils/ast-utils.js
  rules/lines-between-class-members.js         239      rules/utils/ast-utils.js
  rules/logical-assignment-operators.js        425      rules/utils/ast-utils.js
  rules/max-classes-per-file.js                80       —
  rules/max-depth.js                           136      —
  rules/max-len.js                             374      —
  rules/max-lines-per-function.js              183      rules/utils/ast-utils.js, shared/string-utils.js
  rules/max-lines.js                           165      rules/utils/ast-utils.js
  rules/max-nested-callbacks.js                98       —
  rules/max-params.js                          90       rules/utils/ast-utils.js, shared/string-utils.js
  rules/max-statements-per-line.js             177      rules/utils/ast-utils.js
  rules/max-statements.js                      158      rules/utils/ast-utils.js, shared/string-utils.js
  rules/multiline-comment-style.js             406      rules/utils/ast-utils.js
  rules/multiline-ternary.js                   152      rules/utils/ast-utils.js
  rules/new-cap.js                             227      rules/utils/ast-utils.js
  rules/new-parens.js                          79       rules/utils/ast-utils.js
  rules/newline-after-var.js                   212      rules/utils/ast-utils.js
  rules/newline-before-return.js               185      —
  rules/newline-per-chained-call.js            107      rules/utils/ast-utils.js
  rules/no-alert.js                            116      rules/utils/ast-utils.js
  rules/no-array-constructor.js                110      rules/utils/ast-utils.js
  rules/no-async-promise-executor.js           34       —
  rules/no-await-in-loop.js                    91       —
  rules/no-bitwise.js                          104      —
  rules/no-buffer-constructor.js               40       —
  rules/no-caller.js                           34       —
  rules/no-case-declarations.js                54       —
  rules/no-catch-shadow.js                     63       rules/utils/ast-utils.js
  rules/no-class-assign.js                     49       rules/utils/ast-utils.js
  rules/no-compare-neg-zero.js                 51       —
  rules/no-cond-assign.js                      131      rules/utils/ast-utils.js
  rules/no-confusing-arrow.js                  76       rules/utils/ast-utils.js
  rules/no-console.js                          181      rules/utils/ast-utils.js
  rules/no-const-assign.js                     44       rules/utils/ast-utils.js
  rules/no-constant-binary-expression.js       463      globals, rules/utils/ast-utils.js
  rules/no-constant-condition.js               130      rules/utils/ast-utils.js
  rules/no-constructor-return.js               51       —
  rules/no-continue.js                         30       —
  rules/no-control-regex.js                    116      @eslint-community/regexpp
  rules/no-debugger.js                         34       —
  rules/no-delete-var.js                       32       —
  rules/no-div-regex.js                        41       —
  rules/no-dupe-args.js                        65       —
  rules/no-dupe-class-members.js               84       rules/utils/ast-utils.js
  rules/no-dupe-else-if.js                     100      rules/utils/ast-utils.js
  rules/no-dupe-keys.js                        117      rules/utils/ast-utils.js
  rules/no-duplicate-case.js                   58       rules/utils/ast-utils.js
  rules/no-duplicate-imports.js                263      —
  rules/no-else-return.js                      331      rules/utils/ast-utils.js, rules/utils/fix-tracker.js
  rules/no-empty-character-class.js            59       @eslint-community/regexpp
  rules/no-empty-function.js                   145      rules/utils/ast-utils.js
  rules/no-empty-pattern.js                    67       rules/utils/ast-utils.js
  rules/no-empty-static-block.js               39       —
  rules/no-empty.js                            84       rules/utils/ast-utils.js
  rules/no-eq-null.js                          35       —
  rules/no-eval.js                             241      rules/utils/ast-utils.js
  rules/no-ex-assign.js                        42       rules/utils/ast-utils.js
  rules/no-extend-native.js                    155      rules/utils/ast-utils.js, globals
  rules/no-extra-bind.js                       183      rules/utils/ast-utils.js
  rules/no-extra-boolean-cast.js               260      rules/utils/ast-utils.js, @eslint-community/eslint-utils
  rules/no-extra-label.js                      130      rules/utils/ast-utils.js
  rules/no-extra-parens.js                     1124     @eslint-community/eslint-utils, rules/utils/ast-utils.js
  rules/no-extra-semi.js                       126      rules/utils/fix-tracker.js, rules/utils/ast-utils.js
  rules/no-fallthrough.js                      163      shared/directives.js
  rules/no-floating-decimal.js                 61       rules/utils/ast-utils.js
  rules/no-func-assign.js                      65       rules/utils/ast-utils.js
  rules/no-global-assign.js                    82       —
  rules/no-implicit-coercion.js                323      rules/utils/ast-utils.js
  rules/no-implicit-globals.js                 121      —
  rules/no-implied-eval.js                     108      rules/utils/ast-utils.js, @eslint-community/eslint-utils
  rules/no-import-assign.js                    209      @eslint-community/eslint-utils, rules/utils/ast-utils.js
  rules/no-inline-comments.js                  92       rules/utils/ast-utils.js
  rules/no-inner-declarations.js               85       rules/utils/ast-utils.js
  rules/no-invalid-regexp.js                   165      @eslint-community/regexpp
  rules/no-invalid-this.js                     127      rules/utils/ast-utils.js
  rules/no-irregular-whitespace.js             239      rules/utils/ast-utils.js
  rules/no-iterator.js                         39       rules/utils/ast-utils.js
  rules/no-label-var.js                        62       rules/utils/ast-utils.js
  rules/no-labels.js                           129      rules/utils/ast-utils.js
  rules/no-lone-blocks.js                      115      —
  rules/no-lonely-if.js                        74       —
  rules/no-loop-func.js                        172      —
  rules/no-loss-of-precision.js                183      —
  rules/no-magic-numbers.js                    216      rules/utils/ast-utils.js
  rules/no-misleading-character-class.js       254      @eslint-community/eslint-utils, @eslint-community/regexpp, rules/utils/unicode/index.js, rules/utils/ast-utils.js, rules/utils/regular-expressions.js
  rules/no-mixed-operators.js                  205      rules/utils/ast-utils.js
  rules/no-mixed-requires.js                   192      —
  rules/no-mixed-spaces-and-tabs.js            97       —
  rules/no-multi-assign.js                     55       —
  rules/no-multi-spaces.js                     120      rules/utils/ast-utils.js
  rules/no-multi-str.js                        51       rules/utils/ast-utils.js
  rules/no-multiple-empty-lines.js             132      —
  rules/no-native-reassign.js                  83       —
  rules/no-negated-condition.js                83       —
  rules/no-negated-in-lhs.js                   35       —
  rules/no-nested-ternary.js                   36       —
  rules/no-new-func.js                         70       rules/utils/ast-utils.js
  rules/no-new-native-nonconstructor.js        51       —
  rules/no-new-object.js                       52       rules/utils/ast-utils.js
  rules/no-new-require.js                      38       —
  rules/no-new-symbol.js                       44       —
  rules/no-new-wrappers.js                     46       rules/utils/ast-utils.js
  rules/no-new.js                              34       —
  rules/no-nonoctal-decimal-escape.js          127      —
  rules/no-obj-calls.js                        69       @eslint-community/eslint-utils, rules/utils/ast-utils.js
  rules/no-object-constructor.js               95       rules/utils/ast-utils.js
  rules/no-octal-escape.js                     43       —
  rules/no-octal.js                            35       —
  rules/no-param-reassign.js                   198      —
  rules/no-path-concat.js                      47       —
  rules/no-plusplus.js                         85       —
  rules/no-process-env.js                      37       —
  rules/no-process-exit.js                     36       —
  rules/no-promise-executor-return.js          215      @eslint-community/eslint-utils, rules/utils/ast-utils.js
  rules/no-proto.js                            36       rules/utils/ast-utils.js
  rules/no-prototype-builtins.js               132      rules/utils/ast-utils.js
  rules/no-redeclare.js                        148      rules/utils/ast-utils.js
  rules/no-regex-spaces.js                     164      rules/utils/ast-utils.js, @eslint-community/regexpp
  rules/no-restricted-exports.js               168      rules/utils/ast-utils.js
  rules/no-restricted-globals.js               107      —
  rules/no-restricted-imports.js               367      rules/utils/ast-utils.js, ignore
  rules/no-restricted-modules.js               182      rules/utils/ast-utils.js, ignore
  rules/no-restricted-properties.js            148      rules/utils/ast-utils.js
  rules/no-restricted-syntax.js                61       —
  rules/no-return-assign.js                    66       rules/utils/ast-utils.js
  rules/no-return-await.js                     111      rules/utils/ast-utils.js
  rules/no-script-url.js                       51       rules/utils/ast-utils.js
  rules/no-self-assign.js                      159      rules/utils/ast-utils.js
  rules/no-self-compare.js                     47       —
  rules/no-sequences.js                        115      rules/utils/ast-utils.js
  rules/no-setter-return.js                    187      rules/utils/ast-utils.js, @eslint-community/eslint-utils
  rules/no-shadow-restricted-names.js          54       —
  rules/no-shadow.js                           285      rules/utils/ast-utils.js
  rules/no-spaced-func.js                      68       —
  rules/no-sparse-arrays.js                    36       —
  rules/no-sync.js                             53       —
  rules/no-tabs.js                             70       —
  rules/no-template-curly-in-string.js         36       —
  rules/no-ternary.js                          30       —
  rules/no-this-before-super.js                286      rules/utils/ast-utils.js
  rules/no-throw-literal.js                    38       rules/utils/ast-utils.js
  rules/no-trailing-spaces.js                  159      rules/utils/ast-utils.js
  rules/no-undef-init.js                       58       rules/utils/ast-utils.js
  rules/no-undef.js                            67       —
  rules/no-undefined.js                        67       —
  rules/no-underscore-dangle.js                300      —
  rules/no-unexpected-multiline.js             99       rules/utils/ast-utils.js
  rules/no-unmodified-loop-condition.js        302      shared/traverser.js, rules/utils/ast-utils.js
  rules/no-unneeded-ternary.js                 142      rules/utils/ast-utils.js
  rules/no-unreachable-loop.js                 150      —
  rules/no-unreachable.js                      246      —
  rules/no-unsafe-finally.js                   96       —
  rules/no-unsafe-negation.js                  110      rules/utils/ast-utils.js
  rules/no-unsafe-optional-chaining.js         193      —
  rules/no-unused-expressions.js               169      rules/utils/ast-utils.js
  rules/no-unused-labels.js                    120      rules/utils/ast-utils.js
  rules/no-unused-private-class-members.js     163      —
  rules/no-unused-vars.js                      602      rules/utils/ast-utils.js
  rules/no-use-before-define.js                301      —
  rules/no-useless-backreference.js            158      @eslint-community/eslint-utils, @eslint-community/regexpp
  rules/no-useless-call.js                     74       rules/utils/ast-utils.js
  rules/no-useless-catch.js                    50       —
  rules/no-useless-computed-key.js             139      rules/utils/ast-utils.js
  rules/no-useless-concat.js                   95       rules/utils/ast-utils.js
  rules/no-useless-constructor.js              167      —
  rules/no-useless-escape.js                   279      rules/utils/ast-utils.js, @eslint-community/regexpp
  rules/no-useless-rename.js                   144      rules/utils/ast-utils.js
  rules/no-useless-return.js                   313      rules/utils/ast-utils.js, rules/utils/fix-tracker.js
  rules/no-var.js                              292      rules/utils/ast-utils.js
  rules/no-void.js                             56       —
  rules/no-warning-comments.js                 173      escape-string-regexp, rules/utils/ast-utils.js
  rules/no-whitespace-before-property.js       96       rules/utils/ast-utils.js
  rules/no-with.js                             30       —
  rules/nonblock-statement-body-position.js    110      —
  rules/object-curly-newline.js                284      rules/utils/ast-utils.js
  rules/object-curly-spacing.js                268      rules/utils/ast-utils.js
  rules/object-property-newline.js             86       —
  rules/object-shorthand.js                    450      rules/utils/ast-utils.js
  rules/one-var-declaration-per-line.js        78       —
  rules/one-var.js                             501      rules/utils/ast-utils.js
  rules/operator-assignment.js                 180      rules/utils/ast-utils.js
  rules/operator-linebreak.js                  212      rules/utils/ast-utils.js
  rules/padded-blocks.js                       270      rules/utils/ast-utils.js
  rules/padding-line-between-statements.js     529      rules/utils/ast-utils.js
  rules/prefer-arrow-callback.js               317      rules/utils/ast-utils.js
  rules/prefer-const.js                        416      rules/utils/fix-tracker.js, rules/utils/ast-utils.js
  rules/prefer-destructuring.js                265      rules/utils/ast-utils.js
  rules/prefer-exponentiation-operator.js      161      rules/utils/ast-utils.js, @eslint-community/eslint-utils
  rules/prefer-named-capture-group.js          154      @eslint-community/eslint-utils, @eslint-community/regexpp
  rules/prefer-numeric-literals.js             122      rules/utils/ast-utils.js
  rules/prefer-object-has-own.js               94       rules/utils/ast-utils.js
  rules/prefer-object-spread.js                260      @eslint-community/eslint-utils, rules/utils/ast-utils.js
  rules/prefer-promise-reject-errors.js        111      rules/utils/ast-utils.js
  rules/prefer-reflect.js                      112      —
  rules/prefer-regex-literals.js               436      rules/utils/ast-utils.js, @eslint-community/eslint-utils, @eslint-community/regexpp, rules/utils/regular-expressions.js
  rules/prefer-rest-params.js                  98       —
  rules/prefer-spread.js                       73       rules/utils/ast-utils.js
  rules/prefer-template.js                     232      rules/utils/ast-utils.js
  rules/quote-props.js                         272      espree, rules/utils/ast-utils.js, rules/utils/keywords.js
  rules/quotes.js                              298      rules/utils/ast-utils.js
  rules/radix.js                               171      rules/utils/ast-utils.js
  rules/require-atomic-updates.js              278      —
  rules/require-await.js                       95       rules/utils/ast-utils.js
  rules/require-jsdoc.js                       112      —
  rules/require-unicode-regexp.js              108      @eslint-community/eslint-utils, rules/utils/ast-utils.js, rules/utils/regular-expressions.js
  rules/require-yield.js                       63       —
  rules/rest-spread-spacing.js                 109      —
  rules/semi-spacing.js                        215      rules/utils/ast-utils.js
  rules/semi-style.js                          134      rules/utils/ast-utils.js
  rules/semi.js                                383      rules/utils/fix-tracker.js, rules/utils/ast-utils.js
  rules/sort-imports.js                        211      —
  rules/sort-keys.js                           197      rules/utils/ast-utils.js, natural-compare
  rules/sort-vars.js                           85       —
  rules/space-before-blocks.js                 180      rules/utils/ast-utils.js
  rules/space-before-function-paren.js         145      rules/utils/ast-utils.js
  rules/space-in-parens.js                     245      rules/utils/ast-utils.js
  rules/space-infix-ops.js                     167      rules/utils/ast-utils.js
  rules/space-unary-ops.js                     294      rules/utils/ast-utils.js
  rules/spaced-comment.js                      333      escape-string-regexp, rules/utils/ast-utils.js
  rules/strict.js                              240      rules/utils/ast-utils.js
  rules/switch-colon-spacing.js                119      rules/utils/ast-utils.js
  rules/symbol-description.js                  59       rules/utils/ast-utils.js
  rules/template-curly-spacing.js              123      rules/utils/ast-utils.js
  rules/template-tag-spacing.js                81       —
  rules/unicode-bom.js                         60       —
  rules/use-isnan.js                           119      rules/utils/ast-utils.js
  rules/utils/ast-utils.js                     2014     eslint-visitor-keys, esutils, espree, escape-string-regexp, shared/ast-utils.js
  rules/utils/fix-tracker.js                   99       rules/utils/ast-utils.js
  rules/utils/keywords.js                      66       —
  rules/utils/lazy-loading-rule-map.js         101      debug
  rules/utils/patterns/letters.js              33       —
  rules/utils/regular-expressions.js           34       @eslint-community/regexpp
  rules/utils/unicode/index.js                 10       rules/utils/unicode/is-combining-character.js, rules/utils/unicode/is-emoji-modifier.js, rules/utils/unicode/is-regional-indicator-symbol.js, rules/utils/unicode/is-surrogate-pair.js
  rules/utils/unicode/is-combining-character.js 12       —
  rules/utils/unicode/is-emoji-modifier.js     12       —
  rules/utils/unicode/is-regional-indicator-symbol.js 12       —
  rules/utils/unicode/is-surrogate-pair.js     13       —
  rules/valid-jsdoc.js                         456      doctrine
  rules/valid-typeof.js                        105      rules/utils/ast-utils.js
  rules/vars-on-top.js                         136      —
  rules/wrap-iife.js                           171      rules/utils/ast-utils.js, @eslint-community/eslint-utils
  rules/wrap-regex.js                          49       —
  rules/yield-star-spacing.js                  114      —
  rules/yoda.js                                310      rules/utils/ast-utils.js
  shared/ajv.js                                28       ajv
  shared/ast-utils.js                          26       —
  shared/config-validator.js                   298      node:util, /tmp/eslint-inspect/conf/config-schema.js, rules/index.js, @eslint/eslintrc, shared/deprecation-warnings.js, shared/ajv.js
  shared/deprecation-warnings.js               46       node:path
  shared/directives.js                         13       —
  shared/logging.js                            25       —
  shared/relative-module-resolver.js           44       node:module
  shared/runtime-info.js                       139      node:path, cross-spawn, node:os, shared/logging.js, /tmp/eslint-inspect/package.json
  shared/severity.js                           45       —
  shared/string-utils.js                       48       graphemer
  shared/traverser.js                          169      eslint-visitor-keys, debug
  shared/types.js                              193      —
  source-code/index.js                         4        source-code/source-code.js
  source-code/source-code.js                   884      @eslint-community/eslint-utils, source-code/token-store/index.js, shared/ast-utils.js, shared/traverser.js, /tmp/eslint-inspect/conf/globals.js, shared/directives.js, linter/config-comment-parser.js, eslint-scope
  source-code/token-store/backward-token-comment-cursor.js 49       source-code/token-store/cursor.js, source-code/token-store/utils.js
  source-code/token-store/backward-token-cursor.js 50       source-code/token-store/cursor.js, source-code/token-store/utils.js
  source-code/token-store/cursor.js            68       —
  source-code/token-store/cursors.js           78       source-code/token-store/backward-token-comment-cursor.js, source-code/token-store/backward-token-cursor.js, source-code/token-store/filter-cursor.js, source-code/token-store/forward-token-comment-cursor.js, source-code/token-store/forward-token-cursor.js, source-code/token-store/limit-cursor.js, source-code/token-store/skip-cursor.js
  source-code/token-store/decorative-cursor.js 31       source-code/token-store/cursor.js
  source-code/token-store/filter-cursor.js     36       source-code/token-store/decorative-cursor.js
  source-code/token-store/forward-token-comment-cursor.js 49       source-code/token-store/cursor.js, source-code/token-store/utils.js
  source-code/token-store/forward-token-cursor.js 54       source-code/token-store/cursor.js, source-code/token-store/utils.js
  source-code/token-store/index.js             578      node:assert, @eslint-community/eslint-utils, source-code/token-store/cursors.js, source-code/token-store/forward-token-cursor.js, source-code/token-store/padded-token-cursor.js, source-code/token-store/utils.js
  source-code/token-store/limit-cursor.js      34       source-code/token-store/decorative-cursor.js
  source-code/token-store/padded-token-cursor.js 33       source-code/token-store/forward-token-cursor.js
  source-code/token-store/skip-cursor.js       36       source-code/token-store/decorative-cursor.js
  source-code/token-store/utils.js             97       —
  unsupported-api.js                           25       cli-engine/file-enumerator.js, eslint/flat-eslint.js, rule-tester/flat-rule-tester.js, eslint/eslint.js, rules/index.js
  /tmp/eslint-inspect/conf/default-cli-options.js 0        —
  /tmp/eslint-inspect/package.json             0        —
  /tmp/eslint-inspect/conf/replacements.json   0        —
  /tmp/eslint-inspect/conf/config-schema.js    0        —
  /tmp/eslint-inspect/conf/globals.js          0        —
```
