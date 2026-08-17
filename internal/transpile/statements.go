package transpile

import (
	"fmt"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// ---------------------------------------------------------------------------
// Statements (function bodies)
// ---------------------------------------------------------------------------

// variableDeclExpr renders `let i = 0` (for-loop initializers and other
// expression-position declarations) as `i := 0`, typed float64 when the
// initializer is a number so arithmetic with other numbers type-checks.
func (tr *transpiler) variableDeclExpr(n tsmorph.Node) (string, error) {
	var decls []tsmorph.Node
	if ast.IsVariableDeclaration(n.ASTNode()) {
		decls = []tsmorph.Node{n}
	} else {
		for _, c := range n.Children() {
			if ast.IsVariableDeclaration(c.ASTNode()) {
				decls = append(decls, c)
			}
		}
	}
	var parts []string
	for _, dn := range decls {
		d, _ := dn.AsVariableDeclaration()
		name := d.Name()
		init, ok := d.Initializer()
		if !ok {
			parts = append(parts, "var "+name+" any")
			continue
		}
		s, err := tr.emitExpr(init)
		if err != nil {
			return "", err
		}
		if _, hasType := d.TypeNode(); !hasType && init.Type().IsNumber() {
			parts = append(parts, "var "+name+" float64 = "+s)
			continue
		}
		parts = append(parts, name+" := "+s)
	}
	return strings.Join(parts, ", "), nil
}

// emitBlock emits a `{ ... }` block's statements at the current indent. A
// panic inside one statement (a ts-go-morph accessor gap) degrades to a
// TODO placeholder; the rest of the block still emits.
func (tr *transpiler) emitBlock(n tsmorph.Node) error {
	for _, s := range n.GetStatements() {
		err := func() (err error) {
			defer recoverToError(&err)
			return tr.emitStatement(s)
		}()
		if err != nil {
			tr.fatal = append(tr.fatal, fmt.Sprintf("line %d: %v", tr.lineOf(s), err))
			tr.emitTodoStatement(s, "transpilation error: %v", err)
		}
	}
	return nil
}

// emitStatement emits one statement inside a function body.
func (tr *transpiler) emitStatement(n tsmorph.Node) error {
	switch {
	case ast.IsVariableStatement(n.ASTNode()):
		return tr.emitVariableStatement(n)
	case ast.IsReturnStatement(n.ASTNode()):
		expr := ""
		if e, ok := n.GetExpression(); ok {
			s, err := tr.emitExpr(e)
			if err != nil {
				return err
			}
			s = tr.convertReturnExpr(e, s)
			expr = " " + s
		}
		if tr.inDeferred > 0 {
			// Inside a recover/finally/try-IIFE a `return expr` cannot carry
			// a value; discard it — the caller cannot restructure the
			// enclosing function mechanically.
			if expr != "" {
				tr.recordGap(SevTodo, "errors", n, "return inside try/catch: value discarded; restructure function (named result)")
				tr.out.line("_ =" + expr)
			} else {
				tr.out.line("return")
			}
			return nil
		}
		tr.out.line("return" + expr)
		return nil
	case ast.IsIfStatement(n.ASTNode()):
		return tr.emitIfStatement(n)
	case ast.IsForStatement(n.ASTNode()):
		return tr.emitForStatement(n)
	case ast.IsForInStatement(n.ASTNode()), ast.IsForOfStatement(n.ASTNode()):
		return tr.emitForInOf(n)
	case ast.IsWhileStatement(n.ASTNode()):
		return tr.emitWhileStatement(n)
	case ast.IsDoStatement(n.ASTNode()):
		return tr.emitDoStatement(n)
	case ast.IsBlock(n.ASTNode()):
		tr.out.line("{")
		tr.out.indent()
		if err := tr.emitBlock(n); err != nil {
			return err
		}
		tr.out.dedent()
		tr.out.line("}")
		return nil
	case ast.IsExpressionStatement(n.ASTNode()):
		e, ok := n.GetExpression()
		if !ok {
			return nil
		}
		// `.forEach(cb)` in statement position → inline for-range loop.
		if ast.IsCallExpression(e.ASTNode()) {
			if handled, err := tr.emitForEachStatement(e); handled {
				return err
			}
			// `arr.push(x)` in statement position → `arr = append(arr, x)`.
			if s, handled, err := tr.pushStatement(e); handled {
				if err != nil {
					return err
				}
				tr.out.line(s)
				return nil
			}
		}
		s, err := tr.emitExpr(e)
		if err != nil {
			return err
		}
		// A bare placeholder expression is not a valid Go statement — bind
		// it to blank.
		if strings.HasPrefix(s, "any(nil)") {
			s = "_ = " + s
		}
		if s != "" {
			tr.out.line(s)
		}
		return nil
	case ast.IsThrowStatement(n.ASTNode()):
		// throw → panic (Go's closest primitive; error-return style is a
		// later refinement).
		expr := ""
		if e, ok := n.GetExpression(); ok {
			s, err := tr.emitExpr(e)
			if err != nil {
				return err
			}
			expr = s
		}
		tr.out.line("panic(" + expr + ")")
		return nil
	case ast.IsBreakStatement(n.ASTNode()):
		tr.out.line("break")
		return nil
	case ast.IsContinueStatement(n.ASTNode()):
		tr.out.line("continue")
		return nil
	case ast.IsSwitchStatement(n.ASTNode()):
		return tr.emitSwitchStatement(n)
	case ast.IsTryStatement(n.ASTNode()):
		return tr.emitTryStatement(n)
	case ast.IsClassDeclaration(n.ASTNode()):
		return tr.emitClass(n)
	case ast.IsFunctionDeclaration(n.ASTNode()):
		return tr.emitFunction(n)
	case ast.IsEmptyStatement(n.ASTNode()):
		return nil
	default:
		// Resilience: unsupported statements become TODO placeholders so
		// the rest of the file still transpiles and compiles.
		tr.emitTodoStatement(n, "unsupported statement %s", n.KindName())
		return nil
	}
}

// pushStatement rewrites `obj.push(x)` in statement position as
// `obj = append(obj, x)`.
func (tr *transpiler) pushStatement(call tsmorph.Node) (string, bool, error) {
	callee, ok := call.GetExpression()
	if !ok || !ast.IsPropertyAccessExpression(callee.ASTNode()) {
		return "", false, nil
	}
	prop, _ := callee.GetNameNode()
	if prop.Text() != "push" {
		return "", false, nil
	}
	obj, _ := callee.GetExpression()
	objS, err := tr.emitExpr(obj)
	if err != nil {
		return "", true, err
	}
	args, err := tr.emitArgs(call.GetArguments())
	if err != nil {
		return "", true, err
	}
	return objS + " = append(" + objS + ", " + args + ")", true, nil
}

func (tr *transpiler) emitIfStatement(n tsmorph.Node) error {
	ifs, _ := n.AsIfStatement()
	cond, ok := ifs.GetExpression()
	if !ok {
		return fmt.Errorf("if without condition")
	}
	cs, err := tr.emitExpr(cond)
	if err != nil {
		return err
	}
	thenNode, ok := ifs.ThenStatement()
	if !ok {
		return fmt.Errorf("if without then")
	}
	tr.out.line("if " + condExpr(cs) + " {")
	tr.out.indent()
	if ast.IsBlock(thenNode.ASTNode()) {
		if err := tr.emitBlock(thenNode); err != nil {
			return err
		}
	} else {
		if err := tr.emitStatement(thenNode); err != nil {
			return err
		}
	}
	tr.out.dedent()
	if elseNode, ok := ifs.ElseStatement(); ok {
		tr.out.line("} else {")
		tr.out.indent()
		if ast.IsBlock(elseNode.ASTNode()) {
			if err := tr.emitBlock(elseNode); err != nil {
				return err
			}
		} else {
			if err := tr.emitStatement(elseNode); err != nil {
				return err
			}
		}
		tr.out.dedent()
	}
	tr.out.line("}")
	return nil
}

func (tr *transpiler) emitForStatement(n tsmorph.Node) error {
	fs, _ := n.AsForStatement()
	init, hasInit := fs.Initializer()
	cond, hasCond := fs.Condition()
	inc, hasInc := fs.Incrementor()
	body, ok := fs.GetStatement()
	if !ok {
		return fmt.Errorf("for without body")
	}
	var initStr string
	if hasInit {
		s, err := tr.emitExpr(init)
		if err != nil {
			return err
		}
		initStr = s
	}
	var condStr string
	if hasCond {
		s, err := tr.emitExpr(cond)
		if err != nil {
			return err
		}
		condStr = condExpr(s)
	} else {
		condStr = "true"
	}
	var incStr string
	if hasInc {
		s, err := tr.emitExpr(inc)
		if err != nil {
			return err
		}
		incStr = s
	}
	tr.out.line("for " + initStr + ";" + condStr + ";" + incStr + " {")
	tr.out.indent()
	if ast.IsBlock(body.ASTNode()) {
		if err := tr.emitBlock(body); err != nil {
			return err
		}
	} else {
		if err := tr.emitStatement(body); err != nil {
			return err
		}
	}
	tr.out.dedent()
	tr.out.line("}")
	return nil
}

// loopVarFrom digs the loop-variable name out of a for-in/of initializer,
// which is a VariableDeclarationList → VariableDeclaration → identifier.
func loopVarFrom(init tsmorph.Node) string {
	if init.IsZero() {
		return ""
	}
	// Direct identifier name.
	if n := init.Name(); n != "" && init.Kind() != ast.KindVariableDeclarationList {
		return n
	}
	for _, c := range init.Children() {
		if ast.IsVariableDeclarationList(c.ASTNode()) {
			for _, d := range c.Children() {
				if ast.IsVariableDeclaration(d.ASTNode()) {
					return d.Name()
				}
			}
		}
		if ast.IsVariableDeclaration(c.ASTNode()) {
			return c.Name()
		}
	}
	return ""
}

// emitForInOf emits `for (const k in obj)` as `for k := range obj` and
// `for (const x of arr)` as `for _, x := range arr`.
func (tr *transpiler) emitForInOf(n tsmorph.Node) error {
	fio, _ := n.AsForInOrOfStatement()
	isIn := fio.IsForIn()
	expr, ok := fio.GetExpression()
	if !ok {
		return fmt.Errorf("for-each without iterable")
	}
	it, err := tr.emitExpr(expr)
	if err != nil {
		return err
	}
	body, ok := fio.GetStatement()
	if !ok {
		return fmt.Errorf("for-each without body")
	}
	loopVar := "k"
	if init, ok := fio.Initializer(); ok {
		if v := loopVarFrom(init); v != "" {
			loopVar = v
		}
	}
	if isIn {
		tr.out.line("for " + loopVar + " := range " + it + " {")
	} else {
		tr.out.line("for _, " + loopVar + " := range " + it + " {")
	}
	tr.out.indent()
	if ast.IsBlock(body.ASTNode()) {
		if err := tr.emitBlock(body); err != nil {
			return err
		}
	} else {
		if err := tr.emitStatement(body); err != nil {
			return err
		}
	}
	tr.out.dedent()
	tr.out.line("}")
	return nil
}

func (tr *transpiler) emitWhileStatement(n tsmorph.Node) error {
	ws, _ := n.AsWhileStatement()
	cond, ok := ws.GetExpression()
	if !ok {
		return fmt.Errorf("while without condition")
	}
	cs, err := tr.emitExpr(cond)
	if err != nil {
		return err
	}
	body, ok := ws.GetStatement()
	if !ok {
		return fmt.Errorf("while without body")
	}
	tr.out.line("for " + condExpr(cs) + " {")
	tr.out.indent()
	if ast.IsBlock(body.ASTNode()) {
		if err := tr.emitBlock(body); err != nil {
			return err
		}
	} else {
		if err := tr.emitStatement(body); err != nil {
			return err
		}
	}
	tr.out.dedent()
	tr.out.line("}")
	return nil
}

func (tr *transpiler) emitDoStatement(n tsmorph.Node) error {
	ds, _ := n.AsDoStatement()
	body, ok := ds.GetStatement()
	if !ok {
		return fmt.Errorf("do without body")
	}
	cond, ok := ds.GetExpression()
	if !ok {
		return fmt.Errorf("do without condition")
	}
	cs, err := tr.emitExpr(cond)
	if err != nil {
		return err
	}
	tr.out.line("for {")
	tr.out.indent()
	if ast.IsBlock(body.ASTNode()) {
		if err := tr.emitBlock(body); err != nil {
			return err
		}
	} else {
		if err := tr.emitStatement(body); err != nil {
			return err
		}
	}
	tr.out.line("if !(" + condExpr(cs) + ") { break }")
	tr.out.dedent()
	tr.out.line("}")
	return nil
}

func (tr *transpiler) emitSwitchStatement(n tsmorph.Node) error {
	ss, _ := n.AsSwitchStatement()
	expr, ok := ss.GetExpression()
	if !ok {
		return fmt.Errorf("switch without expression")
	}
	es, err := tr.emitExpr(expr)
	if err != nil {
		return err
	}
	tr.out.line("switch " + es + " {")
	tr.out.indent()
	hasDefault := false
	cb, ok := ss.CaseBlock()
	if ok {
		for _, c := range cb.Clauses() {
			if !ast.IsCaseClause(c.ASTNode()) {
				continue
			}
			label, ok := c.GetExpression()
			if ok {
				ls, err := tr.emitExpr(label)
				if err != nil {
					return err
				}
				tr.out.line("case " + ls + ":")
			} else {
				hasDefault = true
				tr.out.line("default:")
			}
			tr.out.indent()
			for _, s := range c.GetStatements() {
				if err := tr.emitStatement(s); err != nil {
					return err
				}
			}
			tr.out.dedent()
		}
	}
	if !hasDefault {
		// Go switches don't fall through, and a switch with no default can
		// leave a function without a return — add a panic default.
		tr.out.line("default:")
		tr.out.indent()
		tr.out.line(`panic("unhandled switch case")`)
		tr.out.dedent()
	}
	tr.out.dedent()
	tr.out.line("}")
	return nil
}

// emitTryStatement maps try/catch/finally to an IIFE with defer/recover.
// The catch binding (if any) is bound to the recovered value; the catch
// block is emitted inside the recover handler, which cannot return from the
// enclosing function — flagged as approx for the LLM to verify.
func (tr *transpiler) emitTryStatement(n tsmorph.Node) error {
	ts, _ := n.AsTryStatement()
	tryBlock, ok := ts.TryBlock()
	if !ok {
		return fmt.Errorf("try without block")
	}
	tr.recordGap(SevApprox, "errors", n, "try/catch via recover: catch body cannot return a value")

	tr.out.line("func() {")
	tr.out.indent()
	tr.out.line("defer func() {")
	tr.out.indent()
	tr.out.line("if r := recover(); r != nil {")
	tr.out.indent()
	tr.inDeferred++
	if cc, hasCatch := ts.CatchClause(); hasCatch {
		bind := "err"
		if vd, ok := cc.VariableDeclaration(); ok && vd.Name() != "" {
			bind = vd.Name()
		}
		tr.out.line(bind + " := r")
		tr.out.line("_ = " + bind)
		if cb, ok := cc.Block(); ok {
			if err := tr.emitBlock(cb.Node); err != nil {
				tr.inDeferred--
				return err
			}
		}
	} else {
		tr.out.line("_ = r")
	}
	tr.inDeferred--
	tr.out.dedent()
	tr.out.line("}")
	tr.out.dedent()
	tr.out.line("}()")
	tr.inDeferred++
	tryErr := tr.emitBlock(tryBlock.Node)
	tr.inDeferred--
	if tryErr != nil {
		return tryErr
	}
	if fb, hasFinally := ts.FinallyBlock(); hasFinally {
		tr.recordGap(SevApprox, "errors", fb.Node, "finally block: emitted after try body (runs even on panic via defer — verify placement)")
		tr.inDeferred++
		err := tr.emitBlock(fb.Node)
		tr.inDeferred--
		if err != nil {
			return err
		}
	}
	tr.out.dedent()
	tr.out.line("}()")
	// If the enclosing function returns a value and the try consumed its
	// returns, Go needs an endpoint: panic marks it unreachable-ish and
	// compiles. The LLM restructures per the recorded TODO.
	if len(tr.retStack) > 0 && tr.retStack[len(tr.retStack)-1] != "" {
		tr.out.line(`panic("ts2go: TODO: restructure try/catch returns")`)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// emitExpr renders a TS expression as a Go expression.
func (tr *transpiler) emitExpr(n tsmorph.Node) (string, error) {
	if n.IsZero() {
		return "", nil
	}
	k := n.Kind()
	switch {
	case ast.IsIdentifier(n.ASTNode()):
		return tr.identifier(n), nil
	case ast.IsStringLiteral(n.ASTNode()), ast.IsNoSubstitutionTemplateLiteral(n.ASTNode()):
		return n.Text(), nil
	case ast.IsNumericLiteral(n.ASTNode()):
		return tr.numericLiteral(n), nil
	case k == ast.KindTrueKeyword:
		return "true", nil
	case k == ast.KindFalseKeyword:
		return "false", nil
	case k == ast.KindNullKeyword:
		return "nil", nil
	case k == ast.KindThisKeyword:
		return "r", nil // receiver name in methods (correct for arrow captures)
	case k == ast.KindSuperKeyword:
		tr.recordGap(SevTodo, "inheritance", n, "super call needs manual dispatch to embedded type")
		return placeholderExpr("super"), nil
	case ast.IsPropertyAccessExpression(n.ASTNode()):
		return tr.propertyAccess(n)
	case ast.IsElementAccessExpression(n.ASTNode()):
		return tr.elementAccess(n)
	case ast.IsCallExpression(n.ASTNode()):
		return tr.callExpression(n)
	case ast.IsNewExpression(n.ASTNode()):
		return tr.newExpression(n)
	case ast.IsBinaryExpression(n.ASTNode()):
		return tr.binaryExpression(n)
	case ast.IsPrefixUnaryExpression(n.ASTNode()):
		return tr.prefixUnary(n)
	case ast.IsPostfixUnaryExpression(n.ASTNode()):
		return tr.postfixUnary(n)
	case ast.IsParenthesizedExpression(n.ASTNode()):
		inner, ok := n.GetExpression()
		if !ok {
			return "", nil
		}
		s, err := tr.emitExpr(inner)
		if err != nil {
			return "", err
		}
		return "(" + s + ")", nil
	case ast.IsVariableDeclarationList(n.ASTNode()), ast.IsVariableDeclaration(n.ASTNode()):
		return tr.variableDeclExpr(n)
	case ast.IsConditionalExpression(n.ASTNode()):
		return tr.conditionalExpression(n)
	case ast.IsArrayLiteralExpression(n.ASTNode()):
		return tr.arrayLiteral(n)
	case ast.IsObjectLiteralExpression(n.ASTNode()):
		return tr.objectLiteral(n)
	case ast.IsArrowFunction(n.ASTNode()), ast.IsFunctionExpression(n.ASTNode()):
		return tr.functionLiteral(n)
	case ast.IsTemplateExpression(n.ASTNode()):
		return tr.templateExpression(n)
	case ast.IsAsExpression(n.ASTNode()), ast.IsSatisfiesExpression(n.ASTNode()),
		ast.IsNonNullExpression(n.ASTNode()):
		inner, ok := n.GetExpression()
		if !ok {
			return "", nil
		}
		return tr.emitExpr(inner)
	case ast.IsVoidExpression(n.ASTNode()):
		return "", nil
	case ast.IsTypeOfExpression(n.ASTNode()):
		return tr.typeOfExpression(n)
	case ast.IsAwaitExpression(n.ASTNode()):
		return tr.awaitExpression(n)
	default:
		// Resilience: unsupported expressions become a compiling `any(nil)`
		// placeholder with an inline TODO; a report entry is recorded.
		return tr.todoExpr(n, "unsupported expression %s", n.KindName()), nil
	}
}

// awaitExpression maps `await e` to jsrtAwait — the shim parks the current
// async-call goroutine until the promise settles. (Inside the wrapper the
// whole body already runs on its own goroutine.)
func (tr *transpiler) awaitExpression(n tsmorph.Node) (string, error) {
	inner, ok := n.GetExpression()
	if !ok {
		return placeholderExpr("await with no operand"), nil
	}
	s, err := tr.emitExpr(inner)
	if err != nil {
		return "", err
	}
	tr.usedShim = true
	tr.recordGap(SevApprox, "async", n, "await via jsrt shim (blocks the async-call goroutine)")
	return "jsrtAwait(" + s + ")", nil
}

// identifier maps TS identifiers; reserved words get a trailing underscore.
func (tr *transpiler) identifier(n tsmorph.Node) string {
	name := n.Text()
	switch name {
	case "undefined":
		// nil works for pointer/slice/map/interface/any contexts; numeric
		// contexts need a zero value — flagged so the LLM checks.
		tr.recordGap(SevApprox, "literal", n, "undefined mapped to nil")
		return "nil"
	case "NaN":
		tr.used["math"] = true
		return "math.NaN()"
	case "Infinity":
		tr.used["math"] = true
		tr.recordGap(SevApprox, "literal", n, "Infinity mapped to math.Inf(1)")
		return "math.Inf(1)"
	case "Math":
		return "math"
	}
	return name
}

func (tr *transpiler) numericLiteral(n tsmorph.Node) string {
	t := n.Text()
	t = strings.ReplaceAll(t, "_", "")
	if strings.HasSuffix(t, "n") {
		t = strings.TrimSuffix(t, "n")
	}
	return t
}

// propertyAccess renders `obj.prop` — mapping TS built-ins to Go:
// .length → len(), .push → append.
func (tr *transpiler) propertyAccess(n tsmorph.Node) (string, error) {
	pa, _ := n.AsPropertyAccessExpression()
	obj, ok := pa.GetExpression()
	if !ok {
		return "", fmt.Errorf("property access without object")
	}
	prop, ok := pa.GetNameNode()
	if !ok {
		return "", fmt.Errorf("property access without name")
	}
	objS, err := tr.emitExpr(obj)
	if err != nil {
		return "", err
	}
	propS := prop.Text()
	// Property access on `any` cannot compile in Go — flag for the LLM.
	if t := obj.Type(); !t.IsUnknown() && t.IsAny() {
		tr.recordGap(SevTodo, "dynamic", n, "dynamic property access %s.%s on any", objS, propS)
		return placeholderExpr(objS + "." + propS), nil
	}
	switch propS {
	case "length":
		// TS `.length` is a number (→ float64); Go len() is int. Wrap so
		// arithmetic with other numbers type-checks.
		return "float64(len(" + objS + "))", nil
	case "toString":
		tr.used["fmt"] = true
		return "fmt.Sprint(" + objS + ")", nil
	case "push":
		return "append", nil
	}
	// Optional chaining (`a?.b`) loses its short-circuit in Go; flag it.
	for _, c := range n.Children() {
		if c.Kind() == ast.KindQuestionDotToken {
			tr.recordGap(SevApprox, "optional", n, "optional chaining %s?.%s emitted as direct access", objS, propS)
			break
		}
	}
	return objS + "." + exportField(propS), nil
}

func (tr *transpiler) elementAccess(n tsmorph.Node) (string, error) {
	ea, _ := n.AsElementAccessExpression()
	obj, ok := ea.GetExpression()
	if !ok {
		return "", fmt.Errorf("element access without object")
	}
	idx, ok := ea.ArgumentExpression()
	if !ok {
		return "", fmt.Errorf("element access without index")
	}
	objS, err := tr.emitExpr(obj)
	if err != nil {
		return "", err
	}
	idxS, err := tr.emitExpr(idx)
	if err != nil {
		return "", err
	}
	// Indexing an `any` cannot compile in Go — flag for the LLM.
	if t := obj.Type(); !t.IsUnknown() && t.IsAny() {
		tr.recordGap(SevTodo, "dynamic", n, "dynamic element access %s[%s] on any", objS, idxS)
		return placeholderExpr(objS + "[" + idxS + "]"), nil
	}
	return objS + "[" + idxS + "]", nil
}

func (tr *transpiler) callExpression(n tsmorph.Node) (string, error) {
	callee, ok := n.GetExpression()
	if !ok {
		return "", fmt.Errorf("call without callee")
	}
	// Banned callees (eval, Object.create, ...): placeholder + TODO. The
	// gap itself was already recorded by scanBans.
	if bannedCallee(n) {
		return placeholderExpr("banned: " + callee.Text()), nil
	}
	// `.map`/`.filter`/`.reduce` with a callback → loop-based IIFE.
	if ast.IsPropertyAccessExpression(callee.ASTNode()) {
		if s, handled, err := tr.arrayMethodCall(n); handled {
			return s, err
		}
	}
	// Special forms first.
	if ast.IsPropertyAccessExpression(callee.ASTNode()) {
		if s, handled, err := tr.callSpecial(n, callee); handled {
			return s, err
		}
	}
	// Identifier-callee builtins (parseFloat(x), isNaN(x), ...).
	if ast.IsIdentifier(callee.ASTNode()) {
		if s, handled := tr.identifierCall(n, callee.Text()); handled {
			return s, nil
		}
		if !tr.declared[callee.Text()] {
			tr.recordGap(SevTodo, "module", n, "call to %s: not defined in this file — wire up the Go equivalent/import", callee.Text())
		}
	}
	calleeS, err := tr.emitExpr(callee)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(calleeS, "any(nil)") {
		// Calling a placeholder (e.g. property access on any) cannot
		// compile; collapse to a placeholder for the whole call.
		return placeholderExpr("call"), nil
	}
	args, err := tr.emitArgs(n.GetArguments())
	if err != nil {
		return "", err
	}
	return calleeS + "(" + args + ")", nil
}

// identifierCall maps TS global function calls to Go equivalents.
func (tr *transpiler) identifierCall(n tsmorph.Node, name string) (string, bool) {
	args, err := tr.emitArgs(n.GetArguments())
	if err != nil {
		return "", false
	}
	switch name {
	case "parseFloat":
		tr.used["strconv"] = true
		return "func() float64 { v, _ := strconv.ParseFloat(" + args + ", 64); return v }()", true
	case "parseInt":
		tr.used["strconv"] = true
		return "func() int64 { v, _ := strconv.ParseInt(" + args + ", 10, 64); return v }()", true
	case "isNaN":
		tr.used["math"] = true
		return "math.IsNaN(" + args + ")", true
	case "isFinite":
		tr.used["math"] = true
		return "func() bool { return !math.IsInf(" + args + ", 0) }()", true
	case "String":
		tr.used["fmt"] = true
		return "fmt.Sprint(" + args + ")", true
	case "setTimeout":
		// setTimeout(cb, ms) → jsrtSetTimeout(ms, cb) (arg order swap).
		if raw := n.GetArguments(); len(raw) >= 2 {
			cb, err1 := tr.emitExpr(raw[0])
			ms, err2 := tr.emitExpr(raw[1])
			if err1 == nil && err2 == nil {
				tr.usedShim = true
				return "jsrtSetTimeout(" + ms + ", " + cb + ")", true
			}
		}
		return placeholderExpr("setTimeout"), true
	}
	return "", false
}

// callSpecial handles `arr.push(x)` → `append(arr, x)`, `console.log(...)`
// → `fmt.Println(...)`, string method mappings, and banned calls.
func (tr *transpiler) callSpecial(n, callee tsmorph.Node) (string, bool, error) {
	obj, _ := callee.GetExpression()
	prop, _ := callee.GetNameNode()
	objS, err := tr.emitExpr(obj)
	if err != nil {
		return "", true, err
	}
	propS := prop.Text()
	args := n.GetArguments()
	argStrs := make([]string, 0, len(args))
	for _, a := range args {
		s, err := tr.emitExpr(a)
		if err != nil {
			return "", true, err
		}
		argStrs = append(argStrs, s)
	}
	argsJ := strings.Join(argStrs, ", ")
	switch propS {
	case "push":
		return "append(" + objS + ", " + argsJ + ")", true, nil
	case "log", "error", "warn", "info":
		tr.used["fmt"] = true
		return "fmt.Println(" + argsJ + ")", true, nil
	case "substring", "substr", "slice":
		// s.substring(a) → s[a:]; s.substring(a, b) → s[a:b]
		switch len(argStrs) {
		case 1:
			return objS + "[" + argStrs[0] + ":]", true, nil
		case 2:
			return objS + "[" + argStrs[0] + ":" + argStrs[1] + "]", true, nil
		}
		return placeholderExpr("TODO(ts2go): " + propS), true, nil
	case "toUpperCase":
		tr.used["strings"] = true
		return "strings.ToUpper(" + objS + ")", true, nil
	case "toLowerCase":
		tr.used["strings"] = true
		return "strings.ToLower(" + objS + ")", true, nil
	case "trim":
		tr.used["strings"] = true
		return "strings.TrimSpace(" + objS + ")", true, nil
	case "includes":
		tr.used["strings"] = true
		return "strings.Contains(" + objS + ", " + argsJ + ")", true, nil
	case "startsWith":
		tr.used["strings"] = true
		return "strings.HasPrefix(" + objS + ", " + argsJ + ")", true, nil
	case "endsWith":
		tr.used["strings"] = true
		return "strings.HasSuffix(" + objS + ", " + argsJ + ")", true, nil
	case "join":
		tr.used["strings"] = true
		return "strings.Join(" + objS + ", " + argsJ + ")", true, nil
	case "parseInt":
		tr.used["strconv"] = true
		if len(argStrs) == 1 {
			return "func() int64 { v, _ := strconv.ParseInt(" + argStrs[0] + ", 10, 64); return v }()", true, nil
		}
		return placeholderExpr("TODO(ts2go): parseInt with radix"), true, nil
	case "parseFloat":
		tr.used["strconv"] = true
		return "func() float64 { v, _ := strconv.ParseFloat(" + strings.Join(argStrs, ", ") + ", 64); return v }()", true, nil
	case "Number":
		tr.used["strconv"] = true
		tr.recordGap(SevApprox, "conversion", n, "Number(x) mapped to strconv.ParseFloat — verify")
		return "func() float64 { v, _ := strconv.ParseFloat(" + argsJ + ", 64); return v }()", true, nil
	case "String":
		tr.used["fmt"] = true
		return "fmt.Sprint(" + argsJ + ")", true, nil
	}
	// Promise static methods → jsrt shim.
	if objS == "Promise" {
		switch propS {
		case "resolve":
			tr.usedShim = true
			return "jsrtResolve(" + argsJ + ")", true, nil
		case "reject":
			tr.usedShim = true
			tr.used["fmt"] = true
			return "jsrtReject(fmt.Errorf(\"%v\", " + argsJ + "))", true, nil
		case "all":
			tr.usedShim = true
			if len(args) == 1 && ast.IsArrayLiteralExpression(args[0].ASTNode()) {
				al, _ := args[0].AsArrayLiteralExpression()
				var elems []string
				for _, e := range al.GetElements() {
					s, err := tr.emitExpr(e)
					if err != nil {
						return "", true, err
					}
					elems = append(elems, s)
				}
				return "jsrtAll(" + strings.Join(elems, ", ") + ")", true, nil
			}
			tr.recordGap(SevTodo, "async", n, "Promise.all with non-literal array needs manual spread")
			return placeholderExpr("Promise.all"), true, nil
		case "then", "catch", "finally":
			tr.recordGap(SevTodo, "async", n, "promise.%s: rewrite as await (jsrt shim has no continuation chains)", propS)
			return placeholderExpr("promise." + propS), true, nil
		}
	}
	// Banned prototype-level tricks (also flagged by scanBans).
	full := objS + "." + propS
	if _, bad := bannedCalls[full]; bad {
		return placeholderExpr("banned: " + full), true, nil
	}
	return "", false, nil
}

func (tr *transpiler) newExpression(n tsmorph.Node) (string, error) {
	ne, _ := n.AsNewExpression()
	expr, ok := ne.GetExpression()
	if !ok {
		return "", fmt.Errorf("new without callee")
	}
	args, err := tr.emitArgs(ne.GetArguments())
	if err != nil {
		return "", err
	}
	callee := expr.Name()
	if callee == "" {
		callee = expr.Text()
	}
	// `new Error(msg)` → errors/fmt constructors.
	if callee == "Error" {
		if len(ne.GetArguments()) == 1 && ast.IsStringLiteral(ne.GetArguments()[0].ASTNode()) {
			tr.used["errors"] = true
			return "errors.New(" + args + ")", nil
		}
		tr.used["fmt"] = true
		tr.recordGap(SevApprox, "errors", n, "new Error mapped to fmt.Errorf (no stack/message semantics)")
		return "fmt.Errorf(\"%v\", " + args + ")", nil
	}
	if callee == "Map" {
		tr.recordGap(SevTodo, "dynamic", n, "new Map(...) needs explicit Go map type and methods")
		return placeholderExpr("new Map"), nil
	}
	if callee == "Set" {
		tr.recordGap(SevTodo, "dynamic", n, "new Set(...) needs map[T]struct{} or a set library")
		return placeholderExpr("new Set"), nil
	}
	return "New" + callee + "(" + args + ")", nil
}

func (tr *transpiler) binaryExpression(n tsmorph.Node) (string, error) {
	be, _ := n.AsBinaryExpression()
	left, ok := be.Left()
	if !ok {
		return "", fmt.Errorf("binary without left")
	}
	right, ok := be.Right()
	if !ok {
		return "", fmt.Errorf("binary without right")
	}
	op, err := tr.binaryOp(be)
	if err != nil {
		return "", err
	}
	ls, err := tr.emitExpr(left)
	if err != nil {
		return "", err
	}
	rs, err := tr.emitExpr(right)
	if err != nil {
		return "", err
	}
	switch op {
	case "===", "==":
		return ls + " == " + rs, nil
	case "!==", "!=":
		return ls + " != " + rs, nil
	case "&&":
		return ls + " && " + rs, nil
	case "||":
		return ls + " || " + rs, nil
	case "??":
		return tr.nullishCoalesce(ls, rs), nil
	case "**":
		tr.used["math"] = true
		return "math.Pow(" + ls + ", " + rs + ")", nil
	case "%":
		if tr.isFloatType(left) || tr.isFloatType(right) {
			tr.used["math"] = true
			return "math.Mod(" + ls + ", " + rs + ")", nil
		}
		return ls + " % " + rs, nil
	case "+":
		if tr.isStringType(left) || tr.isStringType(right) {
			tr.used["fmt"] = true
			return "fmt.Sprintf(\"%s%s\", " + ls + ", " + rs + ")", nil
		}
		return ls + " + " + rs, nil
	case "instanceof":
		tr.recordGap(SevTodo, "dynamic", n, "instanceof needs a manual type assertion/type switch")
		return placeholderExpr("instanceof"), nil
	case "in":
		tr.recordGap(SevTodo, "dynamic", n, "'in' operator needs a map lookup or type switch")
		return placeholderExpr("in-operator"), nil
	default:
		return ls + " " + op + " " + rs, nil
	}
}

// nullishCoalesce maps `a ?? b` to a nil-guard helper inline.
func (tr *transpiler) nullishCoalesce(ls, rs string) string {
	return "func() any { if " + ls + " != nil { return " + ls + " }; return " + rs + " }()"
}

// binaryOp extracts the operator text of a binary expression by slicing the
// source between the left operand's end and the right operand's start.
// (ts-go-morph's OperatorText() is unreliable: its IsToken treats
// identifiers as tokens, so the left operand is often returned instead.)
func (tr *transpiler) binaryOp(be tsmorph.BinaryExpression) (string, error) {
	left, ok := be.Left()
	if !ok {
		return "", fmt.Errorf("binary without left")
	}
	right, ok := be.Right()
	if !ok {
		return "", fmt.Errorf("binary without right")
	}
	start, end := left.End(), right.Pos()
	text := tr.sf.Text()
	if start < 0 || end > len(text) || start > end {
		// Fallback: middle child.
		kids := be.Children()
		if len(kids) >= 3 {
			return strings.TrimSpace(kids[1].Text()), nil
		}
		return "", fmt.Errorf("cannot locate binary operator")
	}
	return strings.TrimSpace(text[start:end]), nil
}

func (tr *transpiler) isStringType(n tsmorph.Node) bool {
	if n.IsZero() {
		return false
	}
	t := n.Type()
	return t.IsString() || t.IsStringLiteral()
}

// isFloatType reports whether an expression's TS type maps to a Go float
// (bare number → float64), meaning `%` must become math.Mod.
func (tr *transpiler) isFloatType(n tsmorph.Node) bool {
	if n.IsZero() {
		return false
	}
	t := n.Type()
	if t.IsUnknown() {
		return false
	}
	return mapCheckerType(tr.cleanCheckerType(t.Text())) == "float64"
}

// convertReturnExpr adapts a return expression to the enclosing function's
// declared Go return type where a mechanical conversion exists (notably
// string-literal-union values returned as string).
func (tr *transpiler) convertReturnExpr(e tsmorph.Node, s string) string {
	if len(tr.retStack) == 0 || s == "" {
		return s
	}
	ret := tr.retStack[len(tr.retStack)-1]
	if ret != "string" || e.IsZero() {
		return s
	}
	t := e.Type()
	if t.IsUnknown() {
		return s
	}
	if info, ok := tr.aliases[t.Text()]; ok && info.unionValues != nil {
		return "string(" + s + ")"
	}
	return s
}

func (tr *transpiler) prefixUnary(n tsmorph.Node) (string, error) {
	pu, _ := n.AsPrefixUnaryExpression()
	op := pu.OperatorText()
	operand, ok := pu.Operand()
	if !ok {
		return "", fmt.Errorf("unary without operand")
	}
	os, err := tr.emitExpr(operand)
	if err != nil {
		return "", err
	}
	switch op {
	case "!":
		return "!" + os, nil
	case "-":
		return "-" + os, nil
	case "+":
		return os, nil
	case "++":
		return os + "++", nil
	case "--":
		return os + "--", nil
	case "typeof":
		return tr.typeOfExpr(os, operand), nil
	case "void":
		return "", nil
	default:
		return op + os, nil
	}
}

func (tr *transpiler) postfixUnary(n tsmorph.Node) (string, error) {
	pu, _ := n.AsPostfixUnaryExpression()
	operand, ok := pu.Operand()
	if !ok {
		return "", fmt.Errorf("postfix without operand")
	}
	os, err := tr.emitExpr(operand)
	if err != nil {
		return "", err
	}
	return os + pu.OperatorText(), nil
}

func (tr *transpiler) conditionalExpression(n tsmorph.Node) (string, error) {
	ce, _ := n.AsConditionalExpression()
	cond, _ := ce.Condition()
	whenTrue, _ := ce.WhenTrue()
	whenFalse, _ := ce.WhenFalse()
	cs, err := tr.emitExpr(cond)
	if err != nil {
		return "", err
	}
	ts, err := tr.emitExpr(whenTrue)
	if err != nil {
		return "", err
	}
	fs, err := tr.emitExpr(whenFalse)
	if err != nil {
		return "", err
	}
	// Go has no ternary; use an IIFE for expression positions.
	return "func() any { if " + cs + " { return " + ts + " }; return " + fs + " }()", nil
}

func (tr *transpiler) arrayLiteral(n tsmorph.Node) (string, error) {
	al, _ := n.AsArrayLiteralExpression()
	var parts []string
	for _, e := range al.GetElements() {
		s, err := tr.emitExpr(e)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return "[]any{" + strings.Join(parts, ", ") + "}", nil
}

// typedObjectLiteral emits an object literal as a typed Go struct literal:
// `Account{Id: ..., Owner: ...}`.
func (tr *transpiler) typedObjectLiteral(n tsmorph.Node, typ string) (string, error) {
	ol, _ := n.AsObjectLiteralExpression()
	var parts []string
	for _, p := range ol.GetProperties() {
		key, val, err := tr.objectLiteralElement(p)
		if err != nil {
			return "", err
		}
		if key == "" {
			continue
		}
		parts = append(parts, key+": "+val)
	}
	return typ + "{" + strings.Join(parts, ", ") + "}", nil
}

func (tr *transpiler) objectLiteral(n tsmorph.Node) (string, error) {
	ol, _ := n.AsObjectLiteralExpression()
	var keys, parts []string
	for _, p := range ol.GetProperties() {
		key, val, err := tr.objectLiteralElement(p)
		if err != nil {
			return "", err
		}
		if key == "" {
			continue
		}
		keys = append(keys, key+" any")
		parts = append(parts, key+": "+val)
	}
	// When the checker resolves this literal to a named type, emit a typed
	// struct literal; otherwise fall back to an anonymous struct.
	typ := n.Type()
	if sym, ok := typ.Symbol(); ok {
		if name := sym.Name(); name != "" && !strings.HasPrefix(name, "__") {
			return name + "{" + strings.Join(parts, ", ") + "}", nil
		}
	}
	if len(parts) == 0 {
		// All elements were gaps (spread, methods, ...): an empty struct
		// literal is the compiling stand-in.
		return "struct{}{}", nil
	}
	return "struct{ " + strings.Join(keys, "; ") + " }{ " + strings.Join(parts, ", ") + " }", nil
}

// objectLiteralElement renders one element of an object literal as a Go
// keyed-field pair. The AST kind is checked before touching ts-go-morph
// accessors: Node.Initializer() panics on elements that aren't plain
// `key: value` assignments, so shorthand, spread, methods, and accessors
// must never reach it. Kinds without a Go struct-literal equivalent record a
// gap and return key "" (the element is skipped).
func (tr *transpiler) objectLiteralElement(p tsmorph.Node) (key, value string, err error) {
	node := p.ASTNode()
	switch {
	case ast.IsShorthandPropertyAssignment(node):
		// TS { a } means { a: a }.
		name := p.Name()
		return exportField(name), name, nil
	case ast.IsSpreadAssignment(node):
		tr.recordGap(SevTodo, "expression", p, "object spread has no Go struct-literal equivalent")
		return "", "", nil
	case ast.IsGetAccessorDeclaration(node), ast.IsSetAccessorDeclaration(node), ast.IsMethodDeclaration(node):
		tr.recordGap(SevTodo, "expression", p, "object-literal %s has no Go struct-literal equivalent — restructure", node.Kind.String())
		return "", "", nil
	case ast.IsPropertyAssignment(node):
		name := p.Name()
		init, ok := p.GetInitializer()
		if !ok {
			return "", "", nil
		}
		s, err := tr.emitExpr(init)
		if err != nil {
			return "", "", err
		}
		return exportField(name), s, nil
	default:
		tr.recordGap(SevTodo, "expression", p, "unhandled object-literal element %s", node.Kind.String())
		return "", "", nil
	}
}

func (tr *transpiler) functionLiteral(n tsmorph.Node) (string, error) {
	params, ret, err := tr.signatureFromChildren(n)
	if err != nil {
		return "", err
	}
	if ret == "" {
		ret = " any"
	}
	tr.retStack = append(tr.retStack, strings.TrimSpace(ret))
	defer func() { tr.retStack = tr.retStack[:len(tr.retStack)-1] }()
	if n.HasModifier(ast.ModifierFlagsAsync) {
		// Async arrow/function expression → shim call returning a promise.
		tr.usedShim = true
		tr.recordGap(SevApprox, "async", n, "async arrow via jsrt shim")
		tr.retStack[len(tr.retStack)-1] = "any"
		body, hasBody := n.GetBody()
		var inner string
		if hasBody {
			saved := tr.out
			tmp := newGoWriter()
			tr.out = tmp
			err := tr.emitBlock(body)
			tr.out = saved
			if err != nil {
				return "", err
			}
			inner = strings.TrimSuffix(tmp.String(), "\n")
		} else {
			inner = "\tpanic(\"not implemented\")"
		}
		return "jsrtAsync(func(" + params + ") any {\n" + inner + "\n})", nil
	}
	body, ok := n.GetBody()
	if !ok {
		return "func(" + params + ")" + ret + " { panic(\"not implemented\") }", nil
	}
	saved := tr.out
	tmp := newGoWriter()
	tr.out = tmp
	if err := tr.emitBlock(body); err != nil {
		tr.out = saved
		return "", err
	}
	tr.out = saved
	inner := strings.TrimSuffix(tmp.String(), "\n")
	return "func(" + params + ")" + ret + " {\n" + inner + "\n}", nil
}

func (tr *transpiler) templateExpression(n tsmorph.Node) (string, error) {
	te, _ := n.AsTemplateExpression()
	var formatParts []string
	var argParts []string
	head, ok := te.Head()
	if ok {
		formatParts = append(formatParts, escapeFmt(templateChunkText(head)))
	}
	for _, span := range te.TemplateSpans() {
		for _, cc := range span.Children() {
			if ast.IsTemplateMiddle(cc.ASTNode()) || ast.IsTemplateTail(cc.ASTNode()) {
				formatParts = append(formatParts, escapeFmt(templateChunkText(cc)))
			} else {
				s, err := tr.emitExpr(cc)
				if err != nil {
					return "", err
				}
				formatParts = append(formatParts, "%v")
				argParts = append(argParts, s)
			}
		}
	}
	if len(argParts) == 0 {
		return `"` + strings.Join(formatParts, "") + `"`, nil
	}
	tr.used["fmt"] = true
	return "fmt.Sprintf(\"" + strings.Join(formatParts, "") + "\", " + strings.Join(argParts, ", ") + ")", nil
}

// templateChunkText strips backtick/`${`/`}` wrappers from a template chunk.
// Head chunks are "`text ${", middles are "} text ${", tails are "} text`".
func templateChunkText(n tsmorph.Node) string {
	text := n.Text()
	text = strings.TrimPrefix(text, "`")
	text = strings.TrimPrefix(text, "}")
	text = strings.TrimSuffix(text, "`")
	text = strings.TrimSuffix(text, "${")
	return text
}

func (tr *transpiler) typeOfExpression(n tsmorph.Node) (string, error) {
	to, _ := n.AsTypeOfExpression()
	expr, ok := to.GetExpression()
	if !ok {
		return "", nil
	}
	s, err := tr.emitExpr(expr)
	if err != nil {
		return "", err
	}
	return tr.typeOfExpr(s, expr), nil
}

func (tr *transpiler) typeOfExpr(s string, operand tsmorph.Node) string {
	_ = operand
	tr.used["fmt"] = true
	return "fmt.Sprintf(\"%T\", " + s + ")"
}

// emitArgs renders call arguments.
func (tr *transpiler) emitArgs(args []tsmorph.Node) (string, error) {
	var parts []string
	for _, a := range args {
		if ast.IsSpreadElement(a.ASTNode()) {
			parts = append(parts, placeholderExpr("spread argument"))
			tr.recordGap(SevTodo, "dynamic", a, "spread argument needs manual expansion")
			continue
		}
		s, err := tr.emitExpr(a)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", "), nil
}

// snippet returns a short source excerpt for error messages.
func snippet(n tsmorph.Node) string {
	s := n.Text()
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// escapeFmt escapes % in format strings destined for fmt.Sprintf.
func escapeFmt(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}
