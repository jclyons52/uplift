package transpile

import (
	"fmt"
	"testing"

	"github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

func TestArrowUnparenProbe(t *testing.T) {
	src := `
declare const messages: any;
const x = messages.map(message: any[] => { let messageType: any; return [messageType]; });
`
	proj, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	sf := proj.CreateSourceFile("/probe.ts", src)
	count := 0
	var findCall func() bool
	findCall = func() bool {
		for _, st := range sf.Statements() {
			_ = st
		}
		return false
	}
	_ = findCall
	_ = count

	// Walk children to find the call expression and the arrow.
	var walk func(n tsmorph.Node, depth int)
	walk = func(n tsmorph.Node, depth int) {
		if n.IsZero() {
			return
		}
		k := n.Kind()
		_ = k
		if ast.IsCallExpression(n.ASTNode()) {
			fmt.Printf("%*sCALL %s args=%d\n", depth*2, "", n.Text(), len(n.GetArguments()))
			for i, a := range n.GetArguments() {
				fmt.Printf("%*s  arg%d kind=%s isArrow=%v isObj=%v text=%q\n", depth*2, "", i, a.Kind().String(), ast.IsArrowFunction(a.ASTNode()), ast.IsObjectLiteralExpression(a.ASTNode()), a.Text())
			}
		}
		if ast.IsArrowFunction(n.ASTNode()) {
			fmt.Printf("%*sARROW kind=%s isArrow=%v isObj=%v text=%q\n", depth*2, "", n.Kind().String(), ast.IsArrowFunction(n.ASTNode()), ast.IsObjectLiteralExpression(n.ASTNode()), n.Text())
		}
		for _, c := range n.Children() {
			walk(c, depth+1)
		}
	}
	for _, st := range sf.Statements() {
		walk(st, 0)
	}
}
