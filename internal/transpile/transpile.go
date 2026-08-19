// Package transpile converts TypeScript source into Go, using the
// ts-go-morph AST (syntax-first: source-level names like `uint8` are
// preserved) and the TypeScript type checker (semantics: unions,
// nullability, conditional/mapped types are resolved).
package transpile

import (
	"fmt"
	"strings"

	"github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// transpiler holds the per-file conversion state.
type transpiler struct {
	p   *tsmorph.Project
	sf  *tsmorph.SourceFile
	out *goWriter
	// aliases maps type-alias names to their definitions, collected in a
	// pre-pass so types can be emitted in any order.
	aliases map[string]aliasInfo
	// nextID allocates unique names for compiler-generated identifiers.
	nextID int
	// items collects LLM work items (gaps) so the output never aborts on
	// unsupported constructs; each gets a placeholder + report entry.
	items []WorkItem
	// audit collects type-tightening sites: places emitted as any/dynamic
	// where the checker resolved a concrete type. See TightenSite.
	audit []TightenSite
	// fatal collects internal errors that prevented (parts of) emission.
	fatal []string
	// retStack tracks the enclosing function's Go return type so return
	// expressions can be converted (e.g. string-union alias → string).
	retStack []string
	// inDeferred is > 0 while emitting a recover/finally handler, where
	// `return expr` is not valid Go.
	inDeferred int
	// initBuf collects top-level expression statements, which are not valid
	// at Go package scope; they are wrapped in a synthesized func init().
	initBuf *goWriter
	// declared holds names declared in this file; calls to identifiers
	// outside it (module imports) are flagged as LLM work.
	declared map[string]bool
	// external holds identifiers bound by imports from sibling packages or
	// external modules. They are unresolved (the Go import is a work item),
	// so dynamic-runtime routing must NOT pull them in — they stay
	// resilience placeholders.
	external map[string]bool
	// usedShim is set when the emitted code references the jsrt async
	// shim; the shim source is appended and its imports declared.
	usedShim bool
	// pkgDeclared, when set, is the union of top-level names across every
	// file of the multi-file package this file belongs to; calls to names
	// declared in sibling files of the package are then not flagged as
	// unknown module imports.
	pkgDeclared map[string]bool
	// classify, when set, classifies a module specifier imported from this
	// file: same-package imports are dropped, sibling-package and external
	// imports become work items.
	classify func(fromFile, spec string) (importClass, string)
	// emitShim controls whether the jsrt shim source is appended to this
	// file's output. A multi-file package emits it once, in its own file.
	emitShim bool
	// pkgName is the Go package clause written in the preamble.
	pkgName string
	// used tracks which stdlib packages the emitted code references, so the
	// import block only declares what is actually used.
	used map[string]bool
}

type aliasInfo struct {
	name   string // alias name as written (e.g. "uint8")
	target string // resolved Go type (e.g. "uint8")
	// unionValues is non-nil (possibly empty) when the alias is a union of
	// string literals, emitted as a named string type + consts.
	unionValues []string
}

// TightenSite is one place the transpiler emitted `any` (or a dynamic gap)
// where the TypeScript checker resolved a concrete type. These are the
// "tighten me" worklist: annotating the source (or the JSDoc, for JS input)
// at this site gives uplift the type it needs. The same checker-typing is the
// basis of the JS→TS lifting path.
type TightenSite struct {
	Category    string // dynamic, param, return, var
	Line        int
	CheckerType string // what the checker resolved (e.g. "SymbolFlags")
	Emitted     string // what uplift emitted (usually "any")
	Snippet     string
}

// NewTranspiler returns a transpiler bound to a source file.
func NewTranspiler(p *tsmorph.Project, sf *tsmorph.SourceFile) *Transpiler {
	return &Transpiler{tr: &transpiler{p: p, sf: sf, out: newGoWriter(), aliases: map[string]aliasInfo{}, used: map[string]bool{}, emitShim: true, pkgName: "main"}}
}

// Transpiler is the public entry point.
type Transpiler struct {
	tr     *transpiler
	report *Report
}

// SetPackageContext shares cross-file knowledge with a multi-file package
// driver: pkgDeclared is the union of top-level names across the files of
// this file's Go package, and classify reports how a module specifier
// imported from fromFile should be treated (same package, sibling package,
// or external).
func (t *Transpiler) SetPackageContext(pkgDeclared map[string]bool, classify func(fromFile, spec string) (importClass, string)) {
	t.tr.pkgDeclared = pkgDeclared
	t.tr.classify = classify
}

// SetEmitShim controls whether the jsrt async shim source is appended to
// this file's output. Multi-file packages emit the shim once, in its own
// file, so every file that references it sets this to false.
func (t *Transpiler) SetEmitShim(v bool) { t.tr.emitShim = v }

// SetPackageName sets the Go package clause written in the preamble
// (default "main").
func (t *Transpiler) SetPackageName(name string) { t.tr.pkgName = name }

// UsedShim reports whether the emitted code references the jsrt async shim.
// Valid after Transpile.
func (t *Transpiler) UsedShim() bool { return t.tr.usedShim }

// tightenable records a type-tightening site: the checker resolved a
// concrete type for n (or its node), but the emitter degraded (emitted `any`
// or a dynamic placeholder). Recording it gives the type-audit report the
// "annotate here to tighten" worklist.
func (tr *transpiler) tightenable(category string, n tsmorph.Node, checkerType, emitted string) {
	if checkerType == "" || checkerType == "any" || checkerType == "unknown" {
		return
	}
	tr.audit = append(tr.audit, TightenSite{
		Category:    category,
		Line:        tr.lineOf(n),
		CheckerType: checkerType,
		Emitted:     emitted,
		Snippet:     oneLine(snippetLong(n)),
	})
}

// notAnyLike reports whether a checker-rendered type text is a real concrete
// type rather than any/unknown/never-ish — used to decide if a site is a
// genuine tightening opportunity.
func notAnyLike(t string) bool {
	switch strings.TrimSpace(t) {
	case "any", "unknown", "never", "{}", "undefined", "null", "":
		return false
	}
	return !strings.HasPrefix(strings.TrimSpace(t), "any")
}

// recoverToError converts a panic into an error, for use in a deferred
// call: `defer recoverToError(&err)`. The resilience contract is panic-proof
// because every statement emitter uses it — a ts-go-morph accessor gap
// degrades to a TODO placeholder instead of aborting the file.
func recoverToError(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("panic: %v", r)
	}
}

