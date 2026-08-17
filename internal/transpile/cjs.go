package transpile

import (
	"path/filepath"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// CommonJS support: `require(...)` and `exports.X` / `module.exports`
// assignments, so plain JS modules (ESLint-class) transpile instead of
// cascading into `any`/dynamic gaps.

// requireSpecOf returns the module specifier of a `require("...")`
// CallExpression, or false.
func requireSpecOf(n tsmorph.Node) (spec string, ok bool) {
	if !ast.IsCallExpression(n.ASTNode()) {
		return "", false
	}
	callee, okc := n.GetExpression()
	if !okc || callee.Text() != "require" {
		return "", false
	}
	args := n.GetArguments()
	if len(args) < 1 {
		return "", false
	}
	if !ast.IsStringLiteral(args[0].ASTNode()) {
		return "", false
	}
	return strings.Trim(args[0].Text(), `"'`), true
}

// classifyRequire classifies a `require(spec)` like an ESM import
// (same-package dropped, sibling/external gapped) and reports whether the
// call was a require at all.
func (tr *transpiler) classifyRequire(n tsmorph.Node) bool {
	spec, ok := requireSpecOf(n)
	if !ok {
		return false
	}
	if tr.classify != nil {
		cls, target := tr.classify(tr.sf.FilePath(), spec)
		switch cls {
		case importSamePackage:
			// The required module's exports are Go symbols in the same
			// package; nothing further to emit.
		case importSiblingPackage:
			tr.recordGap(SevTodo, "import", n, "require of sibling package %q — wire up the Go import (package %s)", spec, target)
		default:
			tr.recordGap(SevTodo, "import", n, "external require %q — port or stub this module", spec)
		}
	}
	return true
}

// commonJSExport describes an `exports.X = v` / `module.exports = v`
// assignment.
type commonJSExport struct {
	name  string       // Go export name ("" for a single default export)
	value tsmorph.Node // RHS value
}

// parseCommonJSExport inspects an assignment expression and returns the CJS
// export it represents, or nil.
func (tr *transpiler) parseCommonJSExport(n tsmorph.Node) (*commonJSExport, bool) {
	be, ok := n.AsBinaryExpression()
	if !ok {
		return nil, false
	}
	op, err := tr.binaryOp(be)
	if err != nil || op != "=" {
		return nil, false
	}
	lhs, lok := be.Left()
	rhs, rok := be.Right()
	if !lok || !rok {
		return nil, false
	}
	member, ok := cjsExportTarget(lhs)
	if !ok {
		return nil, false
	}
	return &commonJSExport{name: member, value: rhs}, true
}

// cjsExportTarget returns the member being exported by a CJS assignment
// target — "" for a default `module.exports = v` — or false if the left side
// is not a CJS export.
func cjsExportTarget(lhs tsmorph.Node) (member string, ok bool) {
	if ast.IsIdentifier(lhs.ASTNode()) {
		return "", lhs.Name() == "exports"
	}
	if !ast.IsPropertyAccessExpression(lhs.ASTNode()) {
		return "", false
	}
	pa, _ := lhs.AsPropertyAccessExpression()
	obj, o := pa.Expression()
	if !o {
		return "", false
	}
	// module.exports.foo -> foo
	if ast.IsPropertyAccessExpression(obj.ASTNode()) {
		ob, _ := obj.AsPropertyAccessExpression()
		inner, _ := ob.Expression()
		if inner.Text() == "module" && obj.Name() == "exports" {
			return lhs.Name(), true
		}
		return "", false
	}
	// exports.foo -> foo
	if obj.Text() == "exports" {
		return lhs.Name(), true
	}
	// module.exports -> default export
	if obj.Text() == "module" && lhs.Name() == "exports" {
		return "", true
	}
	return "", false
}

// handleCommonJSExport emits a CJS export assignment (a named export or a
// default `module.exports = ...`). Returns handled.
func (tr *transpiler) handleCommonJSExport(expr tsmorph.Node) (bool, error) {
	e, ok := tr.parseCommonJSExport(expr)
	if !ok {
		return false, nil
	}
	if e.name == "" {
		return true, tr.emitDefaultExport(e.value)
	}
	return true, tr.emitNamedExport(e.name, e.value)
}

// handleCommonJSRequireStatement handles `const x = require("...")` as an
// import: classify the specifier and drop the binding (same-package exports
// are Go symbols in the package). Returns handled (only when the statement is
// a single-declaration require).
func (tr *transpiler) handleCommonJSRequireStatement(n tsmorph.Node) bool {
	vs, ok := n.AsVariableStatement()
	if !ok {
		return false
	}
	decls := vs.Declarations()
	if len(decls) != 1 {
		return false
	}
	init, ok := decls[0].GetInitializer()
	if !ok || !tr.classifyRequire(init) {
		return false
	}
	return true
}

// emitNamedExport emits one CJS named export (`exports.foo = v` or a member
// of `module.exports = {...}`) as a Go top-level symbol.
func (tr *transpiler) emitNamedExport(name string, value tsmorph.Node) error {
	tr.declared[name] = true
	goName := goSafeName(name)
	an := value.ASTNode()
	switch {
	case ast.IsFunctionExpression(an), ast.IsArrowFunction(an), ast.IsFunctionDeclaration(an):
		return tr.emitFunctionWithName(value, goName, false)
	case ast.IsShorthandPropertyAssignment(an):
		// `{ parse }` ≡ exporting the already-declared `parse`.
		return nil
	case ast.IsIdentifier(an):
		// Re-export of an existing symbol: expose under `goName` only when it
		// differs (`exports.foo = bar`).
		if goName != value.Name() {
			tr.out.line("var " + goName + " = " + value.Name())
		}
		return nil
	case ast.IsObjectLiteralExpression(an):
		ol, _ := value.AsObjectLiteralExpression()
		for _, p := range ol.GetProperties() {
			mn, mv, mok := tr.cjsObjectMember(p)
			if !mok {
				continue
			}
			if err := tr.emitNamedExport(mn, mv); err != nil {
				return err
			}
		}
		return nil
	default:
		s, err := tr.emitExpr(value)
		if err != nil {
			return err
		}
		if s != "" {
			tr.out.line("var " + goName + " = " + s)
		}
		return nil
	}
}

// goKeywords are Go reserved words; an identifier with one of these names
// cannot be a Go declaration.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true, "interface": true,
	"map": true, "package": true, "range": true, "return": true, "select": true,
	"struct": true, "switch": true, "type": true, "var": true,
}

