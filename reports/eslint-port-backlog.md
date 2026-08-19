# ESLint port backlog (from ts2go scaffold + registry)

Leaf packages NOT covered by an existing Go module — genuine ports. Reused deps
(chalk, ajv, text-table, js-yaml...) are excluded by the npm→Go counterpart registry.

```
would scaffold 21 repo(s) under :

  json-schema-traverse                           github.com/jclyons52/json-schema-traverse-go 331 LOC  api=(see original/)
  uri-js                                         github.com/jclyons52/uri-js-go     2390 LOC  api=SCHEMES,pctEncChar,pctDecChars,parse,…
  debug                                          github.com/jclyons52/debug-go      735 LOC  api=(see original/)
  espree                                         github.com/jclyons52/espree-go     783 LOC  api=(see original/)
  acorn                                          github.com/jclyons52/acorn-go      6436 LOC  api=Node,Parser,Position,SourceLocation,…
  type-fest                                      github.com/jclyons52/type-fest-go  2311 LOC  api=(see original/)
  ignore                                         github.com/jclyons52/ignore-go     1102 LOC  api=(see original/)
  argparse                                       github.com/jclyons52/argparse-go   3691 LOC  api=ArgumentParser,ArgumentError,ArgumentTypeError,BooleanOptionalAction,…
  color-name                                     github.com/jclyons52/color-name-go 151 LOC  api=aliceblue,248,255],antiquewhite,…
  flatted                                        github.com/jclyons52/flatted-go    374 LOC  api=parse,stringify,toJSON,fromJSON
  @humanwhocodes/object-schema                   github.com/jclyons52/humanwhocodes-object-schema-go 395 LOC  api=ObjectSchema,MergeStrategy,ValidationStrategy
  @ungap/structured-clone                        github.com/jclyons52/ungap-structured-clone-go 609 LOC  api=deserialize,serialize
  @nodelib/fs.stat                               github.com/jclyons52/nodelib-fs-stat-go 172 LOC  api=statSync,stat,Settings
  prelude-ls                                     github.com/jclyons52/prelude-ls-go 1339 LOC  api=(see original/)
  eslint-scope                                   github.com/jclyons52/eslint-scope-go 1931 LOC  api=(see original/)
  estraverse                                     github.com/jclyons52/estraverse-go 756 LOC  api=Syntax,traverse,replace,attachComments,…
  lodash.merge                                   github.com/jclyons52/lodash-merge-go 1819 LOC  api=nodeType
  esquery                                        github.com/jclyons52/esquery-go    15022 LOC  api=(see original/)
  @eslint-community/regexpp                      github.com/jclyons52/eslint-community-regexpp-go 4184 LOC  api=AST,RegExpParser,RegExpSyntaxError,RegExpValidator,…
  isexe                                          github.com/jclyons52/isexe-go      318 LOC  api=(see original/)
  esutils                                        github.com/jclyons52/esutils-go    409 LOC  api=ast,code,keyword
```