// transpileStatementSafe runs transpileStatement with panic recovery: a
// panic (e.g. a ts-go-morph accessor gap) becomes an error the caller turns
// into a TODO placeholder, instead of aborting the whole file.
func (tr *transpiler) transpileStatementSafe(stmt tsmorph.Node) (err error) {
	defer recoverToError(&err)
	return tr.transpileStatement(stmt, false)
}

// Transpile converts the source file to Go source text. It never aborts on
// unsupported constructs: each gap is replaced by a compiling placeholder,
// recorded in the embedded post-work manifest, and listed in Report().
func (t *Transpiler) Transpile() (string, error) {
	tr := t.tr
	tr.scanBans()
	tr.collectDeclared()
	if err := tr.collectAliases(); err != nil {
		return "", err
	}
	// Emit the body first so the preamble can declare only the imports that
	// the emitted code actually references.
	body := newGoWriter()
	saved := tr.out
	tr.out = body
	tr.initBuf = newGoWriter()
	tr.initBuf.indent()
	for _, stmt := range tr.sf.Statements() {
		if ast.IsExpressionStatement(stmt.ASTNode()) {
			// A CommonJS export statement (`exports.x = v`, `module.exports =
			// {...}`) emits Go declarations, which Go forbids inside func
			// init(); route it straight to package scope.
			if e, ok := stmt.GetExpression(); ok {
				hand, herr := tr.handleCommonJSExport(e)
				if hand {
					if herr != nil {
						tr.fatal = append(tr.fatal, fmt.Sprintf("line %d: %v", tr.lineOf(stmt), herr))
						tr.emitTodoStatement(stmt, "transpilation error: %v", herr)
					}
					continue
				}
			}
			// Top-level expression statements are invalid at Go package
			// scope; route them into a synthesized func init().
			exprOut := tr.out
			tr.out = tr.initBuf
			err := tr.transpileStatementSafe(stmt)
			tr.out = exprOut
			if err != nil {
				tr.fatal = append(tr.fatal, fmt.Sprintf("line %d: %v", tr.lineOf(stmt), err))
				tr.emitTodoStatement(stmt, "transpilation error: %v", err)
			}
			continue
		}
		if err := tr.transpileStatementSafe(stmt); err != nil {
			// Resilience: a failed statement becomes a TODO placeholder and
			// the rest of the file still transpiles.
			tr.fatal = append(tr.fatal, fmt.Sprintf("line %d: %v", tr.lineOf(stmt), err))
			tr.emitTodoStatement(stmt, "transpilation error: %v", err)
		}
	}
	if s := tr.initBuf.String(); s != "" {
		body.line("func init() {")
		body.raw(s)
		body.line("}")
		body.blank()
	}
	tr.out = saved
	// Now emit the preamble (imports only what was used) and glue it on top.
	// Import pruning: a mapped call may mark a package used but later
	// collapse to a placeholder (e.g. a regex-literal argument), leaving the
	// import unreferenced. Drop any import the body never actually mentions.
	bodyText := body.String()
	// The jsrt shim (appended later) references fmt/sync/time itself; keep
	// those if the shim will land in this file.
	if tr.usedShim && tr.emitShim {
		bodyText += jsrtShim
	}
	pruned := pruneUnusedImports(bodyText, tr.used)
	for pkg := range tr.used {
		if !pruned[pkg] {
			delete(tr.used, pkg)
		}
	}
	pre := newGoWriter()
	tr.out = pre
	if err := tr.emitPreamble(); err != nil {
		return "", err
	}
	tr.out = saved
	t.report = tr.buildReport(true)
	src := pre.String() + body.String()
	if tr.usedShim && tr.emitShim {
		src += jsrtShim
	}
	return src, nil
}

