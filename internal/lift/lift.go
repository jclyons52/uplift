// Package lift converts JavaScript (optionally JSDoc-typed) into annotated
// TypeScript by asking the TypeScript checker for the resolved type of every
// parameter and return value. It is the JS→TS bridge for the ts2go
// pipeline: once a codebase (e.g. ESLint-style, JSDoc-only) is lifted to
// typed TS, ts2go can transpile it without the any/dynamic gap cascade.
//
// The output is the original source with type annotations inserted — nothing
// else changes, so the lift is a safe, reviewable commit. Types the checker
// cannot resolve (any/unknown) are left unannotated rather than forced.
package lift

import (
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
// parameter and return value the checker resolves to a concrete type.
func Lift(p *tsmorph.Project, sf *tsmorph.SourceFile) (string, error) {
	l := &lifter{src: sf.Text()}
	for _, stmt := range sf.Statements() {
		l.walk(stmt)
	}
	return l.render(sf.Text()), nil
}

type lifter struct {
	src    string
	insert []insertion
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
	// body's opening brace.
	ret := sig.ReturnType()
	if !ret.IsUnknown() && !ret.IsAny() && !ret.IsVoid() && !ret.IsNever() {
		if isArrow {
			if off := arrowFatArrow(node); off >= 0 {
				l.insert = append(l.insert, insertion{off: off, text: ": " + clean(ret.Text())})
			}
		} else if body, ok := n.GetBody(); ok {
			l.insert = append(l.insert, insertion{off: body.Pos(), text: ": " + clean(ret.Text())})
		}
	}

	// Parameter types, aligned to the call signature by index.
	pi := 0
	for _, c := range n.Children() {
		if !ast.IsParameterDeclaration(c.ASTNode()) {
			continue
		}
		paramIsRest := hasDotDotDot(c.ASTNode())
		if pi < len(sig.Parameters()) {
			pt := sig.Parameters()[pi].Type
			if !pt.IsUnknown() && !pt.IsAny() {
				l.annotateParam(c, pt.Text(), paramIsRest)
			}
		}
		pi++
	}
}

// annotateParam inserts `: T` after a param's name node. Destructured params
// and params that already carry an annotation are skipped.
func (l *lifter) annotateParam(param tsmorph.Node, resolved, isRest string) {
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
