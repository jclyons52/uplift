// Package lift converts JavaScript (optionally JSDoc-typed) into annotated
// TypeScript by asking the TypeScript checker for the resolved type of every
// parameter and return value. It is the JS→TS bridge for the uplift
// pipeline: once a codebase (e.g. ESLint-style, JSDoc-only) is lifted to
// typed TS, uplift can transpile it without the any/dynamic gap cascade.
//
// The output is the original source with type annotations inserted — nothing
// else changes, so the lift is a safe, reviewable commit. Types the checker
// cannot resolve (any/unknown) are left unannotated rather than forced.
package lift

import (
	"regexp"
	"sort"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// insertion is a slice of text to insert at an absolute source offset.
type insertion struct {
	off  int
	text string
}

// Lift returns the JS source with TS type annotations inserted on every
// parameter and return value the checker resolves to a concrete type, plus
// TS type aliases for any JSDoc @typedef definitions.
func Lift(p *tsmorph.Project, sf *tsmorph.SourceFile) (string, error) {
	l := &lifter{src: sf.Text()}
	l.collectTypedefs(sf)
	for _, stmt := range sf.Statements() {
		l.walk(stmt)
	}
	out := l.render(sf.Text())
	if len(l.typedefs) > 0 {
		var lines []string
		for _, td := range l.typedefs {
			lines = append(lines, "type "+td.name+" = "+td.typeText+";")
		}
		out += "\n// lifted from JSDoc @typedef\n" + strings.Join(lines, "\n") + "\n"
	}
	return out, nil
}

// typedefInfo is one `@typedef` lifted to a TS `type` alias.
type typedefInfo struct{ name, typeText string }

// collectTypedefs finds JSDoc typedef statements in the file and parses their
// type + name (and @property members) into TS type aliases.
func (l *lifter) collectTypedefs(sf *tsmorph.SourceFile) {
	for _, stmt := range sf.Statements() {
		if !hasTypedefTag(stmt) {
			continue
		}
		td, ok := parseTypedef(stmt.Text())
		if !ok {
			continue
		}
		l.typedefs = append(l.typedefs, td)
	}
}

// hasTypedefTag reports whether a statement's leading JSDoc carries a
// `@typedef` tag.
func hasTypedefTag(stmt tsmorph.Node) bool {
	for _, d := range stmt.GetJsDocs() {
		for _, c := range d.Node.Children() {
			an := c.ASTNode()
			if an != nil && an.Kind.String() == "KindJSDocTypedefTag" {
				return true
			}
		}
	}
	return false
}

// parseTypedef turns a `@typedef` comment statement into a TS type alias.
// Forms handled: `@typedef {object} Foo` + `@property {T} name` members,
// `@typedef {T} Foo` (primitive/array), `@typedef {{...}} Foo` (inline).
func parseTypedef(text string) (typedefInfo, bool) {
	// Collapse comment decorations: /** * ... */
	s := stripStars(text)
	const tag = "@typedef"
	i := strings.Index(s, tag)
	if i < 0 {
		return typedefInfo{}, false
	}
	s = s[i+len(tag):] // after "@typedef"
	ty, rest := takeBracedType(s)
	if ty == "" {
		return typedefInfo{}, false
	}
	name := strings.Fields(rest)
	if len(name) == 0 {
		return typedefInfo{}, false
	}
	alias := name[0]

	// Collect @property members from the rest of the comment.
	var props [][2]string
	if strings.Contains(rest, "@property") || strings.Contains(rest, "@prop") {
		for _, m := range propRe.FindAllStringSubmatch(rest, -1) {
			props = append(props, [2]string{m[2], m[1]})
		}
	}

	typeText := ty
	if (ty == "object" || ty == "Object") && len(props) > 0 {
		typeText = "{\n" + joinProps(props, "  ") + "\n}"
	}
	if ty == "" && len(props) == 0 {
		return typedefInfo{}, false
	}
	return typedefInfo{name: alias, typeText: typeText}, true
}

// stripStars removes JSDoc comment decoration and line continuations so the
// body is a single spaced string.
func stripStars(s string) string {
	s = strings.ReplaceAll(s, "/*", " ")
	s = strings.ReplaceAll(s, "*/", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "*", " ")
	return strings.Join(strings.Fields(s), " ")
}

// takeBracedType returns the first `{ ... }` (nesting-aware) type expression
// and the text after it.
func takeBracedType(s string) (ty, rest string) {
	o := strings.Index(s, "{")
	if o < 0 {
		return "", s
	}
	depth := 0
	for i := o; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[o+1 : i]), strings.TrimSpace(s[i+1:])
			}
		}
	}
	return strings.TrimSpace(s[o+1:]), ""
}