// Assess runs the full transpilation purely to collect the post-work report
// (dry-run mode); the generated source is discarded. Use after Transpile or
// Assess to read the report.
func (t *Transpiler) Assess() *Report {
	_, _ = t.Transpile()
	return t.Report()
}

// Report returns the post-work report from the last Transpile/Assess call.
func (t *Transpiler) Report() *Report {
	if t.report == nil {
		t.report = t.tr.buildReport(true)
	}
	return t.report
}

func (tr *transpiler) buildReport(ok bool) *Report {
	r := &Report{
		Input:       tr.sf.FilePath(),
		TSLines:     lineAt(tr.sf.Text(), len(tr.sf.Text())),
		DeclCount:   len(tr.sf.Statements()),
		Items:       append([]WorkItem(nil), tr.items...),
		EmittedOK:   ok && len(tr.fatal) == 0,
		FatalErrors: oneLineAll(tr.fatal),
		Audit:       append([]TightenSite(nil), tr.audit...),
	}
	return r
}

// oneLineAll collapses every string in a slice to one line (see oneLine).
func oneLineAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = oneLine(s)
	}
	return out
}

// ---------------------------------------------------------------------------
// Type mapping
// ---------------------------------------------------------------------------

// goType converts a TS type node to a Go type string.
// Syntax-first: TypeReference names are looked up in the alias table so
// `type uint8 = number` maps to Go's uint8, not float64. Where the syntax is
// insufficient (mapped/conditional types) the checker's resolved type text is
// used as a fallback.
func (tr *transpiler) goType(n tsmorph.Node) (string, error) {
	if n.IsZero() {
		return "any", nil
	}
	switch {
	case ast.IsTypeReferenceNode(n.ASTNode()):
		return tr.goTypeReference(n)
	case ast.IsUnionTypeNode(n.ASTNode()):
		return tr.goUnionType(n)
	case ast.IsArrayTypeNode(n.ASTNode()):
		elem, err := tr.goTypeArrayElement(n)
		if err != nil {
			return "", err
		}
		return "[]" + elem, nil
	case ast.IsLiteralTypeNode(n.ASTNode()):
		return tr.goLiteralType(n)
	case ast.IsParenthesizedTypeNode(n.ASTNode()):
		return tr.goType(firstChild(n))
	case ast.IsFunctionTypeNode(n.ASTNode()):
		return tr.goFunctionType(n)
	case ast.IsConstructorTypeNode(n.ASTNode()):
		return tr.goConstructorType(n)
	case ast.IsKeywordTypeNode(n.ASTNode()):
		return tr.goKeywordType(n)
	case ast.IsImportTypeNode(n.ASTNode()):
		// typeof import("mod") / import("mod").Type — resolved through the
		// checker when the module is real; unresolvable modules degrade to
		// any with a gap instead of failing the enclosing statement.
		if typ := n.Type(); !typ.IsUnknown() {
			if s := typ.Text(); s != "" && s != "any" {
				return tr.cleanCheckerType(s), nil
			}
		}
		tr.recordGap(SevTodo, "import", n, "import type %q — resolve or stub the module's types", oneLine(snippetLong(n)))
		return "any", nil
	default:
		// Fall back to the checker: conditional/mapped/template-literal types
		// are already resolved by the TypeScript checker.
		if typ := n.Type(); !typ.IsUnknown() {
			if s := typ.Text(); s != "" && s != "any" {
				return tr.cleanCheckerType(s), nil
			}
		}
		return "", fmt.Errorf("unsupported type node %s at %s", n.KindName(), n.Text())
	}
}