// goSafeName escapes Go reserved words by appending `_`.
func goSafeName(name string) string {
	if goKeywords[name] {
		return name + "_"
	}
	return name
}

// emitDefaultExport emits `module.exports = <value>`: an object literal
// becomes one export per member; any other single value is flagged (default
// interop is not emitted).
func (tr *transpiler) emitDefaultExport(value tsmorph.Node) error {
	an := value.ASTNode()
	if ast.IsObjectLiteralExpression(an) {
		ol, _ := value.AsObjectLiteralExpression()
		for _, p := range ol.GetProperties() {
			mn, mv, mok := tr.cjsObjectMember(p)
			if !mok {
				continue
			}
			if err := tr.emitNamedExport(mn, mv); err != nil {
				return err
			}
		}
		return nil
	}
	if ast.IsFunctionExpression(an) || ast.IsArrowFunction(an) {
		// module.exports = anonymous function — the module's single default.
		// Name it after the source file so it is addressable and the module
		// stays self-contained.
		return tr.emitFunctionWithName(value, tr.defaultExportName(), false)
	}
	if ast.IsIdentifier(an) {
		// module.exports = existingName — the symbol is already top-level.
		tr.declared[value.Name()] = true
		return nil
	}
	tr.recordGap(SevTodo, "import", value, "module.exports = single value — default interop not emitted; give it a named export")
	return nil
}

// defaultExportName returns a deterministic Go name for an unnamed
// `module.exports = function` default, derived from the source file's base
// name (e.g. options.js -> Options).
func (tr *transpiler) defaultExportName() string {
	base := filepath.Base(tr.sf.FilePath())
	base = strings.TrimSuffix(base, filepath.Ext(base))
	var b strings.Builder
	up := true
	for _, r := range base {
		letter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
		digit := r >= '0' && r <= '9'
		if letter {
			if up {
				r = uppercase(r)
				up = false
			}
			b.WriteRune(r)
		} else if digit {
			if up && b.Len() == 0 {
				b.WriteRune('D')
			}
			b.WriteRune(r)
			up = false
		} else {
			up = true // separator: next letter capitalized
		}
	}
	name := strings.TrimSuffix(b.String(), "_")
	if name == "" {
		name = "Module"
	}
	return name
}

func uppercase(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 'a' + 'A'
	}
	return r
}

// cjsObjectMember returns the name and value node of an object-literal member
// that represents a CJS export.
func (tr *transpiler) cjsObjectMember(p tsmorph.Node) (name string, val tsmorph.Node, ok bool) {
	an := p.ASTNode()
	switch {
	case ast.IsShorthandPropertyAssignment(an):
		return p.Name(), p, true
	case ast.IsPropertyAssignment(an):
		n := p.Name()
		if n == "" {
			return "", tsmorph.Node{}, false
		}
		v, vok := p.GetInitializer()
		if !vok {
			return "", tsmorph.Node{}, false
		}
		return n, v, true
	}
	return "", tsmorph.Node{}, false
}
