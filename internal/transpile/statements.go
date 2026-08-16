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

// emitBlock emits a `{ ... }` block's statements at the current indent.
func (tr *transpiler) emitBlock(n tsmorph.Node) error {
	for _, s := range n.GetStatements() {
		if err := tr.emitStatement(s); err != nil {
			return err
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
			expr = " " + s
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
		// `arr.push(x)` in statement position → `arr = append(arr, x)`.
		if ast.IsCallExpression(e.ASTNode()) {
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
		return fmt.Errorf("unsupported statement %s at %q", n.KindName(), snippet(n))
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
	tr.out.line("if " + cs + " {")
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
		condStr = s
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
	tr.out.line("for " + cs + " {")
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
	tr.out.line("if !(" + cs + ") { break }")
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

func (tr *transpiler) emitTryStatement(n tsmorph.Node) error {
	ts, _ := n.AsTryStatement()
	tryBlock, ok := ts.TryBlock()
	if !ok {
		return fmt.Errorf("try without block")
	}
	tr.out.line("func() {")
	tr.out.indent()
	tr.out.line("defer func() {")
	tr.out.indent()
	tr.out.line("if r := recover(); r != nil {")
	tr.out.indent()
	tr.out.line("_ = r // caught exception mapped to recover()")
	tr.out.dedent()
	tr.out.line("}")
	tr.out.dedent()
	tr.out.line("}()")
	if err := tr.emitBlock(tryBlock.Node); err != nil {
		return err
	}
	tr.out.dedent()
	tr.out.line("}()")
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
		return "r", nil // receiver name in methods
	case k == ast.KindSuperKeyword:
		return "", nil
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
	default:
		return "", fmt.Errorf("unsupported expression %s at %q", n.KindName(), snippet(n))
	}
}

// identifier maps TS identifiers; reserved words get a trailing underscore.
func (tr *transpiler) identifier(n tsmorph.Node) string {
	name := n.Text()
	switch name {
	case "undefined", "NaN", "Infinity":
		return "0" // conservative zero mapping; callers may special-case
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
	switch propS {
	case "length":
		return "len(" + objS + ")", nil
	case "toString":
		tr.used["fmt"] = true
		return "fmt.Sprint(" + objS + ")", nil
	case "push":
		return "append", nil
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
	return objS + "[" + idxS + "]", nil
}

func (tr *transpiler) callExpression(n tsmorph.Node) (string, error) {
	callee, ok := n.GetExpression()
	if !ok {
		return "", fmt.Errorf("call without callee")
	}
	// Special forms first.
	if ast.IsPropertyAccessExpression(callee.ASTNode()) {
		if s, handled, err := tr.callSpecial(n, callee); handled {
			return s, err
		}
	}
	calleeS, err := tr.emitExpr(callee)
	if err != nil {
		return "", err
	}
	args, err := tr.emitArgs(n.GetArguments())
	if err != nil {
		return "", err
	}
	return calleeS + "(" + args + ")", nil
}

// callSpecial handles `arr.push(x)` → `append(arr, x)`, `console.log(...)`
// → `fmt.Println(...)`, and bans prototype-level tricks.
func (tr *transpiler) callSpecial(n, callee tsmorph.Node) (string, bool, error) {
	obj, _ := callee.GetExpression()
	prop, _ := callee.GetNameNode()
	objS, err := tr.emitExpr(obj)
	if err != nil {
		return "", true, err
	}
	propS := prop.Text()
	args, err := tr.emitArgs(n.GetArguments())
	if err != nil {
		return "", true, err
	}
	switch propS {
	case "push":
		return "append(" + objS + ", " + args + ")", true, nil
	case "log", "error", "warn", "info":
		return "fmt.Println(" + args + ")", true, nil
	case "substring", "substr", "slice":
		return objS + "[" + args + "]", true, nil
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
		return "strings.Contains(" + objS + ", " + args + ")", true, nil
	case "startsWith":
		tr.used["strings"] = true
		return "strings.HasPrefix(" + objS + ", " + args + ")", true, nil
	case "endsWith":
		tr.used["strings"] = true
		return "strings.HasSuffix(" + objS + ", " + args + ")", true, nil
	case "join":
		tr.used["strings"] = true
		return "strings.Join(" + objS + ", " + args + ")", true, nil
	case "parseInt", "parseFloat", "Number":
		return args, true, nil
	case "String":
		tr.used["fmt"] = true
		return "fmt.Sprint(" + args + ")", true, nil
	}
	// Banned prototype-level tricks.
	full := objS + "." + propS
	switch full {
	case "Object.create", "Object.setPrototypeOf", "Reflect.setPrototypeOf",
		"Reflect.construct", "Reflect.apply", "Proxy":
		return "", true, fmt.Errorf("banned: %s (prototype/dynamic-dispatch mechanism)", full)
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
	case "+":
		if tr.isStringType(left) || tr.isStringType(right) {
			tr.used["fmt"] = true
			return "fmt.Sprintf(\"%s%s\", " + ls + ", " + rs + ")", nil
		}
		return ls + " + " + rs, nil
	case "instanceof":
		return "", fmt.Errorf("instanceof is banned: use type switches in Go")
	case "in":
		return "", fmt.Errorf("'in' operator is banned: use map lookups in Go")
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
		name := p.Name()
		init, ok := p.GetInitializer()
		if !ok {
			continue
		}
		s, err := tr.emitExpr(init)
		if err != nil {
			return "", err
		}
		parts = append(parts, exportField(name)+": "+s)
	}
	return typ + "{" + strings.Join(parts, ", ") + "}", nil
}

func (tr *transpiler) objectLiteral(n tsmorph.Node) (string, error) {
	ol, _ := n.AsObjectLiteralExpression()
	var parts []string
	for _, p := range ol.GetProperties() {
		name := p.Name()
		init, ok := p.GetInitializer()
		if !ok {
			continue
		}
		s, err := tr.emitExpr(init)
		if err != nil {
			return "", err
		}
		parts = append(parts, exportField(name)+": "+s)
	}
	// When the checker resolves this literal to a named type, emit a typed
	// struct literal; otherwise fall back to an anonymous struct.
	typ := n.Type()
	if sym, ok := typ.Symbol(); ok {
		if n := sym.Name(); n != "" && !strings.HasPrefix(n, "__") {
			return n + "{" + strings.Join(parts, ", ") + "}", nil
		}
	}
	return "struct{ " + strings.Join(parts, ", ") + " }{}", nil
}

func (tr *transpiler) functionLiteral(n tsmorph.Node) (string, error) {
	params, ret, err := tr.signatureFromChildren(n)
	if err != nil {
		return "", err
	}
	if ret == "" {
		ret = " any"
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
			return "", fmt.Errorf("spread arguments are banned in v1")
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