// goTypeReference handles `Foo`, `Foo<T>`, `ns.Foo`.
func (tr *transpiler) goTypeReference(n tsmorph.Node) (string, error) {
	name := strings.TrimSpace(n.Text())
	args := n.GetTypeArguments()
	base := name
	if i := strings.Index(name, "<"); i >= 0 {
		base = name[:i]
	}
	if len(args) > 0 {
		// Convert each type argument recursively: `Record<string, Vec3[]>`
		// must become `map[string][]Vec3`, not raw TS source text.
		conv := make([]string, 0, len(args))
		for _, a := range args {
			t, err := tr.goType(a)
			if err != nil {
				return "", err
			}
			conv = append(conv, t)
		}
		// Generic alias: preserve the alias name, keep Go-style args.
		if info, ok := tr.aliases[base]; ok {
			return tr.applyTypeArgs(info, strings.Join(conv, ", ")), nil
		}
		switch base {
		case "Array":
			return "[]" + conv[0], nil
		case "Record":
			if len(conv) == 2 {
				return "map[" + conv[0] + "]" + conv[1], nil
			}
		case "Map":
			// TS's Map<K, V> interface → Go's builtin map[K]V.
			if len(conv) == 2 {
				return "map[" + conv[0] + "]" + conv[1], nil
			}
		case "Promise":
			// Promise<T> maps to the jsrt shim's promise type (the shim is
			// appended to the output when referenced).
			tr.usedShim = true
			return "*jsrtPromise", nil
		default:
			return base + "[" + strings.Join(conv, ", ") + "]", nil
		}
		return "any", nil
	}
	// Plain reference: alias (uint8 → uint8) or named type (Vec3 → Vec3).
	if info, ok := tr.aliases[base]; ok {
		if info.unionValues != nil {
			// String-literal union alias: references resolve to the named
			// string type emitted by emitTypeAlias.
			return info.name, nil
		}
		return info.target, nil
	}
	// JS root types with no direct Go equivalent: `Object` (and lowercase
	// `object`) map to the dynamic `any` value — a JS object is traversed
	// through the jsrt runtime, and `any` lets callers pass values from
	// dynamic iteration without type assertions. Function and RegExp have
	// runtime-type equivalents.
	switch base {
	case "Object", "object":
		return "any", nil
	case "Function", "CallableFunction":
		return "any", nil
	case "RegExp":
		tr.used["regexp"] = true
		return "*regexp.Regexp", nil
	}
	return base, nil
}