// joinProps renders @property members as TS object-literal fields.
func joinProps(props [][2]string, indent string) string {
	parts := make([]string, 0, len(props))
	for _, p := range props {
		parts = append(parts, indent+p[0]+": "+p[1]+";")
	}
	return strings.Join(parts, "\n")
}

// propRe matches `@property {Type} name` / `@prop {Type} [name]`.
var propRe = regexp.MustCompile(`@(?:property|prop)\s*\{([^}]*)\}\s*\[?([A-Za-z_$][\w$]*)\]?`)

type lifter struct {
	src      string
	insert   []insertion
	typedefs []typedefInfo
}

// walk descends the tree, annotating every function-like node it finds
// (nested functions and arrows included).
func (l *lifter) walk(n tsmorph.Node) {
	if !n.IsZero() {
		l.maybeAnnotate(n)
	}
	for _, c := range n.Children() {
		l.walk(c)
	}
}

// maybeAnnotate inserts types for a function/arrow/method node, in reverse
// offset order so earlier insertions don't shift later ones.
func (l *lifter) maybeAnnotate(n tsmorph.Node) {
	node := n.ASTNode()
	if node == nil {
		return
	}
	isArrow := ast.IsArrowFunction(node)
	if !isArrow && !ast.IsFunctionDeclaration(node) && !ast.IsFunctionExpression(node) &&
		!ast.IsMethodDeclaration(node) && !ast.IsConstructorDeclaration(node) &&
		!ast.IsGetAccessorDeclaration(node) && !ast.IsSetAccessorDeclaration(node) &&
		!ast.IsFunctionTypeNode(node) {
		return
	}
	sigs := n.Type().CallSignatures()
	if len(sigs) == 0 {
		return
	}
	sig := sigs[0]

	// Return type: for arrows, insert before the `=>`; otherwise before the
	// body's opening brace. An unparenthesized single-param arrow annotated
	// here (`message => …` → `(message): any[] => …`) must wrap the param in
	// parens so the transcripted TS reparses (see `unpar` note below).
	firstParamStart, firstParamEnd := 0, 0
	if isArrow {
		for _, c := range n.Children() {
			if ast.IsParameterDeclaration(c.ASTNode()) {
				firstParamStart = c.Pos()
				firstParamEnd = c.End()
				break
			}
		}
	}
	unpar := isArrow && l.src[n.Pos()] != '('
	if isArrow {
		for _, c := range n.Children() {
			if ast.IsParameterDeclaration(c.ASTNode()) {
				firstParamStart, firstParamEnd = c.Pos(), c.End()
				break
			}
		}
	}
	ret := sig.ReturnType()
	if !ret.IsUnknown() && !ret.IsAny() && !ret.IsVoid() && !ret.IsNever() {
		if isArrow {
			if off := arrowFatArrow(node); off >= 0 {
				if unpar {
					l.insert = append(l.insert, insertion{off: firstParamStart, text: "("}, insertion{off: firstParamEnd, text: ")"})
				}
				l.insert = append(l.insert, insertion{off: off, text: ": " + clean(ret.Text())})
			}
		} else if body, ok := n.GetBody(); ok {
			l.insert = append(l.insert, insertion{off: body.Pos(), text: ": " + clean(ret.Text())})
		}
	}

	// Parameter types, aligned to the call signature by index.
	// An unparenthesized single bare param (`message => …`) that we annotate
	// becomes `message: T => …`, which ts-go-morph mis-parses back as
	// `map(message, T)`. Parenthesize so the transcripted TS reparses:
	// `(message: T) => …`.
	pi := 0
	for _, c := range n.Children() {
		if !ast.IsParameterDeclaration(c.ASTNode()) {
			continue
		}
		paramIsRest := hasDotDotDot(c.ASTNode())
		if pi < len(sig.Parameters()) {
			pt := sig.Parameters()[pi].Type
			if !pt.IsUnknown() && !pt.IsAny() {
				if unpar {
					l.insert = append(l.insert, insertion{off: c.Pos(), text: "("})
				}
				l.annotateParam(c, pt.Text(), paramIsRest, unpar)
			}
		}
		pi++
	}
}

