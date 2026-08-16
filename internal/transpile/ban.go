package transpile

import (
	"fmt"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// bannedSuffixes are the JS runtime mechanisms that have no faithful Go
// equivalent and are rejected up front. The goal is speed via idiomatic Go,
// so anything that depends on prototype mutation, dynamic dispatch, or
// eval-style dynamism is out of scope.
var bannedSuffixes = []string{
	// Prototype manipulation ("prototype insurance" mechanisms).
	".createPrototype", "setPrototypeOf", "__proto__", "Object.create",
	"Object.setPrototypeOf", "Reflect.setPrototypeOf", "Reflect.construct",
	"Reflect.apply", "Reflect.defineProperty",
	// Dynamic dispatch / eval.
	"eval(", "new Function", "Function(",
	// Proxy-based interception.
	"new Proxy", "Proxy(",
	// with statement.
	"with (",
}

// banCheck scans a source file for banned constructs and returns an error
// listing every hit. Run before transpiling so failures are upfront.
func banCheck(sf *tsmorph.SourceFile) error {
	var hits []string
	text := sf.Text()
	for _, suffix := range bannedSuffixes {
		if idx := strings.Index(text, suffix); idx >= 0 {
			hits = append(hits, fmt.Sprintf("line %d: banned construct %q", lineAt(text, idx), suffix))
		}
	}
	// AST-level checks: any `X.prototype.member = ...` assignment.
	for _, node := range allNodes(sf) {
		if !ast.IsBinaryExpression(node.ASTNode()) {
			continue
		}
		be, _ := node.AsBinaryExpression()
		op := be.OperatorText()
		if op != "=" && op != "+=" {
			continue
		}
		left, ok := be.Left()
		if !ok {
			continue
		}
		if isPrototypeAccess(left) {
			hits = append(hits, fmt.Sprintf("line %d: prototype mutation %q", lineAt(text, left.Pos()), left.Text()))
		}
	}
	if len(hits) == 0 {
		return nil
	}
	return fmt.Errorf("banned constructs in %s:\n  %s", sf.FilePath(), strings.Join(hits, "\n  "))
}

// isPrototypeAccess reports whether a node is `X.prototype` or `X.prototype.y`.
func isPrototypeAccess(n tsmorph.Node) bool {
	text := n.Text()
	return strings.Contains(text, ".prototype.")
}

// allNodes returns every node in the file (BFS).
func allNodes(sf *tsmorph.SourceFile) []tsmorph.Node {
	var out []tsmorph.Node
	var stack []tsmorph.Node
	stack = append(stack, sf.RootNode())
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, n)
		stack = append(stack, n.Children()...)
	}
	return out
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