// pruneUnusedImports keeps only stdlib packages the body text actually
// references (as `pkg.`), so an import whose usage collapsed into a
// placeholder is dropped instead of failing the build.
func pruneUnusedImports(body string, used map[string]bool) map[string]bool {
	keep := map[string]bool{}
	for pkg := range used {
		if strings.Contains(body, pkg+".") {
			keep[pkg] = true
		}
	}
	return keep
}

// goUnionType handles `A | B | null | undefined`. Nullable members become
// pointers (Go idiom); string-literal unions become a named type + consts
// (emitted by the caller); object unions use a sealed-interface marker.
func (tr *transpiler) goUnionType(n tsmorph.Node) (string, error) {
	typ := n.Type()
	if typ.IsUnknown() {
		return "any", nil
	}
	// Try the checker decomposition first: it handles aliased unions.
	var nonNull []string
	for _, u := range typ.UnionTypes() {
		if u.IsNull() || u.IsUndefined() {
			continue
		}
		nonNull = append(nonNull, u.Text())
	}
	if len(nonNull) == 0 {
		return "any", nil
	}
	base := tr.cleanCheckerType(nonNull[0])
	if len(nonNull) > 1 {
		// A union of string literals (`"a" | "b"`) collapses to `string`.
		allStr := true
		for _, m := range nonNull {
			if !strings.HasPrefix(m, `"`) || !strings.HasSuffix(m, `"`) {
				allStr = false
				break
			}
		}
		if allStr {
			return "string", nil
		}
		return "any /* union: " + strings.Join(nonNull, " | ") + " */", nil
	}
	if typ.IsNullable() {
		return "*" + base, nil
	}
	return base, nil
}

// goTypeArrayElement extracts the element type of `T[]`.
func (tr *transpiler) goTypeArrayElement(n tsmorph.Node) (string, error) {
	for _, c := range n.Children() {
		k := c.Kind()
		if k == ast.KindTypeReference || k == ast.KindUnionType ||
			k == ast.KindArrayType || k == ast.KindLiteralType ||
			k == ast.KindParenthesizedType || k == ast.KindFunctionType {
			return tr.goType(c)
		}
	}
	// Fall back to checker text: `string[]` → strip trailing [].
	s := n.Type().Text()
	if strings.HasSuffix(s, "[]") {
		return tr.cleanCheckerType(strings.TrimSuffix(s, "[]")), nil
	}
	return tr.cleanCheckerType(s), nil
}

// goLiteralType handles literal types like `"active"`, `5`, `true`.
func (tr *transpiler) goLiteralType(n tsmorph.Node) (string, error) {
	typ := n.Type()
	switch {
	case typ.IsStringLiteral():
		return "string", nil
	case typ.IsNumberLiteral():
		return "float64", nil
	case typ.IsBooleanLiteral():
		return "bool", nil
	}
	return tr.cleanCheckerType(typ.Text()), nil
}

// goFunctionType handles `(a: T) => R` — emitted as a func literal type.
func (tr *transpiler) goFunctionType(n tsmorph.Node) (string, error) {
	sig := n.Type().CallSignatures()
	if len(sig) == 0 {
		return "func()", nil
	}
	var params []string
	for _, p := range sig[0].Parameters() {
		params = append(params, p.Name+" "+tr.cleanCheckerType(p.Type.Text()))
	}
	ret := tr.cleanCheckerType(sig[0].ReturnType().Text())
	return "func(" + strings.Join(params, ", ") + ") " + ret, nil
}

// goConstructorType handles `new (args) => T`: Go has no constructor-typed
// values, so this maps to a plain func type (approximately — construction
// semantics are lost) and is flagged for LLM verification.
func (tr *transpiler) goConstructorType(n tsmorph.Node) (string, error) {
	params, ret, err := tr.signatureFromChildren(n)
	if err != nil {
		return "", err
	}
	if ret == "" {
		ret = "any"
	}
	tr.recordGap(SevApprox, "type", n, "constructor type mapped to func type (construction semantics not preserved)")
	return "func(" + params + ") " + ret, nil
}

