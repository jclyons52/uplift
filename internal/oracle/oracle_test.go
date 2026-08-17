package oracle

import (
	"fmt"
	"testing"
)

func TestInterpolateParity(t *testing.T) {
	if testing.Short() {
		t.Skip("oracle run exercises the full pipeline")
	}
	r, err := Run(
		"/tmp/eslint-inspect/lib/linter/interpolate.js",
		"/tmp/eslint-inspect/tests/lib/linter/interpolate.js",
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	fmt.Println("JS OUT >>>", r.JSOut)
	fmt.Println("GO BUILD >>>", r.GoBuildOK, r.GoBuildEr)
	fmt.Println("GO RUN >>>", r.GoRunOut)
	fmt.Println("SUMMARY >>>", r.Summary())
	if !r.GoBuildOK {
		t.Fatalf("go build failed:\n%s", r.GoBuildEr)
	}
	if !r.ParityOK() {
		t.Errorf("parity diverged: %s\nJS:\n%s\nGO:\n%s", r.Summary(), r.JSOut, r.GoRunOut)
	}
}