// annotateParam inserts `: T` after a param's name node. Destructured params
// and params that already carry an annotation are skipped.
func (l *lifter) annotateParam(param tsmorph.Node, resolved, isRest string, closeParen bool) {
	// Skip if the param already has an inline type annotation.
	if strings.Contains(oneLine(param.Text()), ":") {
		return
	}
	name, ok := param.GetNameNode()
	if !ok {
		return
	}
	n := name.ASTNode()
	if n == nil || ast.IsObjectBindingPattern(n) || ast.IsArrayBindingPattern(n) {
		return
	}
	typ := clean(resolved)
	if typ == "" {
		return
	}
	// Optional params (JSDoc [x], checker "T | undefined") become `x?: T`;
	// rest params become `...args: T[]`.
	optional := strings.HasSuffix(strings.TrimSpace(resolved), "| undefined")
	var text string
	switch {
	case isRest != "":
		text = ": " + typ + "[]"
	case optional:
		text = "?: " + typ
	default:
		text = ": " + typ
	}
	if closeParen {
		text += ")"
	}
	l.insert = append(l.insert, insertion{off: name.End(), text: text})
}

// arrowFatArrow returns the source offset of an arrow function's `=>` token,
// or -1.
func arrowFatArrow(node *ast.Node) int {
	if node == nil || !ast.IsArrowFunction(node) {
		return -1
	}
	eg := node.AsArrowFunction().EqualsGreaterThanToken
	if eg == nil {
		return -1
	}
	return eg.Pos()
}

// hasDotDotDot reports whether a param is a rest param (`...args`).
func hasDotDotDot(node *ast.Node) string {
	if node == nil || !ast.IsParameterDeclaration(node) {
		return ""
	}
	if node.AsParameterDeclaration().DotDotDotToken != nil {
		return "..."
	}
	return ""
}

// clean maps a checker-rendered type text to TS: strips a trailing
// "| undefined" that the optional handling already consumed.
func clean(t string) string {
	s := strings.TrimSpace(t)
	s = strings.TrimSuffix(s, "| undefined")
	return strings.TrimSpace(s)
}

// oneLine collapses whitespace (for a contains-check on param text).
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// render applies the collected insertions in ascending offset order so the
// source text between insertions is written in order.
func (l *lifter) render(src string) string {
	if len(l.insert) == 0 {
		return src
	}
	sort.SliceStable(l.insert, func(i, j int) bool { return l.insert[i].off < l.insert[j].off })
	var b strings.Builder
	prev := 0
	for _, in := range l.insert {
		if in.off < prev || in.off > len(src) {
			continue // out of order or out of bounds; drop defensively
		}
		b.WriteString(src[prev:in.off])
		b.WriteString(in.text)
		prev = in.off
	}
	b.WriteString(src[prev:])
	return b.String()
}