// goKeywordType handles `string`, `number`, `boolean`, `any`, etc.
func (tr *transpiler) goKeywordType(n tsmorph.Node) (string, error) {
	name := n.Text()
	switch name {
	case "string":
		return "string", nil
	case "number":
		return "float64", nil // bare number: JS double; use aliases for ints
	case "boolean":
		return "bool", nil
	case "any", "unknown":
		return "any", nil
	case "void":
		return "", nil
	case "null", "undefined", "never":
		return "any", nil
	case "bigint":
		return "int64", nil
	default:
		return "any", nil
	}
}

// cleanCheckerType converts checker type text (e.g. "string[]", "{ x: number }")
// into a usable Go type string. Used as a fallback where the syntax walk is
// insufficient.
func (tr *transpiler) cleanCheckerType(s string) string {
	s = strings.TrimSpace(s)
	// Object literal types: `{ x: number; y: number }` → anonymous struct.
	if strings.HasPrefix(s, "{") {
		return tr.objectTypeToStruct(s)
	}
	// String-literal types (inferred return types render as `"a" | "b"`)
	// collapse to `string`.
	if strings.Contains(s, `"`) {
		return "string"
	}
	// `T | null` handled by caller; strip trailing ` | undefined`.
	s = strings.TrimSuffix(s, " | undefined")
	s = strings.TrimSuffix(s, " | null")
	s = strings.TrimSpace(s)
	if s == "" || s == "any" {
		return "any"
	}
	s = mapCheckerType(s)
	// Generic type text arrives TS-style (`Map<K, V>`, `NodeArray<Node>`);
	// Go needs square brackets. Function types (`(x) => y`) never contain
	// type arguments, so the arrow is a safe discriminator.
	if !strings.Contains(s, "=>") {
		s = strings.ReplaceAll(s, "<", "[")
		s = strings.ReplaceAll(s, ">", "]")
	}
	// Postfix array syntax: `Foo[]` / `Foo[][]` → `[]Foo` / `[][]Foo`.
	for strings.HasSuffix(s, "[]") {
		s = "[]" + strings.TrimSuffix(s, "[]")
	}
	// TS's Map<K, V> interface maps to Go's builtin map[K]V.
	if strings.HasPrefix(s, "Map[") {
		s = "map[" + strings.TrimPrefix(s, "Map[")
		if i := strings.Index(s, ", "); i > 0 {
			s = s[:i] + "]" + s[i+2:]
		}
	}
	return s
}

// mapCheckerType maps TS primitive type names (as rendered by the checker)
// to Go types.
func mapCheckerType(s string) string {
	switch s {
	case "number":
		return "float64"
	case "string":
		return "string"
	case "boolean":
		return "bool"
	case "any", "unknown":
		return "any"
	case "void", "undefined", "null", "never":
		return "any"
	case "bigint":
		return "int64"
	}
	// Array shorthand from the checker: `string[]`, `number[]`.
	if strings.HasSuffix(s, "[]") {
		return "[]" + mapCheckerType(strings.TrimSuffix(s, "[]"))
	}
	return s
}

