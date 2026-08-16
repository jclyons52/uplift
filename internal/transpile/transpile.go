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
	// errors collects non-fatal diagnostics; fatal ones return directly.
	errors []string
	// used tracks which stdlib packages the emitted code references, so the
	// import block only declares what is actually used.
	used map[string]bool
}

type aliasInfo struct {
	name   string // alias name as written (e.g. "uint8")
	target string // resolved Go type (e.g. "uint8")
}

// NewTranspiler returns a transpiler bound to a source file.
func NewTranspiler(p *tsmorph.Project, sf *tsmorph.SourceFile) *Transpiler {
	return &Transpiler{tr: &transpiler{p: p, sf: sf, out: newGoWriter(), aliases: map[string]aliasInfo{}, used: map[string]bool{}}}
}

// Transpiler is the public entry point.
type Transpiler struct {
	tr *transpiler
}

// Transpile converts the source file to Go source text.
func (t *Transpiler) Transpile() (string, error) {
	tr := t.tr
	if err := banCheck(tr.sf); err != nil {
		return "", err
	}
	if err := tr.collectAliases(); err != nil {
		return "", err
	}
	// Emit the body first so the preamble can declare only the imports that
	// the emitted code actually references.
	body := newGoWriter()
	saved := tr.out
	tr.out = body
	for _, stmt := range tr.sf.Statements() {
		if err := tr.transpileStatement(stmt, false); err != nil {
			tr.out = saved
			return "", err
		}
	}
	tr.out = saved
	if len(tr.errors) > 0 {
		return "", fmt.Errorf("transpile errors:\n  %s", strings.Join(tr.errors, "\n  "))
	}
	// Now emit the preamble (imports only what was used) and glue it on top.
	pre := newGoWriter()
	tr.out = pre
	if err := tr.emitPreamble(); err != nil {
		return "", err
	}
	tr.out = saved
	return pre.String() + body.String(), nil
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
	case ast.IsKeywordTypeNode(n.ASTNode()):
		return tr.goKeywordType(n)
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
	name := n.Text()
	// Strip type arguments: `Vec3[]` handled elsewhere; `Array<T>` etc.
	if i := strings.Index(name, "<"); i >= 0 {
		base := name[:i]
		args := name[i+1 : len(name)-1]
		// Generic alias: preserve the alias name, keep Go-style args.
		if info, ok := tr.aliases[base]; ok {
			return tr.applyTypeArgs(info, args), nil
		}
		switch base {
		case "Array":
			return "[]" + strings.TrimSpace(args), nil
		case "Record":
			return "map[" + strings.TrimSpace(args) + "]", nil
		case "Promise":
			return "any /* Promise<" + args + "> */", nil
		default:
			return base + "[" + args + "]", nil
		}
	}
	// Plain reference: alias (uint8 → uint8) or named type (Vec3 → Vec3).
	if info, ok := tr.aliases[name]; ok {
		return info.target, nil
	}
	return name, nil
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
	// `T | null` handled by caller; strip trailing ` | undefined`.
	s = strings.TrimSuffix(s, " | undefined")
	s = strings.TrimSuffix(s, " | null")
	s = strings.TrimSpace(s)
	if s == "" || s == "any" {
		return "any"
	}
	return mapCheckerType(s)
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
		var target string
		// The `uint8 = number` convention: the alias NAME selects the Go
		// integer type; the RHS (`number`) is just TS syntax.
		if goType, ok := primitiveAliases[name]; ok {
			target = goType
		} else {
			t, err := tr.goType(tn)
			if err != nil {
				tr.errors = append(tr.errors, fmt.Sprintf("alias %s: %v", name, err))
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

// freshName returns a unique identifier.
func (tr *transpiler) freshName(base string) string {
	tr.nextID++
	return fmt.Sprintf("%s_%d", base, tr.nextID)
}

func (tr *transpiler) warnf(format string, args ...any) {
	tr.errors = append(tr.errors, fmt.Sprintf(format, args...))
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
