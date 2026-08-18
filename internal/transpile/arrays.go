package transpile

import (
	"fmt"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// callbackInfo describes an inline-able callback (arrow function or
// function expression).
type callbackInfo struct {
	params []string     // parameter names
	body   tsmorph.Node // body node (block or expression)
	isExpr bool         // body is a single expression
}

// parseCallback extracts params and body from an arrow/function expression.
func parseCallback(cb tsmorph.Node) (callbackInfo, bool) {
	if !ast.IsArrowFunction(cb.ASTNode()) && !ast.IsFunctionExpression(cb.ASTNode()) {
		return callbackInfo{}, false
	}
	var info callbackInfo
	for _, c := range cb.Children() {
		if ast.IsParameterDeclaration(c.ASTNode()) {
			info.params = append(info.params, c.Name())
		}
	}
	body, ok := cb.GetBody()
	if !ok {
		return callbackInfo{}, false
	}
	info.body = body
	info.isExpr = !ast.IsBlock(body.ASTNode())
	return info, true
}

// emitBodyStatements writes a callback body (block or single expression) at
// the current indent.
func (tr *transpiler) emitCallbackBody(cb callbackInfo, retExpr func(string) string) error {
	if cb.isExpr {
		s, err := tr.emitExpr(cb.body)
		if err != nil {
			return err
		}
		tr.out.line(retExpr(s))
		return nil
	}
	return tr.emitBlock(cb.body)
}

// emitForEachStatement rewrites `arr.forEach(cb)` in statement position as
// an inline for-range loop. Returns handled=false for non-forEach calls.
func (tr *transpiler) emitForEachStatement(call tsmorph.Node) (bool, error) {
	callee, ok := call.GetExpression()
	if !ok || !ast.IsPropertyAccessExpression(callee.ASTNode()) {
		return false, nil
	}
	prop, _ := callee.GetNameNode()
	if prop.Text() != "forEach" {
		return false, nil
	}
	obj, _ := callee.GetExpression()
	objS, err := tr.emitExpr(obj)
	if err != nil {
		return true, err
	}
	// Iterating an any/`Object`-typed collection cannot range in Go — coerce
	// through the jsrt dynamic runtime (map iteration yields values).
	rangeExpr := objS
	if tr.isDynamicReceiver(obj) {
		tr.usedShim = true
		rangeExpr = "jsrtArray(" + objS + ")"
	}
	args := call.GetArguments()
	if len(args) == 0 {
		return true, fmt.Errorf("forEach without callback")
	}
	tr.recordGap(SevApprox, "iteration", call, "forEach mapped to for-range loop")

	elem := "x"
	var idx string
	if cb, ok := parseCallback(args[0]); ok {
		if len(cb.params) > 0 {
			elem = cb.params[0]
		}
		if len(cb.params) > 1 {
			idx = cb.params[1]
		}
		// Go range gives (i, v) — TS forEach gives (v, i).
		if idx != "" {
			tr.out.line("for " + idx + ", " + elem + " := range " + rangeExpr + " {")
		} else {
			tr.out.line("for _, " + elem + " := range " + rangeExpr + " {")
		}
		tr.out.indent()
		// Wrap the body in an IIFE so `return` inside the callback only
		// skips the element (TS semantics), not the enclosing function.
		tr.out.line("func() {")
		tr.out.indent()
		if err := tr.emitCallbackBody(cb, func(s string) string { return s }); err != nil {
			return true, err
		}
		tr.out.dedent()
		tr.out.line("}()")
		tr.out.dedent()
		tr.out.line("}")
		return true, nil
	}
	// Non-literal callback: plain loop calling it.
	tr.out.line("for _, " + elem + " := range " + rangeExpr + " {")
	tr.out.indent()
	s, err := tr.emitExpr(args[0])
	if err != nil {
		return true, err
	}
	tr.out.line(s + "(" + elem + ")")
	tr.out.dedent()
	tr.out.line("}")
	return true, nil
}

// callbackGoTypes resolves the callback's element and return types (as Go
// strings) from the checker, so map/reduce loops can be typed instead of
// falling back to any.
func (tr *transpiler) callbackGoTypes(cb tsmorph.Node, obj tsmorph.Node) (elem, ret string) {
	elem = strings.TrimPrefix(tr.goTypeFromChecker(obj), "[]")
	if elem == "" || strings.ContainsAny(elem, " {}|,") {
		elem = "any"
	}
	sigs := cb.Type().CallSignatures()
	if len(sigs) > 0 {
		ret = mapCheckerType(tr.cleanCheckerType(sigs[0].ReturnType().Text()))
	} else {
		ret = "any"
	}
	if strings.ContainsAny(ret, " {}|,") {
		ret = "any"
	}
	return elem, ret
}

// goTypeFromChecker maps a node's checker type text to a Go type string.
func (tr *transpiler) goTypeFromChecker(n tsmorph.Node) string {
	if n.IsZero() {
		return "any"
	}
	t := n.Type()
	if t.IsUnknown() {
		return "any"
	}
	return mapCheckerType(tr.cleanCheckerType(t.Text()))
}

// arrayMethodCall rewrites `.map`/`.filter`/`.reduce` (expression position,
// arrow callbacks) as loop-based IIFEs. Element/return types come from the
// checker where possible; otherwise they widen to any (flagged approx).
func (tr *transpiler) arrayMethodCall(n tsmorph.Node) (string, bool, error) {
	callee, ok := n.GetExpression()
	if !ok || !ast.IsPropertyAccessExpression(callee.ASTNode()) {
		return "", false, nil
	}
	prop, _ := callee.GetNameNode()
	method := prop.Text()
	if method != "map" && method != "filter" && method != "reduce" {
		return "", false, nil
	}
	obj, _ := callee.GetExpression()
	args := n.GetArguments()
	if len(args) == 0 {
		return "", false, nil
	}
	cb, isCB := parseCallback(args[0])
	if !isCB {
		return "", false, nil
	}
	objS, err := tr.emitExpr(obj)
	if err != nil {
		return "", true, err
	}
	// Iterating an any/`Object`-typed collection cannot range in Go — coerce
	// through the jsrt dynamic runtime (map iteration yields values, matching
	// JS `of`/iteration semantics).
	rangeExpr := objS
	if tr.isDynamicReceiver(obj) {
		tr.usedShim = true
		rangeExpr = "jsrtArray(" + objS + ")"
	}
	tr.recordGap(SevApprox, "iteration", n, "%s mapped to loop", method)

	elemGo, retGo := tr.callbackGoTypes(args[0], obj)
	cbElem := "x"
	if len(cb.params) > 0 {
		cbElem = cb.params[0]
	}
	out := tr.freshName("out")

	// Render the callback body as a nested closure string so `return`
	// inside it behaves like the TS arrow. retType of the closure matches
	// the callback's return type.
	bodyS, err := tr.renderCallbackClosure(cb, retGo)
	if err != nil {
		return "", true, err
	}

	var b strings.Builder
	switch method {
	case "map":
		fmt.Fprintf(&b, "func() []%s {\n%s := []%s{}\nfor _, %s := range %s {\n%s = append(%s, %s)\n}\nreturn %s\n}()",
			retGo, out, retGo, cbElem, rangeExpr, out, out, bodyS, out)
	case "filter":
		cond := bodyS
		if cb.isExpr {
			cond = stripClosure(bodyS)
		}
		fmt.Fprintf(&b, "func() []%s {\n%s := []%s{}\nfor _, %s := range %s {\nif %s {\n%s = append(%s, %s)\n}\n}\nreturn %s\n}()",
			elemGo, out, elemGo, cbElem, rangeExpr, cond, out, out, cbElem, out)
	case "reduce":
		// TS reduce callback signature is (acc, curValue[, i]).
		acc, v := "acc", "v"
		if len(cb.params) > 0 {
			acc = cb.params[0]
		}
		if len(cb.params) > 1 {
			v = cb.params[1]
		}
		init := "nil"
		if len(args) > 1 {
			s, err := tr.emitExpr(args[1])
			if err != nil {
				return "", true, err
			}
			init = s
		} else {
			tr.recordGap(SevTodo, "iteration", n, "reduce without initial value: acc starts at nil")
		}
		expr := bodyS
		if cb.isExpr {
			expr = stripClosure(bodyS)
		}
		fmt.Fprintf(&b, "func() %s {\nvar %s %s = %s\nfor _, %s := range %s {\n%s = %s\n}\nreturn %s\n}()",
			retGo, acc, retGo, init, v, rangeExpr, acc, expr, acc)
	}
	return b.String(), true, nil
}

// stripClosure unwraps `func() T { return EXPR }()` to EXPR.
func stripClosure(s string) string {
	prefix := "func() any { return "
	if strings.HasPrefix(s, prefix) {
		s = strings.TrimPrefix(s, prefix)
	} else if i := strings.Index(s, "{ return "); i >= 0 {
		s = s[i+len("{ return "):]
	}
	return strings.TrimSuffix(s, " }()")
}

// renderCallbackClosure renders a callback as `func() T { ... }()` —
// expression bodies become `return <expr>`; block bodies are emitted
// verbatim (their `return` exits only the closure).
func (tr *transpiler) renderCallbackClosure(cb callbackInfo, retType string) (string, error) {
	if cb.isExpr {
		s, err := tr.emitExpr(cb.body)
		if err != nil {
			return "", err
		}
		return "func() " + retType + " { return " + s + " }()", nil
	}
	saved := tr.out
	tmp := newGoWriter()
	tr.out = tmp
	tr.retStack = append(tr.retStack, retType)
	err := tr.emitBlock(cb.body)
	tr.retStack = tr.retStack[:len(tr.retStack)-1]
	tr.out = saved
	if err != nil {
		return "", err
	}
	inner := strings.TrimSuffix(tmp.String(), "\n")
	return "func() " + retType + " {\n" + inner + "\n}()", nil
}