// objectTypeToStruct converts a checker object-type text to a Go anonymous
// struct. Best-effort: only simple `name: Type;` fields are supported.
func (tr *transpiler) objectTypeToStruct(s string) string {
	inner := strings.TrimPrefix(s, "{")
	inner = strings.TrimSuffix(inner, "}")
	var fields []string
	for _, part := range strings.Split(inner, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colon := strings.Index(part, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(part[:colon])
		typ := tr.cleanCheckerType(part[colon+1:])
		fields = append(fields, exportField(name)+" "+typ)
	}
	return "struct { " + strings.Join(fields, "; ") + " }"
}

// applyTypeArgs substitutes Go type arguments into a generic alias target.
func (tr *transpiler) applyTypeArgs(info aliasInfo, args string) string {
	return info.target + "[" + args + "]"
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// primitiveAliases maps alias names that should translate to Go's native
// integer types (the "uint8 = number" convention) to their Go targets.
var primitiveAliases = map[string]string{
	"uint8": "uint8", "uint16": "uint16", "uint32": "uint32", "uint64": "uint64",
	"int8": "int8", "int16": "int16", "int32": "int32", "int64": "int64",
	"uint": "uint", "int": "int", "byte": "byte", "rune": "rune",
	"float32": "float32", "float64": "float64", "string": "string",
	"bool": "bool", "any": "any", "error": "error",
}

// collectAliases pre-scans top-level `type X = ...` declarations so type
// references resolve regardless of declaration order.
func (tr *transpiler) collectAliases() error {
	for _, stmt := range tr.sf.Statements() {
		if !stmt.IsTypeAliasDeclaration() {
			continue
		}
		td, _ := stmt.AsTypeAliasDeclaration()
		name := td.Name()
		tn, ok := td.TypeNode()
		if !ok {
			continue
		}
		// String-literal unions become a named string type + consts.
		if vals, isUnion := stringLiteralUnionValues(tn); isUnion {
			tr.aliases[name] = aliasInfo{name: name, target: name, unionValues: vals}
			continue
		}
		var target string
		// The `uint8 = number` convention: the alias NAME selects the Go
		// integer type; the RHS (`number`) is just TS syntax.
		if goType, ok := primitiveAliases[name]; ok {
			target = goType
		} else {
			t, err := tr.goType(tn)
			if err != nil {
				tr.recordGap(SevTodo, "type", stmt, "alias %s: %v", name, err)
				continue
			}
			target = t
		}
		tr.aliases[name] = aliasInfo{name: name, target: target}
	}
	// Re-resolve aliases that point at other aliases (fixpoint).
	for i := 0; i < len(tr.aliases); i++ {
		changed := false
		for k, a := range tr.aliases {
			if a.unionValues != nil {
				continue
			}
			if t, ok := tr.aliases[strings.TrimSpace(a.target)]; ok && t.name != a.name {
				tr.aliases[k] = aliasInfo{name: a.name, target: t.target}
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return nil
}

// stringLiteralUnionValues reports whether a type node is a union of string
// literals only (`"a" | "b"`) and returns the literal texts (quoted).
func stringLiteralUnionValues(tn tsmorph.Node) ([]string, bool) {
	if !ast.IsUnionTypeNode(tn.ASTNode()) {
		return nil, false
	}
	var vals []string
	for _, c := range tn.Children() {
		if !ast.IsLiteralTypeNode(c.ASTNode()) {
			return nil, false
		}
		t := strings.TrimSpace(c.Text())
		if !strings.HasPrefix(t, `"`) && !strings.HasPrefix(t, `'`) {
			return nil, false
		}
		vals = append(vals, normalizeQuote(t))
	}
	return vals, true
}

// normalizeQuote converts single-quoted literals to double-quoted so they are
// valid Go string literals.
func normalizeQuote(s string) string {
	if len(s) >= 2 && s[0] == '\'' {
		return `"` + strings.ReplaceAll(s[1:len(s)-1], `"`, `\"`) + `"`
	}
	return s
}

// collectDeclared records every name declared at the top level so calls to
// unknown identifiers can be flagged (they usually mean a module import the
// LLM must wire up). In multi-file mode the package-wide union is used, so
// calls to symbols in sibling files resolve cleanly.
func (tr *transpiler) collectDeclared() {
	if tr.pkgDeclared != nil {
		tr.declared = tr.pkgDeclared
		return
	}
	tr.declared = map[string]bool{}
	for _, stmt := range tr.sf.Statements() {
		if n := stmt.Name(); n != "" {
			tr.declared[n] = true
		}
		if vs, ok := stmt.AsVariableStatement(); ok {
			for _, d := range vs.Declarations() {
				if d.Name() != "" {
					tr.declared[d.Name()] = true
				}
			}
		}
	}
}

// freshName returns a unique identifier.
func (tr *transpiler) freshName(base string) string {
	tr.nextID++
	return fmt.Sprintf("%s_%d", base, tr.nextID)
}

// exportField converts a TS property name to an exported Go field name.
func exportField(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// goName converts a TS identifier to a valid Go identifier.
func goName(name string) string {
	return name
}

// firstChild returns the first child node (or a zero Node).
func firstChild(n tsmorph.Node) tsmorph.Node {
	cs := n.Children()
	if len(cs) == 0 {
		return tsmorph.Node{}
	}
	return cs[0]
}
