package transpile

import (
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// bannedCalls maps fully-qualified call targets (as written, e.g.
// "Object.create") to the reason they are banned. Checked against the AST
// (call/new expression callee text), so occurrences inside string literals
// or comments do not trigger false positives.
var bannedCalls = map[string]string{
	"Object.create":           "prototype manipulation",
	"Object.setPrototypeOf":   "prototype manipulation",
	"Object.defineProperty":   "hidden property definition",
	"Reflect.setPrototypeOf":  "prototype manipulation",
	"Reflect.construct":       "dynamic construction",
	"Reflect.apply":           "dynamic dispatch",
	"Reflect.defineProperty":  "hidden property definition",
	"eval":                    "eval-style dynamism",
	"Function":                "eval-style dynamism",
	"Proxy":                   "proxy interception",
	"Object.defineProperties": "hidden property definition",
}

// scanBans walks the AST and records every banned construct as a work item
// (severity banned). It does not abort transpilation: the construct is
// replaced by a TODO placeholder at emit time and shows up in the report.
func (tr *transpiler) scanBans() {
	var stack []tsmorph.Node
	stack = append(stack, tr.sf.RootNode())
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.IsZero() {
			continue
		}
		switch {
		case ast.IsWithStatement(n.ASTNode()):
			tr.recordGap(SevBanned, "dynamic", n, "with statement has no Go equivalent")
		case ast.IsCallExpression(n.ASTNode()), ast.IsNewExpression(n.ASTNode()):
			if callee, ok := n.GetExpression(); ok {
				if reason, bad := bannedCalls[strings.TrimSpace(callee.Text())]; bad {
					tr.recordGap(SevBanned, "dynamic", n, "banned: %s (%s)", callee.Text(), reason)
				}
			}
		}
		stack = append(stack, n.Children()...)
	}
}

// bannedCallee reports whether a call/new-expression node targets a banned
// callee; used at emit time to substitute a placeholder.
func bannedCallee(n tsmorph.Node) bool {
	callee, ok := n.GetExpression()
	if !ok {
		return false
	}
	_, bad := bannedCalls[strings.TrimSpace(callee.Text())]
	return bad
}

// lineAt returns the 1-based line number for a byte offset.
func lineAt(text string, offset int) int {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	return strings.Count(text[:offset], "\n") + 1
}
